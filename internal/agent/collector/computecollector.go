package collector

// computecollector.go — polls Odoo's ir.profile model for compute-method
// timing data and emits aggregator.Event entries with IsCompute=true so the
// ingest worker can build ComputeChain visualizations.
//
// Pipeline:
//
//	ir.model.fields  ─── build depends cache on startup ──► methodIndex
//	ir.profile       ─── poll every N seconds           ──► speedscope JSON
//	computeparser    ─── extract _compute_* frames      ──► []ComputeFrameResult
//	methodIndex      ─── look up model + field + deps   ──► aggregator.Event{IsCompute:true}
//	aggregator.EventCh ◄────────────────────────────────────
//
// Requirements on the Odoo side:
//   - Odoo 15+ (ir.profile model must exist)
//   - Profiling enabled for at least some requests.
//     The agent can optionally enable it via the `enable_profiling` RPC call.
//   - The Odoo user must have read access to ir.profile.
//
// To enable Odoo profiling for all requests (development only), set the
// environment variable in the Odoo process:
//
//	ODOO_PROFILING=1  (or via Odoo settings → Technical → Profiling)

import (
	"context"
	"fmt"
	"strings"
	"time"

	"Intelligent_Dev_ToolKit_Odoo/internal/agent/aggregator"
	"Intelligent_Dev_ToolKit_Odoo/internal/agent/odoo"

	"github.com/rs/zerolog"
)

// profilingEnabledUntil is the datetime written to Odoo's config parameter
// when AGENT_ODOO_ENABLE_PROFILING=true. Far future so it never expires.
const profilingEnabledUntil = "2099-12-31 23:59:59"

// computeFieldMeta holds the @api.depends metadata for one computed field.
type computeFieldMeta struct {
	Model     string   // "sale.order"
	FieldName string   // "amount_total"
	Method    string   // "_compute_amount_total"
	DependsOn []string // ["order_line", "order_line.price_unit", ...]
	Module    string   // "sale"
}
type nodeState struct {
	frame ComputeFrameResult
	metas []computeFieldMeta
}

// ComputeChainCollector polls ir.profile and emits compute-chain events.
type ComputeChainCollector struct {
	client  *odoo.Client
	eventCh chan<- aggregator.Event
	logger  zerolog.Logger

	// lastProfileID is the high-water mark for ir.profile polling.
	lastProfileID int

	// methodIndex maps "_compute_foo" → []computeFieldMeta.
	// A method name can appear in multiple models (inheritance), so we keep a slice.
	methodIndex map[string][]computeFieldMeta

	// dependsCacheBuilt tracks whether the initial field cache has been loaded.
	dependsCacheBuilt bool

	// autoEnableProfiling, when true, sets base_setup.profiling_enabled_until
	// in Odoo on the first Poll so that ir.profile records are created.
	autoEnableProfiling bool
	// profilingEnabled tracks whether enableProfilingInOdoo has succeeded.
	profilingEnabled bool
}

// NewComputeChainCollector creates a new ComputeChainCollector.
// Set autoEnableProfiling=true to have the collector automatically write
// base_setup.profiling_enabled_until into Odoo on startup (dev only).
func NewComputeChainCollector(
	client *odoo.Client,
	eventCh chan<- aggregator.Event,
	logger zerolog.Logger,
	autoEnableProfiling bool,
) *ComputeChainCollector {
	return &ComputeChainCollector{
		client:              client,
		eventCh:             eventCh,
		logger:              logger.With().Str("component", "compute-chain-collector").Logger(),
		methodIndex:         make(map[string][]computeFieldMeta),
		autoEnableProfiling: autoEnableProfiling,
	}
}

// RunLoop polls on the given interval until ctx is canceled.
func (c *ComputeChainCollector) RunLoop(ctx context.Context, interval time.Duration) {
	runPollLoop(ctx, interval, c.logger, "compute-chain", c.Poll)
}

// Poll performs one collection cycle:
//  1. Optionally enables Odoo profiling (once, if autoEnableProfiling is set).
//  2. Builds (or refreshes) the computed-field metadata cache from ir.model.fields.
//  3. Fetches new ir.profile records since the last poll.
//  4. Parses their speedscope traces to find _compute_* frames.
//  5. Emits one aggregator.Event per compute node.
func (c *ComputeChainCollector) Poll(ctx context.Context) error {
	// Enable Odoo profiling once so ir.profile records are created.
	if c.autoEnableProfiling && !c.profilingEnabled {
		if err := c.enableProfilingInOdoo(ctx); err != nil {
			c.logger.Warn().Err(err).Msg("failed to enable Odoo profiling automatically — set it manually via Odoo Settings → Technical → Profiling")
		} else {
			c.profilingEnabled = true
		}
	}

	// Build the depends cache once (and refresh every 10 polls via a simple
	// counter tracked by dependsCacheBuilt + a refresh ticker in the caller).
	if !c.dependsCacheBuilt {
		if err := c.buildDependsCache(ctx); err != nil {
			// Non-fatal: log and continue — we can still emit partial events.
			c.logger.Warn().Err(err).Msg("failed to build @api.depends cache, compute events will lack dependency info")
		}
		c.dependsCacheBuilt = true
	}

	profiles, err := c.fetchNewProfiles(ctx)
	if err != nil {
		return fmt.Errorf("fetch ir.profile: %w", err)
	}
	if len(profiles) == 0 {
		return nil
	}

	emitted := 0
	for _, prof := range profiles {
		n, err := c.processProfile(ctx, prof)
		if err != nil {
			c.logger.Warn().Err(err).
				Int("profile_id", intVal(prof["id"])).
				Msg("failed to process ir.profile record")
			continue
		}
		emitted += n
	}

	if emitted > 0 {
		c.logger.Info().
			Int("profiles_processed", len(profiles)).
			Int("compute_events_emitted", emitted).
			Msg("compute chain events captured")
	}

	return nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// buildDependsCache fetches all computed, non-stored fields from ir.model.fields
// and builds the methodIndex map used to enrich compute events.
func (c *ComputeChainCollector) buildDependsCache(ctx context.Context) error {
	// Domain: fields that have a compute method defined.
	// We include both stored and non-stored computed fields.
	domain := []any{
		[]any{"compute", "!=", false},
		[]any{"compute", "!=", ""},
	}

	fields := []string{"model", "name", "compute", "depends", "modules"}

	records, err := fetchAllPages(ctx, c.client, "ir.model.fields", fields, domain, 2000)
	if err != nil {
		return fmt.Errorf("ir.model.fields search_read: %w", err)
	}

	// Reset and rebuild.
	c.methodIndex = make(map[string][]computeFieldMeta, len(records))

	for _, r := range records {
		model := stringVal(r["model"])
		fieldName := stringVal(r["name"])
		computeMethod := stringVal(r["compute"])
		dependsStr := stringVal(r["depends"])
		modules := stringVal(r["modules"])

		if model == "" || fieldName == "" || computeMethod == "" {
			continue
		}

		// `depends` is a comma-separated list of dotted field paths,
		// e.g. "order_line,order_line.price_unit,order_line.product_id"
		var dependsOn []string
		if dependsStr != "" {
			for _, dep := range strings.Split(dependsStr, ",") {
				dep = strings.TrimSpace(dep)
				if dep != "" {
					dependsOn = append(dependsOn, dep)
				}
			}
		}

		// Pick first module name (modules is comma-separated too).
		module := ""
		if modules != "" {
			parts := strings.SplitN(modules, ",", 2)
			module = strings.TrimSpace(parts[0])
		}

		meta := computeFieldMeta{
			Model:     model,
			FieldName: fieldName,
			Method:    computeMethod,
			DependsOn: dependsOn,
			Module:    module,
		}
		c.methodIndex[computeMethod] = append(c.methodIndex[computeMethod], meta)
	}

	c.logger.Info().
		Int("computed_fields", len(records)).
		Int("methods_indexed", len(c.methodIndex)).
		Msg("@api.depends cache built")

	return nil
}

// fetchNewProfiles fetches ir.profile records created after the last known ID.
func (c *ComputeChainCollector) fetchNewProfiles(ctx context.Context) ([]map[string]any, error) {
	domain := []any{
		[]any{"id", ">", c.lastProfileID},
	}

	profileFields := []string{"id", "name", "duration", "create_date", "traces_async"}

	records, err := FetchRecordsWithDomain(ctx, c.client, "ir.profile", profileFields, domain,
		map[string]any{
			"order": "id asc",
			"limit": 50, // process at most 50 profiles per poll
		},
	)
	if err != nil {
		// ir.profile doesn't exist on Odoo <15 or when profiling is not installed.
		// Treat as a non-fatal empty result so the collector keeps running.
		if isModelNotFoundError(err) {
			c.logger.Debug().Msg("ir.profile model not available on this Odoo instance (requires Odoo 15+)")
			return nil, nil
		}
		// Log the real error (e.g. access denied) so it is visible.
		c.logger.Error().Err(err).Msg("failed to fetch ir.profile records — check Odoo user permissions")
		return nil, err
	}

	// Advance the high-water mark.
	for _, r := range records {
		if id := intVal(r["id"]); id > c.lastProfileID {
			c.lastProfileID = id
		}
	}

	return records, nil
}

// Stage 1: Build Nodes
func (c *ComputeChainCollector) buildNodes(frames []ComputeFrameResult) []nodeState {
	nodes := make([]nodeState, 0, len(frames))

	for _, f := range frames {
		metas := c.methodIndex[f.MethodName]
		if len(metas) == 0 {
			metas = []computeFieldMeta{{
				Method: f.MethodName,
				Module: moduleFromSourceFile(f.SourceFile),
			}}
		}
		nodes = append(nodes, nodeState{frame: f, metas: metas})
	}

	return nodes
}

// Stage 2: Field → Method Map
func buildFieldToMethod(nodes []nodeState) map[string]string {
	fieldToMethod := make(map[string]string)

	for _, ns := range nodes {
		for _, m := range ns.metas {
			if m.FieldName != "" {
				key := m.Model + "." + m.FieldName
				fieldToMethod[key] = m.Method
			}
		}
	}

	return fieldToMethod
}

// Stage 3: Dependency Graph
func buildDependencyGraph(
	nodes []nodeState,
	fieldToMethod map[string]string,
) (methodDepth map[string]int, methodParent map[string]string) {

	methodDepth = make(map[string]int)
	methodParent = make(map[string]string)

	for _, ns := range nodes {
		for _, m := range ns.metas {
			for _, dep := range m.DependsOn {

				depKey := m.Model + "." + dep
				parentMethod, ok := fieldToMethod[depKey]
				if !ok || parentMethod == m.Method {
					continue
				}

				if methodDepth[m.Method] <= methodDepth[parentMethod] {
					methodDepth[m.Method] = methodDepth[parentMethod] + 1
					methodParent[m.Method] = parentMethod
				}
			}
		}
	}

	return
}

// Stage 4: Trigger Field
func resolveTriggerField(nodes []nodeState, methodDepth map[string]int) string {
	for _, ns := range nodes {
		if methodDepth[ns.frame.MethodName] != 0 {
			continue
		}

		for _, m := range ns.metas {
			if m.FieldName != "" {
				return m.Model + "." + m.FieldName
			}
		}
	}
	return ""
}

// Stage 5: Emit Events
func (c *ComputeChainCollector) emitEvents(
	nodes []nodeState,
	methodDepth map[string]int,
	methodParent map[string]string,
	triggerField string,
	profileName string,
	profileDate int64,
) int {

	emitted := 0

	for _, ns := range nodes {
		method := ns.frame.MethodName
		parentMethod := methodParent[method]

		metas := ns.metas
		if len(metas) == 0 {
			metas = []computeFieldMeta{{Method: method}}
		}

		for _, m := range metas {

			ev := buildEvent(ns, m, triggerField, profileDate)

			// Override trigger from parent
			if parentMethod != "" {
				if parentMetas, ok := c.methodIndex[parentMethod]; ok && len(parentMetas) > 0 {
					ev.TriggerField = parentMetas[0].Model + "." + parentMetas[0].FieldName
				}
			}

			if ev.Model == "" {
				ev.Model = profileName
			}

			if c.sendEvent(ev, method) {
				emitted++
			}
		}
	}

	return emitted
}

// Stage 6: Helpers

func buildEvent(
	ns nodeState,
	m computeFieldMeta,
	triggerField string,
	profileDate int64,
) aggregator.Event {

	return aggregator.Event{
		Category:     "profiler",
		Model:        m.Model,
		Method:       m.Method,
		Module:       m.Module,
		DurationMS:   ns.frame.DurationMS,
		IsCompute:    true,
		FieldName:    m.FieldName,
		DependsOn:    m.DependsOn,
		TriggerField: triggerField,
		Timestamp:    time.Unix(profileDate, 0),
	}
}

func (c *ComputeChainCollector) sendEvent(ev aggregator.Event, method string) bool {
	select {
	case c.eventCh <- ev:
		return true
	default:
		c.logger.Warn().
			Str("method", method).
			Msg("event channel full, dropping compute event")
		return false
	}
}

// processProfile parses one ir.profile record and emits compute events.
// Returns the number of events emitted.

func (c *ComputeChainCollector) processProfile(
	_ context.Context,
	prof map[string]any,
) (int, error) {

	tracesJSON := stringVal(prof["traces_async"])
	if tracesJSON == "" {
		return 0, nil
	}

	frames, err := ParseComputeFrames(tracesJSON)
	if err != nil {
		return 0, fmt.Errorf("parse speedscope: %w", err)
	}
	if len(frames) == 0 {
		return 0, nil
	}

	profileName := stringVal(prof["name"])
	profileDate := parseDate(stringVal(prof["create_date"]))

	nodes := c.buildNodes(frames)
	fieldToMethod := buildFieldToMethod(nodes)

	methodDepth, methodParent := buildDependencyGraph(nodes, fieldToMethod)

	triggerField := resolveTriggerField(nodes, methodDepth)

	emitted := c.emitEvents(nodes, methodDepth, methodParent, triggerField, profileName, profileDate.Unix())

	return emitted, nil
}

// func (c *ComputeChainCollector) processProfile(_ context.Context, prof map[string]any) (int, error) {
// 	tracesJSON := stringVal(prof["traces_async"])
// 	if tracesJSON == "" {
// 		return 0, nil // profile has no sampled traces (e.g. SQL-only profile)
// 	}

// 	frames, err := ParseComputeFrames(tracesJSON)
// 	if err != nil {
// 		return 0, fmt.Errorf("parse speedscope: %w", err)
// 	}
// 	if len(frames) == 0 {
// 		return 0, nil
// 	}

// 	profileName := stringVal(prof["name"])
// 	profileDate := parseDate(stringVal(prof["create_date"]))

// 	// Compute the trigger field: first compute node at depth 0 (root of chain).
// 	// We'll infer it below when building nodes.
// 	// Build a parent-tracking map for chain reconstruction.
// 	// Simple heuristic: if method A's DependsOn includes a field computed by method B,
// 	// then B is a parent of A.
// 	type nodeState struct {
// 		frame ComputeFrameResult
// 		metas []computeFieldMeta
// 	}

// 	// Resolve metas for all frames.
// 	nodes := make([]nodeState, 0, len(frames))
// 	for _, f := range frames {
// 		metas := c.methodIndex[f.MethodName]
// 		if len(metas) == 0 {
// 			// Unknown method — emit with limited metadata (just the method name).
// 			metas = []computeFieldMeta{{
// 				Method: f.MethodName,
// 				Module: moduleFromSourceFile(f.SourceFile),
// 			}}
// 		}
// 		nodes = append(nodes, nodeState{frame: f, metas: metas})
// 	}

// 	// Assign depths based on dependency relationships.
// 	// Build a quick lookup: field_name → method_name for fields found in this profile.
// 	fieldToMethod := make(map[string]string) // "model.field" → method
// 	for _, ns := range nodes {
// 		for _, m := range ns.metas {
// 			if m.FieldName != "" {
// 				fieldToMethod[m.Model+"."+m.FieldName] = m.Method
// 			}
// 		}
// 	}

// 	// For each node, check if any of its depends fields are computed by another
// 	// node in this profile — if so, that node is a child (higher depth).
// 	methodDepth := make(map[string]int)
// 	methodParent := make(map[string]string)
// 	for _, ns := range nodes {
// 		for _, m := range ns.metas {
// 			for _, dep := range m.DependsOn {
// 				depKey := m.Model + "." + dep
// 				if parentMethod, ok := fieldToMethod[depKey]; ok && parentMethod != m.Method {
// 					// This node depends on parentMethod → parent is one level up.
// 					if methodDepth[m.Method] <= methodDepth[parentMethod] {
// 						methodDepth[m.Method] = methodDepth[parentMethod] + 1
// 						methodParent[m.Method] = parentMethod
// 					}
// 				}
// 			}
// 		}
// 	}

// 	// Determine trigger field: the root of the chain (depth 0, longest chain).
// 	triggerField := ""
// 	for _, ns := range nodes {
// 		if methodDepth[ns.frame.MethodName] == 0 {
// 			for _, m := range ns.metas {
// 				if m.FieldName != "" {
// 					triggerField = m.Model + "." + m.FieldName
// 					break
// 				}
// 			}
// 			break
// 		}
// 	}

// 	emitted := 0
// 	for _, ns := range nodes {
// 		method := ns.frame.MethodName
// 		depth := methodDepth[method]
// 		parentMethod := methodParent[method]

// 		// Emit one event per meta (model) that this method serves.
// 		// In most cases there is exactly one; with inheritance there may be a few.
// 		metasToEmit := ns.metas
// 		if len(metasToEmit) == 0 {
// 			metasToEmit = []computeFieldMeta{{Method: method}}
// 		}

// 		for _, m := range metasToEmit {
// 			ev := aggregator.Event{
// 				Category:     "profiler",
// 				Model:        m.Model,
// 				Method:       m.Method,
// 				Module:       m.Module,
// 				DurationMS:   ns.frame.DurationMS,
// 				IsCompute:    true,
// 				FieldName:    m.FieldName,
// 				DependsOn:    m.DependsOn,
// 				TriggerField: triggerField,
// 				Timestamp:    profileDate,
// 			}

// 			// Encode parent reference as the first field name of the parent method.
// 			if parentMethod != "" {
// 				if parentMetas, ok := c.methodIndex[parentMethod]; ok && len(parentMetas) > 0 {
// 					ev.TriggerField = parentMetas[0].Model + "." + parentMetas[0].FieldName
// 				}
// 			}

// 			_ = depth // depth is encoded server-side from the parent chain

// 			// Label for the event (used if model/method are missing).
// 			if ev.Model == "" {
// 				ev.Model = profileName
// 			}

// 			// Non-blocking send.
// 			select {
// 			case c.eventCh <- ev:
// 				emitted++
// 			default:
// 				c.logger.Warn().Str("method", method).Msg("event channel full, dropping compute event")
// 			}
// 		}
// 	}

// 	return emitted, nil
// }

// enableProfilingInOdoo writes base_setup.profiling_enabled_until to Odoo's
// ir.config_parameter so that all subsequent Odoo requests are profiled and
// stored as ir.profile records. This is a no-op after the first successful call.
func (c *ComputeChainCollector) enableProfilingInOdoo(ctx context.Context) error {
	body, err := c.client.ExecuteKw(
		ctx,
		"ir.config_parameter",
		"set_param",
		[]any{"base_setup.profiling_enabled_until", profilingEnabledUntil},
		map[string]any{},
	)
	if err != nil {
		return fmt.Errorf("set_param: %w", err)
	}

	// ExecuteKw returns a fault response on permission errors; check for it.
	if len(body) > 0 && strings.Contains(string(body), "<fault>") {
		return fmt.Errorf("odoo rejected set_param: %s", string(body))
	}

	c.logger.Info().
		Str("until", profilingEnabledUntil).
		Msg("Odoo profiling enabled (base_setup.profiling_enabled_until set)")
	return nil
}

// isModelNotFoundError returns true when the Odoo XML-RPC error indicates that
// the model doesn't exist (Odoo 14 and below, or when profiling is not installed).
// Note: "access denied" is intentionally excluded — that is a permissions problem
// that should be surfaced as an error, not silently swallowed as "model not found".
func isModelNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "object has no attribute") ||
		strings.Contains(msg, "model not found") ||
		strings.Contains(msg, "keyerror: 'ir.profile'") ||
		(strings.Contains(msg, "ir.profile") && !strings.Contains(msg, "access"))
}
