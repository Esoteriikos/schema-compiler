package schema

import (
	"strings"
	"testing"
)

func TestParseAcceptsObjectSchema(t *testing.T) {
	doc, err := Parse([]byte(`{
		"$id": "https://example.com/person.json",
		"title": "Person",
		"type": "object",
		"required": ["name"],
		"additionalProperties": false,
		"properties": {
			"name": {"type": "string", "minLength": 1}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Root.Title != "Person" || doc.Root.ID == "" {
		t.Fatalf("root metadata = %+v", doc.Root)
	}
	if _, ok := doc.Lookup("#/properties/name"); !ok {
		t.Fatal("missing name property pointer")
	}
}

func TestParseBooleanSchemas(t *testing.T) {
	doc, err := Parse([]byte(`true`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Root.Boolean == nil || !*doc.Root.Boolean {
		t.Fatal("expected true schema")
	}
	doc, err = Parse([]byte(`false`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Root.Boolean == nil || *doc.Root.Boolean {
		t.Fatal("expected false schema")
	}
}

func TestParseRejectsInvalidSchemas(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unknown type", raw: `{"type":"bogus"}`, want: "unknown type"},
		{name: "duplicate type", raw: `{"type":["string","string"]}`, want: "duplicate type"},
		{name: "empty type", raw: `{"type":[]}`, want: "at least one"},
		{name: "required shape", raw: `{"required":"name"}`, want: "required must be an array"},
		{name: "required entry", raw: `{"required":[1]}`, want: "required entries must be strings"},
		{name: "properties shape", raw: `{"properties":[]}`, want: "properties must be an object"},
		{name: "negative length", raw: `{"minLength":-1}`, want: "minLength must be >= 0"},
		{name: "bad pattern", raw: `{"pattern":"("}`, want: "regular expression"},
		{name: "missing ref", raw: `{"$ref":"#/missing"}`, want: "unresolved $ref"},
		{name: "external ref", raw: `{"$ref":"http://example.com/schema"}`, want: "only local"},
		{name: "min above max", raw: `{"minimum":10,"maximum":1}`, want: "minimum is greater than maximum"},
		{name: "items tuple", raw: `{"items":[{"type":"string"}]}`, want: "items must be a schema"},
		{name: "enum shape", raw: `{"enum":"x"}`, want: "enum must be an array"},
		{name: "additional shape", raw: `{"additionalProperties":1}`, want: "additionalProperties"},
		{name: "unique shape", raw: `{"uniqueItems":"yes"}`, want: "uniqueItems"},
		{name: "root array", raw: `[]`, want: "schema must be an object or boolean"},
		{name: "item bounds", raw: `{"minItems":3,"maxItems":1}`, want: "minItems is greater than maxItems"},
		{name: "oneOf shape", raw: `{"oneOf":{}}`, want: "oneOf must be an array"},
		{name: "defs shape", raw: `{"$defs":[]}`, want: "$defs must be an object"},
		{name: "both def keywords", raw: `{"$defs":{},"definitions":{}}`, want: "both $defs and definitions"},
		{name: "exclusive bool", raw: `{"exclusiveMinimum":true}`, want: "exclusiveMinimum must be a number"},
		{name: "trailing", raw: `{}{}`, want: "trailing data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseResolvesLocalAndDocumentRefs(t *testing.T) {
	doc, err := Parse([]byte(`{
		"$id": "https://example.com/schema.json",
		"$defs": {
			"a/b": {"type": "string"},
			"a~b": {"type": "integer"}
		},
		"type": "object",
		"properties": {
			"x": {"$ref": "#/$defs/a~1b"},
			"y": {"$ref": "https://example.com/schema.json#/$defs/a~0b"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	x, ok := doc.Lookup("#/properties/x")
	if !ok || x.Ref != "#/$defs/a~1b" {
		t.Fatalf("x ref = %+v", x)
	}
	y, ok := doc.Lookup("#/properties/y")
	if !ok || y.Ref != "#/$defs/a~0b" {
		t.Fatalf("y ref = %+v", y)
	}
}

func TestParseAllowsRecursiveSchemaAndRejectsRefCycle(t *testing.T) {
	if _, err := Parse([]byte(`{
		"type": "object",
		"properties": {"child": {"$ref": "#"}},
		"additionalProperties": false
	}`)); err != nil {
		t.Fatal(err)
	}
	_, err := Parse([]byte(`{
		"$defs": {
			"A": {"$ref": "#/$defs/B"},
			"B": {"$ref": "#/$defs/A"}
		},
		"$ref": "#/$defs/A"
	}`))
	if err == nil || !strings.Contains(err.Error(), "cyclic $ref") {
		t.Fatalf("expected cyclic ref error, got %v", err)
	}
}
