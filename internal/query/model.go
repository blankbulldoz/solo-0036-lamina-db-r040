package query

import "lamina/internal/datum"

type Predicate struct {
	Column string      `json:"column"`
	Op     string      `json:"op"`
	Value  datum.Value `json:"value"`
}
type Ordering struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc"`
}
type Aggregate struct {
	Function string `json:"function"`
	Column   string `json:"column,omitempty"`
}
type Request struct {
	Relation    string      `json:"relation"`
	Columns     []string    `json:"columns,omitempty"`
	Where       []Predicate `json:"where,omitempty"`
	Order       []Ordering  `json:"order,omitempty"`
	Limit       int         `json:"limit,omitempty"`
	At          *uint64     `json:"at,omitempty"`
	Transaction string      `json:"transaction,omitempty"`
	Aggregate   *Aggregate  `json:"aggregate,omitempty"`
	ForceScan   bool        `json:"force_scan,omitempty"`
}
type Plan struct {
	Access     string `json:"access"`
	Index      string `json:"index,omitempty"`
	Candidates int    `json:"candidates"`
	Snapshot   uint64 `json:"snapshot"`
	Residuals  int    `json:"residuals"`
	// Examined counts ordered-walk index entries inspected. On a completed
	// ordered plan it is the actual early-termination cost; on a plan with
	// Fallback set it is the abandoned walk's wasted work.
	Examined int `json:"examined,omitempty"`
	// Fallback records that an ordered walk exceeded its candidate budget
	// and the result was produced by the bounded-selection plan instead.
	Fallback bool `json:"fallback,omitempty"`
}
type Result struct {
	Rows      []datum.Row `json:"rows"`
	Plan      Plan        `json:"plan"`
	Scanned   int         `json:"scanned"`
	Matched   int         `json:"matched"`
	Truncated bool        `json:"truncated"`
}
