package schema

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestPropertyConformingInstancesPass(t *testing.T) {
	const seed = 20261126
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	g := &generator{rng: rng}
	for i := 0; i < 80; i++ {
		sch, inst, typ := g.document()
		rawSchema, err := json.Marshal(sch)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := Parse(rawSchema)
		if err != nil {
			t.Fatalf("iter %d type %s parse: %v\nschema %s", i, typ, err, rawSchema)
		}
		rawInst, err := json.Marshal(inst)
		if err != nil {
			t.Fatal(err)
		}
		diags, err := doc.Validate(rawInst)
		if err != nil || len(diags) != 0 {
			t.Fatalf("iter %d type %s validate %v %v\nschema %s\ninstance %s", i, typ, err, diags, rawSchema, rawInst)
		}

		wrong := wrongTypeJSON(typ)
		diags, err = doc.Validate(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if len(diags) == 0 {
			t.Fatalf("iter %d type %s accepted wrong value %s\nschema %s", i, typ, wrong, rawSchema)
		}

		if typ == "object" {
			if req, _ := sch["required"].([]string); len(req) > 0 {
				clone := map[string]any{}
				for k, v := range inst.(map[string]any) {
					clone[k] = v
				}
				delete(clone, req[0])
				raw, _ := json.Marshal(clone)
				diags, err = doc.Validate(raw)
				if err != nil || len(diags) == 0 {
					t.Fatalf("iter %d missing required %s did not fail: %v %v", i, req[0], err, diags)
				}
			}
		}
	}
}

func TestPropertyInvalidSchemasAreRejected(t *testing.T) {
	const seed = 20261127
	rng := rand.New(rand.NewPCG(seed, seed^0x51))
	cases := invalidSchemaFixtures()
	for i := 0; i < 60; i++ {
		raw := cases[rng.IntN(len(cases))]
		_, err := Parse([]byte(raw))
		if err == nil {
			t.Fatalf("iter %d accepted invalid schema %s", i, raw)
		}
	}
}

func TestPropertyRandomBytesDoNotPanic(t *testing.T) {
	const seed = 20261128
	rng := rand.New(rand.NewPCG(seed, seed^0x22))
	for i := 0; i < 40; i++ {
		n := rng.IntN(64)
		buf := make([]byte, n)
		for j := range buf {
			buf[j] = byte(rng.IntN(256))
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("panic on %q: %v", buf, rec)
				}
			}()
			_, _ = Parse(buf)
		}()
	}
}

func invalidSchemaFixtures() []string {
	return []string{
		`{"type":"bogus"}`,
		`{"type":["string","string"]}`,
		`{"type":[]}`,
		`{"required":"name"}`,
		`{"required":[1]}`,
		`{"properties":[]}`,
		`{"minLength":-1}`,
		`{"pattern":"("}`,
		`{"$ref":"#/missing"}`,
		`{"$ref":"http://example.com/schema"}`,
		`{"minimum":10,"maximum":1}`,
		`{"items":"nope"}`,
		`{"enum":"x"}`,
		`{"additionalProperties":1}`,
		`{"uniqueItems":"yes"}`,
		`[]`,
		`{"minItems":3,"maxItems":1}`,
		`{"oneOf":{}}`,
		`{"$defs":[]}`,
		`{"$defs":{},"definitions":{}}`,
		`{"exclusiveMinimum":false}`,
		`{"minProperties":2,"maxProperties":1}`,
		`{"type":["nope"]}`,
	}
}

func wrongTypeJSON(typ string) []byte {
	switch typ {
	case "string":
		return []byte(`0`)
	case "integer", "number":
		return []byte(`"x"`)
	case "boolean":
		return []byte(`"x"`)
	case "null":
		return []byte(`0`)
	case "object":
		return []byte(`[]`)
	case "array":
		return []byte(`{}`)
	default:
		return []byte(`"x"`)
	}
}

type generator struct {
	rng *rand.Rand
}

func (g *generator) document() (map[string]any, any, string) {
	inner := g.schema(0)
	inst := g.value(inner)
	typ, _ := inner["type"].(string)
	if g.rng.IntN(3) == 0 {
		return map[string]any{
			"$defs": map[string]any{"Node": inner},
			"$ref":  "#/$defs/Node",
		}, inst, typ
	}
	return inner, inst, typ
}

func (g *generator) schema(depth int) map[string]any {
	kinds := []string{"string", "integer", "number", "boolean", "null", "object", "array"}
	if depth >= 3 {
		kinds = []string{"string", "integer", "number", "boolean", "null"}
	}
	switch kinds[g.rng.IntN(len(kinds))] {
	case "string":
		minL := g.rng.IntN(4)
		maxL := minL + g.rng.IntN(5)
		sch := map[string]any{"type": "string", "minLength": minL, "maxLength": maxL}
		if minL >= 1 && g.rng.IntN(2) == 0 {
			sch["pattern"] = "^[a-z]+$"
		}
		return sch
	case "integer":
		minV := g.rng.IntN(20)
		maxV := minV + g.rng.IntN(20)
		return map[string]any{"type": "integer", "minimum": minV, "maximum": maxV}
	case "number":
		minV := g.rng.IntN(20)
		maxV := minV + 1 + g.rng.IntN(20)
		return map[string]any{"type": "number", "minimum": minV, "maximum": maxV}
	case "boolean":
		return map[string]any{"type": "boolean"}
	case "null":
		return map[string]any{"type": "null"}
	case "object":
		n := 1 + g.rng.IntN(3)
		props := map[string]any{}
		var req []string
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("p%d", i)
			props[name] = g.schema(depth + 1)
			if g.rng.IntN(2) == 0 {
				req = append(req, name)
			}
		}
		sch := map[string]any{
			"type":                 "object",
			"properties":           props,
			"additionalProperties": false,
		}
		if len(req) > 0 {
			sch["required"] = req
		}
		return sch
	case "array":
		minI := g.rng.IntN(2)
		maxI := minI + g.rng.IntN(3)
		return map[string]any{
			"type":     "array",
			"items":    g.schema(depth + 1),
			"minItems": minI,
			"maxItems": maxI,
		}
	default:
		return map[string]any{"type": "string"}
	}
}

func (g *generator) value(sch map[string]any) any {
	switch sch["type"] {
	case "string":
		minL, _ := sch["minLength"].(int)
		maxL, _ := sch["maxLength"].(int)
		if _, ok := sch["pattern"]; ok && minL < 1 {
			minL = 1
		}
		return g.letters(minL, maxL)
	case "integer":
		minV, _ := sch["minimum"].(int)
		maxV, _ := sch["maximum"].(int)
		if maxV < minV {
			maxV = minV
		}
		return minV + g.rng.IntN(maxV-minV+1)
	case "number":
		minV, _ := sch["minimum"].(int)
		maxV, _ := sch["maximum"].(int)
		if g.rng.IntN(2) == 0 {
			return float64(minV) + 0.5
		}
		return minV + g.rng.IntN(maxV-minV+1)
	case "boolean":
		return g.rng.IntN(2) == 0
	case "null":
		return nil
	case "object":
		props := sch["properties"].(map[string]any)
		req := map[string]bool{}
		if raw, ok := sch["required"].([]string); ok {
			for _, name := range raw {
				req[name] = true
			}
		}
		out := map[string]any{}
		for i := 0; i < len(props); i++ {
			name := fmt.Sprintf("p%d", i)
			child, ok := props[name].(map[string]any)
			if !ok {
				continue
			}
			if req[name] || g.rng.IntN(2) == 0 {
				out[name] = g.value(child)
			}
		}
		return out
	case "array":
		items := sch["items"].(map[string]any)
		minI, _ := sch["minItems"].(int)
		maxI, _ := sch["maxItems"].(int)
		n := minI
		if maxI > minI {
			n = minI + g.rng.IntN(maxI-minI+1)
		}
		arr := make([]any, n)
		for i := range arr {
			arr[i] = g.value(items)
		}
		return arr
	default:
		return "x"
	}
}

func (g *generator) letters(minL, maxL int) string {
	if maxL < minL {
		maxL = minL
	}
	n := minL
	if maxL > minL {
		n = minL + g.rng.IntN(maxL-minL+1)
	}
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte('a' + g.rng.IntN(26))
	}
	return string(buf)
}

func TestPropertyCorruptTypeMessageIsUseful(t *testing.T) {
	doc := mustParse(t, `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`)
	diags, err := doc.Validate([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 || !strings.Contains(diags[0].Error(), "expected object") {
		t.Fatal(diags)
	}
}
