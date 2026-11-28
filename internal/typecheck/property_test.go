package typecheck

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/Esoteriikos/schema-compiler/internal/schema"
)

func TestPropertyModelsMatchGeneratedSchemas(t *testing.T) {
	const seed = 20261126
	rng := rand.New(rand.NewPCG(seed, seed^0xabc))
	types := []string{"string", "integer", "number", "boolean"}
	for i := 0; i < 40; i++ {
		n := 1 + rng.IntN(4)
		props := map[string]any{}
		required := make([]string, n)
		fields := make([]any, n)
		for f := 0; f < n; f++ {
			name := fmt.Sprintf("f%d", f)
			typ := types[rng.IntN(len(types))]
			props[name] = map[string]any{"type": typ}
			required[f] = name
			fields[f] = map[string]any{"name": name, "type": typ}
		}
		rawSchema, err := json.Marshal(map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties":           props,
		})
		if err != nil {
			t.Fatal(err)
		}
		doc, err := schema.Parse(rawSchema)
		if err != nil {
			t.Fatal(err)
		}
		rawModel, err := json.Marshal(map[string]any{"name": "M", "fields": fields})
		if err != nil {
			t.Fatal(err)
		}
		models, err := ParseModels(rawModel)
		if err != nil {
			t.Fatal(err)
		}
		if diags := Check(doc, models); len(diags) != 0 {
			t.Fatalf("iter %d valid model failed: %v\n%s", i, diags, rawSchema)
		}
		first := map[string]any{}
		for k, v := range fields[0].(map[string]any) {
			first[k] = v
		}
		first["type"] = badType(first["type"].(string))
		fields[0] = first
		rawBad, err := json.Marshal(map[string]any{"name": "M", "fields": fields})
		if err != nil {
			t.Fatal(err)
		}
		badModels, err := ParseModels(rawBad)
		if err != nil {
			t.Fatal(err)
		}
		if diags := Check(doc, badModels); len(diags) == 0 {
			t.Fatalf("iter %d incompatible model was accepted\n%s", i, rawBad)
		}
	}
}

func badType(typ string) string {
	if typ == "string" {
		return "boolean"
	}
	return "string"
}
