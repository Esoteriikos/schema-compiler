package schema

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, raw string) *Document {
	t.Helper()
	doc, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestValidateObject(t *testing.T) {
	doc := mustParse(t, `{
		"type": "object",
		"required": ["name"],
		"additionalProperties": false,
		"properties": {
			"name": {"type": "string", "minLength": 1, "pattern": "^[A-Z]"},
			"age": {"type": "integer", "minimum": 0, "maximum": 120},
			"score": {"type": "number", "exclusiveMinimum": 0, "exclusiveMaximum": 1},
			"role": {"type": "string", "enum": ["admin", "user"]},
			"tags": {"type": "array", "items": {"type": "string"}, "uniqueItems": true, "minItems": 1}
		}
	}`)
	diags, err := doc.Validate([]byte(`{
		"name": "Ada",
		"age": 36,
		"score": 0.5,
		"role": "admin",
		"tags": ["a", "b"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatal(diags)
	}

	diags, err = doc.Validate([]byte(`{
		"age": 1.5,
		"score": 0,
		"role": "root",
		"tags": ["a", "a"],
		"extra": true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blob := joinDiags(diags)
	for _, want := range []string{
		"required property is missing",
		"expected integer",
		"not above exclusiveMinimum",
		"value is not in enum",
		"duplicate item",
		"additional property is not allowed",
	} {
		if !strings.Contains(blob, want) {
			t.Errorf("missing %q in\n%s", want, blob)
		}
	}
}

func TestValidateRefSiblingAndFormats(t *testing.T) {
	doc := mustParse(t, `{
		"$defs": {"Name": {"type": "string"}},
		"properties": {
			"name": {"$ref": "#/$defs/Name", "minLength": 3},
			"email": {"type": "string", "format": "email"},
			"id": {"type": "string", "format": "uuid"},
			"when": {"type": "string", "format": "date-time"},
			"site": {"type": "string", "format": "uri"}
		},
		"required": ["name", "email", "id", "when", "site"]
	}`)
	diags, err := doc.Validate([]byte(`{
		"name": "Ada",
		"email": "ada@example.com",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"when": "2026-11-26T15:04:05Z",
		"site": "https://example.com/a"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	diags, err = doc.Validate([]byte(`{
		"name": "Ad",
		"email": "not-an-email",
		"id": "nope",
		"when": "yesterday",
		"site": "example.com"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	blob := joinDiags(diags)
	for _, want := range []string{"minLength", "email", "uuid", "date-time", "uri"} {
		if !strings.Contains(blob, want) {
			t.Errorf("missing %q in\n%s", want, blob)
		}
	}
}

func TestValidateCombinatorsConstAndBounds(t *testing.T) {
	doc := mustParse(t, `{
		"oneOf": [{"type": "string", "const": "a"}, {"type": "integer", "minimum": 1}],
		"anyOf": [{"type": "string"}, {"minimum": 0}],
		"allOf": [{"not": {"const": "z"}}]
	}`)
	for _, raw := range []string{`"a"`, `2`} {
		diags, err := doc.Validate([]byte(raw))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%s: %v %v", raw, err, diags)
		}
	}
	diags, err := doc.Validate([]byte(`"z"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 {
		t.Fatal("expected const/not failure")
	}
	diags, err = doc.Validate([]byte(`true`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joinDiags(diags), "oneOf") {
		t.Fatal(diags)
	}
}

func TestValidateIntegerAcceptsWholeNumbers(t *testing.T) {
	doc := mustParse(t, `{"type":"integer"}`)
	for _, raw := range []string{`1`, `1.0`, `-4`} {
		diags, err := doc.Validate([]byte(raw))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%s: %v %v", raw, err, diags)
		}
	}
	diags, err := doc.Validate([]byte(`1.5`))
	if err != nil || len(diags) == 0 {
		t.Fatalf("1.5 should fail, diags=%v err=%v", diags, err)
	}
}

func TestValidateFalseSchemaAndBadJSON(t *testing.T) {
	doc := mustParse(t, `false`)
	diags, err := doc.Validate([]byte(`1`))
	if err != nil || len(diags) == 0 {
		t.Fatal(err, diags)
	}
	if _, err := doc.Validate([]byte(`{`)); err == nil {
		t.Fatal("expected json error")
	}
}

func TestValidateRecursiveTree(t *testing.T) {
	doc := mustParse(t, `{
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"name": {"type": "string"},
			"child": {"$ref": "#"}
		},
		"required": ["name"]
	}`)
	diags, err := doc.Validate([]byte(`{"name":"a","child":{"name":"b"}}`))
	if err != nil || len(diags) != 0 {
		t.Fatal(err, diags)
	}
	diags, err = doc.Validate([]byte(`{"name":"a","child":{"name":1}}`))
	if err != nil || len(diags) == 0 {
		t.Fatal(err, diags)
	}
}

func joinDiags(d []Diagnostic) string {
	parts := make([]string, len(d))
	for i, item := range d {
		parts[i] = item.Error()
	}
	return strings.Join(parts, "\n")
}
