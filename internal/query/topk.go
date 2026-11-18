package query

import (
	"lamina/internal/datum"
	"sort"
)

// rowLess is the single total order shared by full sorts, bounded selection
// and the ordered index walk: explicit ordering terms first, primary key
// ascending as the final tiebreak. Primary keys are unique, so the order is
// total and every strategy emits byte-identical row sequences.
func rowLess(order []Ordering, primary string) func(a, b datum.Row) bool {
	return func(a, b datum.Row) bool {
		for _, o := range order {
			n := datum.Order(a[o.Column], b[o.Column])
			if n != 0 {
				if o.Desc {
					return n > 0
				}
				return n < 0
			}
		}
		return datum.Order(a[primary], b[primary]) < 0
	}
}

// topK keeps the k smallest rows under less using O(k) space: a max-heap
// whose root is the worst row currently retained. It replaces the previous
// collect-everything-then-sort selection so ORDER BY/LIMIT queries never
// materialize more than the requested result plus one candidate slot.
type topK struct {
	less func(a, b datum.Row) bool
	rows []datum.Row
	cap  int
}

func newTopK(less func(a, b datum.Row) bool, cap int) *topK {
	return &topK{less: less, cap: cap}
}

func (t *topK) offer(row datum.Row) {
	if t.cap <= 0 {
		return
	}
	if len(t.rows) < t.cap {
		t.rows = append(t.rows, row)
		t.up(len(t.rows) - 1)
		return
	}
	if t.less(row, t.rows[0]) {
		t.rows[0] = row
		t.down(0)
	}
}

func (t *topK) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !t.less(t.rows[parent], t.rows[i]) {
			return
		}
		t.rows[parent], t.rows[i] = t.rows[i], t.rows[parent]
		i = parent
	}
}

func (t *topK) down(i int) {
	for {
		worst := i
		for _, child := range [2]int{2*i + 1, 2*i + 2} {
			if child < len(t.rows) && t.less(t.rows[worst], t.rows[child]) {
				worst = child
			}
		}
		if worst == i {
			return
		}
		t.rows[worst], t.rows[i] = t.rows[i], t.rows[worst]
		i = worst
	}
}

// reset empties the structure and retargets the capacity; used by the ordered
// walk where each same-value group needs only the remaining row count.
func (t *topK) reset(cap int) {
	t.rows = t.rows[:0]
	t.cap = cap
}

// sorted returns the retained rows in final ascending output order. The
// comparator is total, so this equals the prefix of a full sort.
func (t *topK) sorted() []datum.Row {
	out := make([]datum.Row, len(t.rows))
	copy(out, t.rows)
	sort.Slice(out, func(i, j int) bool { return t.less(out[i], out[j]) })
	return out
}
