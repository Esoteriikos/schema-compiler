package schema

import (
	"encoding/json"
	"math"
	"strconv"
)

func deepEqual(a, b any) bool {
	if _, aBool := a.(bool); aBool {
		bv, ok := b.(bool)
		ab, _ := a.(bool)
		return ok && ab == bv
	}
	if af, aok := asFloat(a); aok {
		bf, bok := asFloat(b)
		return bok && af == bf
	}
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			bvChild, ok := bv[k]
			if !ok || !deepEqual(v, bvChild) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		return f, true
	case float64:
		if math.IsInf(n, 0) || math.IsNaN(n) {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, ferr := n.Float64()
			if ferr != nil || f != math.Trunc(f) || math.IsInf(f, 0) || math.IsNaN(f) {
				return 0, false
			}
			return int(f), true
		}
		if i > int64(maxInt()) || i < int64(minInt()) {
			return 0, false
		}
		return int(i), true
	case float64:
		if n != math.Trunc(n) || math.IsInf(n, 0) || math.IsNaN(n) {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func isInteger(v any) bool {
	f, ok := asFloat(v)
	return ok && f == math.Trunc(f)
}

func floatPtr(f float64) *float64 { return &f }

func intPtr(n int) *int { return &n }

func maxInt() int { return int(^uint(0) >> 1) }

func minInt() int { return -maxInt() - 1 }

func preview(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return strconv.Quote(fmtSprint(v))
	}
	if len(raw) > 80 {
		return string(raw[:77]) + "..."
	}
	return string(raw)
}

func fmtSprint(v any) string {
	return stringify(v)
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}
