package document

import (
	"github.com/google/uuid"
	"sort"
)

// AlignIDs follows SequenceMatcher's matching blocks and positional replacements.
// Unchanged blocks retain identity even when blocks are inserted before them.
func AlignIDs(old []Doc, oldIDs []string, next []Doc) []string {
	ids := append([]string{}, oldIDs...)
	for len(ids) < len(old) {
		ids = append(ids, uuid.NewString())
	}
	a := make([]string, len(old))
	b := make([]string, len(next))
	for i, n := range old {
		a[i] = string(serialize(n))
	}
	for i, n := range next {
		b[i] = string(serialize(n))
	}
	positions := map[string][]int{}
	for i, s := range b {
		positions[s] = append(positions[s], i)
	}
	if len(b) >= 200 {
		for s, ns := range positions {
			if len(ns) > len(b)/100+1 {
				delete(positions, s)
			}
		}
	}
	type match struct{ i, j, n int }
	matches := []match{}
	var find func(int, int, int, int)
	find = func(alo, ahi, blo, bhi int) {
		best := match{alo, blo, 0}
		previous := map[int]int{}
		for i := alo; i < ahi; i++ {
			current := map[int]int{}
			for _, j := range positions[a[i]] {
				if j < blo {
					continue
				}
				if j >= bhi {
					break
				}
				size := previous[j-1] + 1
				current[j] = size
				if size > best.n {
					best = match{i - size + 1, j - size + 1, size}
				}
			}
			previous = current
		}
		for best.i > alo && best.j > blo && a[best.i-1] == b[best.j-1] {
			best.i--
			best.j--
			best.n++
		}
		for best.i+best.n < ahi && best.j+best.n < bhi && a[best.i+best.n] == b[best.j+best.n] {
			best.n++
		}
		if best.n > 0 {
			if alo < best.i && blo < best.j {
				find(alo, best.i, blo, best.j)
			}
			matches = append(matches, best)
			if best.i+best.n < ahi && best.j+best.n < bhi {
				find(best.i+best.n, ahi, best.j+best.n, bhi)
			}
		}
	}
	find(0, len(a), 0, len(b))
	sort.Slice(matches, func(i, j int) bool { return matches[i].i < matches[j].i })
	matches = append(matches, match{len(a), len(b), 0})
	out := make([]string, len(next))
	ai, bi := 0, 0
	for _, m := range matches {
		if ai < m.i && bi < m.j {
			n := min(m.i-ai, m.j-bi)
			for k := 0; k < n; k++ {
				out[bi+k] = ids[ai+k]
			}
		}
		for k := 0; k < m.n; k++ {
			out[m.j+k] = ids[m.i+k]
		}
		ai = m.i + m.n
		bi = m.j + m.n
	}
	for i := range out {
		if out[i] == "" {
			out[i] = uuid.NewString()
		}
	}
	return out
}
func ApplyOperations(d Doc, oldIDs []string, ops []Operation, v AttachmentValidator) (Doc, []string, error) {
	if len(ops) < 1 || len(ops) > 100 {
		return nil, nil, &Error{422, "invalid_operation", "每次需要 1 到 100 个块操作。"}
	}
	ns := nodes(clone(d).(Doc))
	ids := append([]string{}, oldIDs...)
	if len(ids) != len(ns) {
		ids = AlignIDs(ns, ids, ns)
	}
	index := func(id string) int {
		for i, v := range ids {
			if v == id {
				return i
			}
		}
		return -1
	}
	for _, op := range ops {
		switch op.Op {
		case "insert":
			if op.BlockID != nil || op.Content == nil {
				return nil, nil, &Error{422, "invalid_operation", "插入需要 content 和可选 after_id。"}
			}
			at := 0
			if op.AfterID != nil {
				pos := index(*op.AfterID)
				if pos < 0 {
					return nil, nil, &Error{422, "unknown_block", "目标块不存在。"}
				}
				at = pos + 1
			}
			ns = append(ns, nil)
			copy(ns[at+1:], ns[at:])
			ns[at] = clone(op.Content).(Doc)
			ids = append(ids, "")
			copy(ids[at+1:], ids[at:])
			ids[at] = uuid.NewString()
		case "replace", "delete":
			at := -1
			if op.BlockID != nil {
				at = index(*op.BlockID)
			}
			if at < 0 || op.AfterID != nil {
				return nil, nil, &Error{422, "unknown_block", "目标块不存在或操作参数不正确。"}
			}
			if op.Op == "delete" {
				if op.Content != nil {
					return nil, nil, &Error{422, "invalid_operation", "删除不能携带正文。"}
				}
				ns = append(ns[:at], ns[at+1:]...)
				ids = append(ids[:at], ids[at+1:]...)
			} else {
				if op.Content == nil {
					return nil, nil, &Error{422, "invalid_operation", "替换需要 content。"}
				}
				ns[at] = clone(op.Content).(Doc)
			}
		default:
			return nil, nil, &Error{422, "invalid_operation", "不支持的块操作。"}
		}
	}
	clean, e := Validate(Doc{"type": "doc", "content": anyNodes(ns)}, v)
	if e != nil {
		return nil, nil, e
	}
	return clean, ids, nil
}
