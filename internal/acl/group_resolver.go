package acl

import (
	"fmt"
	"sort"
)

const (
	stageGroups    = "groups"
	modelResGroups = "res.groups"
)

// GroupResolver is Stage 2 of the ACL pipeline.
// It takes a ResolvedUser (from Stage 1) and a set of group records fetched
// from Odoo, then expands the user's direct groups through the implied_ids
// chain to produce the full effective group set.
//
// Odoo group inheritance: if group A has implied_ids = [B, C], and group B
// has implied_ids = [D], then a user in group A effectively belongs to
// {A, B, C, D}. This is transitive and must handle cycles.
type GroupResolver struct{}

// NewGroupResolver creates a GroupResolver.
func NewGroupResolver() *GroupResolver {
	return &GroupResolver{}
}

// Resolve expands the user's direct group memberships through implied_ids
// to produce the full effective group set.
//
// Parameters:
//   - schema: the deserialized models JSONB — used to verify res.groups exists
//   - user: the ResolvedUser from Stage 1 (provides direct GroupIDs)
//   - groupData: all res.groups records from the agent, as raw maps.
//     Each must have at minimum: id, name, implied_ids.
func (r *GroupResolver) Resolve(schema SchemaModels, user *ResolvedUser, groupData []map[string]any) (*ResolvedGroups, *StageResult, error) {
	// ── Superuser bypass ─────────────────────────────────────────────────
	if user.IsSuperUser() {
		return &ResolvedGroups{
				DirectIDs:    user.GroupIDs,
				EffectiveIDs: user.GroupIDs,
				EffectiveSet: intSliceToSet(user.GroupIDs),
			}, &StageResult{
				Stage:   stageGroups,
				Verdict: VerdictOK,
				Reason:  fmt.Sprintf("user %d is SUPERUSER — group resolution skipped, all access granted", user.UID),
			}, nil
	}

	// ── Validate res.groups exists in schema ─────────────────────────────
	if _, ok := schema[modelResGroups]; !ok {
		return nil, &StageResult{
			Stage:   stageGroups,
			Verdict: VerdictError,
			Reason:  "model 'res.groups' not found in schema snapshot",
		}, nil
	}

	// ── Parse group records into lookup map ──────────────────────────────
	groupByID, err := parseGroupRecords(groupData)
	if err != nil {
		return nil, nil, fmt.Errorf("group resolver: %w", err)
	}

	if len(groupByID) == 0 {
		return nil, &StageResult{
			Stage:   stageGroups,
			Verdict: VerdictError,
			Reason:  "no group records provided — agent may not have fetched res.groups",
		}, nil
	}

	// ── Expand implied groups transitively ───────────────────────────────
	directSet := intSliceToSet(user.GroupIDs)
	effectiveSet := make(map[int]bool, len(user.GroupIDs)*2)

	// Copy direct groups into effective set.
	for _, gid := range user.GroupIDs {
		effectiveSet[gid] = true
	}

	// BFS through implied_ids.
	queue := make([]int, len(user.GroupIDs))
	copy(queue, user.GroupIDs)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		group, ok := groupByID[current]
		if !ok {
			continue // group not in data — may be from a module not installed
		}

		for _, implied := range group.ImpliedIDs {
			if !effectiveSet[implied] {
				effectiveSet[implied] = true
				queue = append(queue, implied)
			}
		}
	}

	// ── Build result ─────────────────────────────────────────────────────
	var impliedIDs []int
	effectiveIDs := make([]int, 0, len(effectiveSet))

	for gid := range effectiveSet {
		effectiveIDs = append(effectiveIDs, gid)
		if !directSet[gid] {
			impliedIDs = append(impliedIDs, gid)
		}
	}

	sort.Ints(effectiveIDs)
	sort.Ints(impliedIDs)

	// Collect full GroupRecord entries for all effective groups.
	groups := make([]GroupRecord, 0, len(effectiveIDs))
	for _, gid := range effectiveIDs {
		if g, ok := groupByID[gid]; ok {
			groups = append(groups, g)
		}
	}

	resolved := &ResolvedGroups{
		DirectIDs:    user.GroupIDs,
		ImpliedIDs:   impliedIDs,
		EffectiveIDs: effectiveIDs,
		EffectiveSet: effectiveSet,
		Groups:       groups,
	}

	// ── Determine verdict ────────────────────────────────────────────────
	if len(effectiveIDs) == 0 {
		return resolved, &StageResult{
			Stage:   stageGroups,
			Verdict: VerdictOK,
			Reason:  fmt.Sprintf("user %d (%s) has no groups — only global rules will apply", user.UID, user.Login),
			Detail:  resolved,
		}, nil
	}

	reason := fmt.Sprintf(
		"user %d (%s): %d direct groups + %d implied = %d effective groups",
		user.UID, user.Login,
		len(user.GroupIDs), len(impliedIDs), len(effectiveIDs),
	)

	return resolved, &StageResult{
		Stage:   stageGroups,
		Verdict: VerdictOK,
		Reason:  reason,
		Detail:  resolved,
	}, nil
}

// parseGroupRecords converts raw agent data ([]map[string]any) into a map
// keyed by group ID.
func parseGroupRecords(raw []map[string]any) (map[int]GroupRecord, error) {
	result := make(map[int]GroupRecord, len(raw))
	for _, r := range raw {
		id, err := extractInt(r, "id")
		if err != nil {
			return nil, fmt.Errorf("group record missing 'id': %w", err)
		}

		group := GroupRecord{
			ID:         id,
			Name:       extractString(r, "name"),
			FullName:   extractString(r, "full_name"),
			ImpliedIDs: extractIntSlice(r, "implied_ids"),
			CategoryID: extractMany2OneID(r, "category_id"),
		}
		result[id] = group
	}
	return result, nil
}

func intSliceToSet(ids []int) map[int]bool {
	s := make(map[int]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
