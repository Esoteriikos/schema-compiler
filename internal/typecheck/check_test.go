package typecheck

import (
	"strings"
	"testing"

	"github.com/Esoteriikos/schema-compiler/internal/schema"
)

func TestCheckCompatibleModel(t *testing.T) {
	doc := mustSchema(t, `{
		"type": "object",
		"additionalProperties": false,
		"required": ["id", "owner"],
		"properties": {
			"id": {"type": "string"},
			"count": {"type": "number"},
			"owner": {"$ref": "#/$defs/Org"}
		},
		"$defs": {
			"Org": {"type": "object", "required": ["name"], "properties": {"name": {"type": "string"}}}
		}
	}`)
	models, err := ParseModels([]byte(`{
		"name": "Record",
		"fields": [
			{"name": "id", "type": "string"},
			{"name": "count", "type": "integer", "optional": true},
			{"name": "owner", "type": "object", "ref": "Org"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if diags := Check(doc, models); len(diags) != 0 {
		t.Fatal(diags)
	}
}

func TestCheckRejectsIncompatibleModel(t *testing.T) {
	doc := mustSchema(t, `{
		"type": "object",
		"additionalProperties": false,
		"required": ["id"],
		"properties": {"id": {"type": "string"}, "n": {"type": "integer"}}
	}`)
	models, err := ParseModels([]byte(`{
		"name": "Record",
		"fields": [
			{"name": "n", "type": "number"},
			{"name": "extra", "type": "boolean"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blob := join(Check(doc, models))
	for _, want := range []string{"missing required field", "not compatible", "not allowed"} {
		if !strings.Contains(blob, want) {
			t.Errorf("missing %q in\n%s", want, blob)
		}
	}
}

func TestParseModelsRejectsUnknowns(t *testing.T) {
	if _, err := ParseModels([]byte(`{"name":"A","fields":[{"name":"id","type":"int"}]}`)); err == nil {
		t.Fatal("expected unknown type")
	}
	if _, err := ParseModels([]byte(`{"name":"A","fields":[],"nope":true}`)); err == nil {
		t.Fatal("expected unknown field")
	}
}

func mustSchema(t *testing.T, raw string) *schema.Document {
	t.Helper()
	doc, err := schema.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func join(diags []schema.Diagnostic) string {
	parts := make([]string, len(diags))
	for i, d := range diags {
		parts[i] = d.Error()
	}
	return strings.Join(parts, "\n")
}
