package query

import (
	"fmt"
	"lamina/internal/catalog"
	"lamina/internal/datum"
	"math"
)

func Execute(r catalog.Relation, q Request, rows map[string]datum.Row, plan Plan, keys []string) (Result, error) {
	result := Result{Rows: []datum.Row{}, Plan: plan}
	if plan.Access == "scan" {
		keys = datum.Keys(rows)
	}
	limit := q.Limit
	if limit == 0 {
		limit = 1000
	}
	if q.Aggregate != nil {
		selected := make([]datum.Row, 0)
		for _, key := range keys {
			row, ok := rows[key]
			if !ok {
				continue
			}
			result.Scanned++
			if Matches(row, q.Where) {
				selected = append(selected, row)
			}
		}
		result.Matched = len(selected)
		row, err := reduce(selected, *q.Aggregate)
		if err != nil {
			return result, err
		}
		result.Rows = append(result.Rows, row)
		return result, nil
	}
	// Bounded selection: every candidate is still visibility-checked and
	// counted, but only limit rows are ever retained for ordering.
	top := newTopK(rowLess(q.Order, r.Primary), limit)
	for _, key := range keys {
		row, ok := rows[key]
		if !ok {
			continue
		}
		result.Scanned++
		if Matches(row, q.Where) {
			result.Matched++
			top.offer(row)
		}
	}
	result.Truncated = result.Matched > limit
	for _, row := range top.sorted() {
		result.Rows = append(result.Rows, project(row, q.Columns))
	}
	return result, nil
}

func project(row datum.Row, columns []string) datum.Row {
	if len(columns) == 0 {
		return datum.Clone(row)
	}
	projection := datum.Row{}
	for _, name := range columns {
		projection[name] = row[name]
	}
	return projection
}

func reduce(rows []datum.Row, a Aggregate) (datum.Row, error) {
	var total int64
	var count int64
	for _, row := range rows {
		v := row[a.Column]
		if a.Column != "" && a.Column != "*" && v.Null() {
			continue
		}
		count++
		if a.Function == "sum" {
			if v.Int > 0 && total > math.MaxInt64-v.Int || v.Int < 0 && total < math.MinInt64-v.Int {
				return nil, fmt.Errorf("sum overflow")
			}
			total += v.Int
		}
	}
	if a.Function == "count" {
		return datum.Row{"count": datum.Integer(count)}, nil
	}
	if count == 0 {
		return datum.Row{"sum": datum.Value{Kind: "null"}}, nil
	}
	return datum.Row{"sum": datum.Integer(total)}, nil
}
