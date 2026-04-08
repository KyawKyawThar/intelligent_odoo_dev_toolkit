package collector

// computeparser.go — parses Odoo ir.profile speedscope JSON to extract
// _compute_* method frames with their timing data.
//
// Odoo 15+ stores profiling data in ir.profile.traces_async as a JSON string
// using the speedscope file format (https://www.speedscope.app/file-format-schema.json).
//
// Relevant structure:
//
//	{
//	  "shared": {
//	    "frames": [
//	      {"name": "_compute_amount_total", "file": "...sale_order.py", "line": 123},
//	      ...
//	    ]
//	  },
//	  "profiles": [{
//	    "type": "sampled",
//	    "unit": "milliseconds",
//	    "startValue": 0,
//	    "endValue": 1234.5,
//	    "samples": [[0, 1, 2], [3, 4]],   // each sample = call stack (frame indices)
//	    "weights": [10.5, 20.0]            // time spent on each sample (ms)
//	  }]
//	}

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// speedscopeRoot is the top-level speedscope JSON structure.
type speedscopeRoot struct {
	Shared   speedscopeShared    `json:"shared"`
	Profiles []speedscopeProfile `json:"profiles"`
}

// speedscopeShared holds the shared frame table.
type speedscopeShared struct {
	Frames []speedscopeFrame `json:"frames"`
}

// speedscopeFrame describes a single call-stack frame.
type speedscopeFrame struct {
	Name string `json:"name"` // function / method name
	File string `json:"file"` // source file path
	Line int    `json:"line"` // line number
}

// speedscopeProfile is a single sampled profile within the file.
type speedscopeProfile struct {
	Type       string    `json:"type"`
	Unit       string    `json:"unit"`
	StartValue float64   `json:"startValue"`
	EndValue   float64   `json:"endValue"`
	Samples    [][]int   `json:"samples"` // each sample = []frameIndex
	Weights    []float64 `json:"weights"` // ms per sample (parallel to Samples)
}

// ComputeFrameResult is a parsed compute method call with timing.
type ComputeFrameResult struct {
	// MethodName is the Python method name, e.g. "_compute_amount_total".
	MethodName string
	// SourceFile is the Odoo addon source file, e.g. "sale/models/sale_order.py".
	SourceFile string
	// DurationMS is the total sampled CPU time spent inside this method.
	DurationMS int
	// ProfileDurationMS is the total wall-clock duration of the profile.
	ProfileDurationMS int
}

//
// -------------------------------
// Stage 1: Parse
// -------------------------------
//

func parseSpeedscope(tracesJSON string) (*speedscopeRoot, error) {
	var root speedscopeRoot
	if err := json.Unmarshal([]byte(tracesJSON), &root); err != nil {
		return nil, err
	}
	return &root, nil
}

//
// -------------------------------
// Stage 2: Extract compute frames
// -------------------------------
//

func extractComputeFrames(frames []speedscopeFrame) map[int]speedscopeFrame {
	computeIdx := make(map[int]speedscopeFrame)

	for i, f := range frames {
		if isComputeMethod(f.Name) {
			computeIdx[i] = f
		}
	}
	return computeIdx
}

//
// -------------------------------
// Stage 3: Aggregate time
// -------------------------------
//

func aggregateComputeTime(
	profiles []speedscopeProfile,
	computeIdx map[int]speedscopeFrame,
) (msPerFrame map[int]float64, totalProfileMS float64) {

	msPerFrame = make(map[int]float64)
	totalProfileMS = 0.0

	for _, prof := range profiles {
		if prof.Type != "sampled" {
			continue
		}

		totalProfileMS += profileDuration(prof)
		processSamples(prof, computeIdx, msPerFrame)
	}

	return
}

func profileDuration(prof speedscopeProfile) float64 {
	return prof.EndValue - prof.StartValue
}

func processSamples(
	prof speedscopeProfile,
	computeIdx map[int]speedscopeFrame,
	msPerFrame map[int]float64,
) {
	for sampleIdx, stack := range prof.Samples {

		weight := normalizeWeight(prof, sampleIdx)

		for _, frameIdx := range stack {
			if _, ok := computeIdx[frameIdx]; ok {
				msPerFrame[frameIdx] += weight
			}
		}
	}
}

//
// -------------------------------
// Stage 4: Weight normalization
// -------------------------------
//

func normalizeWeight(prof speedscopeProfile, sampleIdx int) float64 {
	if sampleIdx >= len(prof.Weights) {
		return 0
	}

	weight := prof.Weights[sampleIdx]

	switch {
	case strings.EqualFold(prof.Unit, "nanoseconds"):
		return weight / 1e6
	case strings.EqualFold(prof.Unit, "microseconds"):
		return weight / 1e3
	default:
		return weight
	}
}

//
// -------------------------------
// Stage 5: Build results
// -------------------------------
//

func buildResults(
	msPerFrame map[int]float64,
	computeIdx map[int]speedscopeFrame,
	totalProfileMS float64,
) []ComputeFrameResult {

	profileDurationMS := int(totalProfileMS)
	byMethod := mergeByMethod(msPerFrame, computeIdx, profileDurationMS)

	results := make([]ComputeFrameResult, 0, len(byMethod))
	for _, r := range byMethod {
		if r.DurationMS < 1 {
			r.DurationMS = 1
		}
		results = append(results, *r)
	}

	return results
}

func mergeByMethod(
	msPerFrame map[int]float64,
	computeIdx map[int]speedscopeFrame,
	profileDurationMS int,
) map[string]*ComputeFrameResult {

	byMethod := make(map[string]*ComputeFrameResult)

	for idx, ms := range msPerFrame {
		frame := computeIdx[idx]
		key := frame.Name

		if existing, ok := byMethod[key]; ok {
			existing.DurationMS += int(ms)
			continue
		}

		byMethod[key] = &ComputeFrameResult{
			MethodName:        frame.Name,
			SourceFile:        frame.File,
			DurationMS:        int(ms),
			ProfileDurationMS: profileDurationMS,
		}
	}

	return byMethod
}

// ParseComputeFrames parses a speedscope JSON string (from ir.profile.traces_async)
// and returns one ComputeFrameResult per unique _compute_* frame found.
// Returns nil (not an error) when no compute frames are present.
func ParseComputeFrames(tracesJSON string) ([]ComputeFrameResult, error) {
	if tracesJSON == "" {
		return nil, nil
	}

	root, err := parseSpeedscope(tracesJSON)
	if err != nil {
		return nil, err
	}

	computeIdx := extractComputeFrames(root.Shared.Frames)
	if len(computeIdx) == 0 {
		return nil, nil
	}

	msPerFrame, totalProfileMS := aggregateComputeTime(root.Profiles, computeIdx)

	results := buildResults(msPerFrame, computeIdx, totalProfileMS)
	return results, nil
}

// func ParseComputeFrames(tracesJSON string) ([]ComputeFrameResult, error) {
// 	if tracesJSON == "" {
// 		return nil, nil
// 	}

// 	var root speedscopeRoot
// 	if err := json.Unmarshal([]byte(tracesJSON), &root); err != nil {
// 		return nil, err
// 	}

// 	frames := root.Shared.Frames

// 	// Build index of compute frame indices → frame.
// 	computeIdx := make(map[int]speedscopeFrame) // frameIndex → frame
// 	for i, f := range frames {
// 		if isComputeMethod(f.Name) {
// 			computeIdx[i] = f
// 		}
// 	}
// 	if len(computeIdx) == 0 {
// 		return nil, nil
// 	}

// 	// Accumulate milliseconds per compute frame across all profiles.
// 	// key = frame index, value = total ms
// 	msPerFrame := make(map[int]float64)
// 	totalProfileMS := 0.0

// 	for _, prof := range root.Profiles {
// 		if prof.Type != "sampled" {
// 			continue
// 		}
// 		totalProfileMS += prof.EndValue - prof.StartValue

// 		for sampleIdx, stack := range prof.Samples {
// 			weight := 0.0
// 			if sampleIdx < len(prof.Weights) {
// 				weight = prof.Weights[sampleIdx]
// 			}
// 			// Account for unit — Odoo uses "milliseconds" but guard for "nanoseconds".
// 			if strings.EqualFold(prof.Unit, "nanoseconds") {
// 				weight /= 1e6
// 			} else if strings.EqualFold(prof.Unit, "microseconds") {
// 				weight /= 1e3
// 			}

// 			for _, frameIdx := range stack {
// 				if _, isCompute := computeIdx[frameIdx]; isCompute {
// 					msPerFrame[frameIdx] += weight
// 				}
// 			}
// 		}
// 	}

// 	profileDurationMS := int(totalProfileMS)

// 	// Deduplicate by method name — if the same _compute_ appears in multiple
// 	// frames (rare), sum their times.
// 	byMethod := make(map[string]*ComputeFrameResult)
// 	for idx, ms := range msPerFrame {
// 		f := computeIdx[idx]
// 		key := f.Name
// 		if existing, ok := byMethod[key]; ok {
// 			existing.DurationMS += int(ms)
// 		} else {
// 			byMethod[key] = &ComputeFrameResult{
// 				MethodName:        f.Name,
// 				SourceFile:        f.File,
// 				DurationMS:        int(ms),
// 				ProfileDurationMS: profileDurationMS,
// 			}
// 		}
// 	}

// 	results := make([]ComputeFrameResult, 0, len(byMethod))
// 	for _, r := range byMethod {
// 		if r.DurationMS < 1 {
// 			r.DurationMS = 1 // floor at 1ms so the node is visible
// 		}
// 		results = append(results, *r)
// 	}
// 	return results, nil
// }

// isComputeMethod returns true for Python methods that are Odoo compute handlers.
// Matches: _compute_*, _inverse_*, _search_* (all are field computation methods).
func isComputeMethod(name string) bool {
	return strings.HasPrefix(name, "_compute_") ||
		strings.HasPrefix(name, "_inverse_")
}

// moduleFromSourceFile infers the Odoo module name from a file path like
// "/opt/odoo/addons/sale/models/sale_order.py" → "sale".
// Returns empty string when the path doesn't follow the addons/ convention.
func moduleFromSourceFile(path string) string {
	// Normalise separators.
	path = filepath.ToSlash(path)

	// Look for the addons/ segment.
	for _, marker := range []string{"/addons/", "/odoo/addons/", "/enterprise/"} {
		if idx := strings.Index(path, marker); idx != -1 {
			rest := path[idx+len(marker):]
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) >= 1 && parts[0] != "" {
				return parts[0]
			}
		}
	}
	return ""
}
