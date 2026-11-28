package schema

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"type":"string"}`))
	f.Add([]byte(`{"type":`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"$ref":"#/$defs/A","$defs":{"A":{"type":"integer"}}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("panic: %v", rec)
			}
		}()
		_, _ = Parse(data)
	})
}

func FuzzValidate(f *testing.F) {
	f.Add([]byte(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`), []byte(`{"n":1}`))
	f.Add([]byte(`{"type":"string","minLength":1}`), []byte(`""`))
	f.Fuzz(func(t *testing.T, schemaJSON, instance []byte) {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("panic: %v", rec)
			}
		}()
		doc, err := Parse(schemaJSON)
		if err != nil {
			return
		}
		_, _ = doc.Validate(instance)
	})
}
