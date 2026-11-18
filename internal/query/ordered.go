package query

import (
	"lamina/internal/catalog"
	"lamina/internal/datum"
	"lamina/internal/index"
	"sort"
)

// OrderedPlan decides whether the query may walk one index in output order
// and stop after limit rows, instead of selecting from every match. The
// walk is only offered when it is provably equivalent to the full ordering
// and bounded against the fallback plan:
//   - an explicit positive limit and exactly one ordering term;
//   - no aggregate (aggregates must consume every match) and no forced scan
//     (transactions with private writes keep the scan read view);
//   - the ordering column is non-nullable: null values never enter index
//     entries, so a nullable ordering could not place nulls without an
//     unbounded side merge;
//   - an index on that column visible at the snapshot exists;
//   - limit < budget, where budget is the fallback plan's candidate count:
//     the walk needs at least limit entries, so otherwise it can never
//     finish for less than bounded selection would examine anyway.
//
// The returned plan carries the fallback candidate count in Candidates; it
// doubles as the walk's examination budget, so a walk that degenerates
// through sparse residuals or historical bloat provably falls back instead
// of out-costing the plan it replaced.
func OrderedPlan(c *catalog.Catalog, r catalog.Relation, registry *index.Registry, q Request, at uint64, budget int) (Plan, *index.Tree, bool) {
	if q.ForceScan || q.Aggregate != nil || len(q.Order) != 1 || q.Limit <= 0 || q.Limit >= budget {
		return Plan{}, nil, false
	}
	column, ok := r.Column(q.Order[0].Column)
	if !ok || column.Nullable {
		return Plan{}, nil, false
	}
	names := make([]string, 0, len(c.Indexes))
	for name := range c.Indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		definition := c.Indexes[name]
		if definition.Relation != q.Relation || definition.Column != q.Order[0].Column || definition.Created > at {
			continue
		}
		tree := registry.Trees[name]
		if tree == nil {
			continue
		}
		plan := Plan{Access: "ordered", Index: name, Candidates: budget, Snapshot: at, Residuals: len(q.Where)}
		return plan, tree, true
	}
	return Plan{}, nil, false
}

// ExecuteOrdered walks index entries in output order and rechecks every
// candidate against the snapshot's visible version, so historical entries,
// deletes and same-value keys can never pick the wrong rows:
//   - an entry whose key is invisible at the snapshot (deleted or not yet
//     committed) is skipped;
//   - an entry whose value differs from the visible row's current value is
//     a historical duplicate and is skipped — the row is emitted once, at
//     the entry matching its visible value;
//   - entries sharing one value form a group emitted in primary-key order
//     through a buffer bounded by the remaining limit.
//
// completed=false means the walk examined more entries than the fallback
// plan's candidate budget; the caller must discard the partial result and
// re-run bounded selection from scratch. The two strategies never mix: a
// result comes from exactly one of them.
func ExecuteOrdered(r catalog.Relation, q Request, plan Plan, tree *index.Tree, lookup func(key string) (datum.Row, bool)) (result Result, completed bool) {
	result = Result{Rows: []datum.Row{}, Plan: plan}
	limit := q.Limit
	column := q.Order[0].Column
	group := newTopK(rowLess(nil, r.Primary), 0)
	out := make([]datum.Row, 0, limit)
	var groupValue datum.Value
	inGroup := false
	stop := ""
	flush := func() {
		if !inGroup {
			return
		}
		out = append(out, group.sorted()...)
		inGroup = false
	}
	tree.Iterate(q.Order[0].Desc, func(e index.Entry) bool {
		result.Plan.Examined++
		if result.Plan.Examined > plan.Candidates {
			stop = "budget"
			return false
		}
		if inGroup && datum.Order(e.Value, groupValue) != 0 {
			flush()
			if len(out) >= limit {
				stop = "limit"
				return false
			}
		}
		row, ok := lookup(e.Key)
		if !ok {
			return true
		}
		if datum.Order(row[column], e.Value) != 0 {
			return true
		}
		result.Scanned++
		if !Matches(row, q.Where) {
			return true
		}
		result.Matched++
		if !inGroup {
			inGroup = true
			groupValue = e.Value
			group.reset(limit - len(out))
		}
		group.offer(row)
		return true
	})
	if stop == "budget" {
		return result, false
	}
	flush()
	// When the walk stops at the limit, unexamined entries may hold further
	// matches, so truncated stays conservative; an exhausted walk reports
	// the exact pre-limit match count like every other plan.
	result.Truncated = stop == "limit" || result.Matched > limit
	for _, row := range out {
		result.Rows = append(result.Rows, project(row, q.Columns))
	}
	return result, true
}
