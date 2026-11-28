// Package typecheck compares structural models with a compiled JSON Schema.
package typecheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Esoteriikos/schema-compiler/internal/schema"
)

// Field is one named slot in a model.
type Field struct {
	Name     string
	Type     string
	Ref      string
	Optional bool
	Items    *Field
	Fields   []Field
}

// Model is a structural type checked against a schema target.
type Model struct {
	Name   string
	Target string
	Fields []Field
}

var modelTypes = map[string]bool{
	"": true, "string": true, "number": true, "integer": true,
	"boolean": true, "object": true, "array": true, "null": true,
}

// ParseModels reads one model object or a {"models":[...]} bundle.
func ParseModels(data []byte) ([]Model, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var top map[string]any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("invalid model JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	if _, ok := top["models"]; ok {
		for key := range top {
			if key != "models" {
				return nil, fmt.Errorf("unknown model bundle field %q", key)
			}
		}
		arr, ok := top["models"].([]any)
		if !ok {
			return nil, fmt.Errorf("models must be an array")
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("models must not be empty")
		}
		out := make([]Model, 0, len(arr))
		seen := map[string]bool{}
		for _, item := range arr {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("each model must be an object")
			}
			model, err := parseModel(obj)
			if err != nil {
				return nil, err
			}
			if seen[model.Name] {
				return nil, fmt.Errorf("duplicate model %q", model.Name)
			}
			seen[model.Name] = true
			out = append(out, model)
		}
		return out, nil
	}
	if _, ok := top["fields"]; ok {
		model, err := parseModel(top)
		if err != nil {
			return nil, err
		}
		return []Model{model}, nil
	}
	return nil, fmt.Errorf("model file needs \"models\" or \"fields\"")
}

func parseModel(obj map[string]any) (Model, error) {
	for key := range obj {
		switch key {
		case "name", "target", "fields", "description":
		default:
			return Model{}, fmt.Errorf("unknown model field %q", key)
		}
	}
	name, _ := obj["name"].(string)
	if strings.TrimSpace(name) == "" {
		return Model{}, fmt.Errorf("model name is required")
	}
	if _, ok := obj["name"].(string); !ok {
		return Model{}, fmt.Errorf("model name must be a string")
	}
	target, err := optionalString(obj, "target")
	if err != nil {
		return Model{}, err
	}
	fields, err := parseFields(obj["fields"], true)
	if err != nil {
		return Model{}, fmt.Errorf("model %s: %w", name, err)
	}
	return Model{Name: name, Target: target, Fields: fields}, nil
}

func parseFields(v any, requireName bool) ([]Field, error) {
	if v == nil {
		return nil, fmt.Errorf("fields are required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("fields must be an array")
	}
	out := make([]Field, 0, len(arr))
	seen := map[string]bool{}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field must be an object")
		}
		field, err := parseField(obj, requireName)
		if err != nil {
			return nil, err
		}
		if field.Name != "" {
			if seen[field.Name] {
				return nil, fmt.Errorf("duplicate field %q", field.Name)
			}
			seen[field.Name] = true
		}
		out = append(out, field)
	}
	return out, nil
}

func parseField(obj map[string]any, requireName bool) (Field, error) {
	for key := range obj {
		switch key {
		case "name", "type", "ref", "optional", "items", "fields", "description":
		default:
			return Field{}, fmt.Errorf("unknown field property %q", key)
		}
	}
	name, err := optionalString(obj, "name")
	if err != nil {
		return Field{}, err
	}
	if requireName && name == "" {
		return Field{}, fmt.Errorf("field name is required")
	}
	typ, err := optionalString(obj, "type")
	if err != nil {
		return Field{}, err
	}
	if !modelTypes[typ] {
		return Field{}, fmt.Errorf("unknown field type %q", typ)
	}
	ref, err := optionalString(obj, "ref")
	if err != nil {
		return Field{}, err
	}
	optional := false
	if raw, ok := obj["optional"]; ok {
		b, ok := raw.(bool)
		if !ok {
			return Field{}, fmt.Errorf("optional must be a boolean")
		}
		optional = b
	}
	var items *Field
	if raw, ok := obj["items"]; ok {
		obj, ok := raw.(map[string]any)
		if !ok {
			return Field{}, fmt.Errorf("items must be an object")
		}
		item, err := parseField(obj, false)
		if err != nil {
			return Field{}, fmt.Errorf("items: %w", err)
		}
		items = &item
	}
	var nested []Field
	if _, ok := obj["fields"]; ok {
		nested, err = parseFields(obj["fields"], true)
		if err != nil {
			return Field{}, err
		}
	}
	return Field{Name: name, Type: typ, Ref: ref, Optional: optional, Items: items, Fields: nested}, nil
}

func optionalString(obj map[string]any, key string) (string, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return "", nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return text, nil
}

// Check reports where a model cannot inhabit the schema.
func Check(doc *schema.Document, models []Model) []schema.Diagnostic {
	if doc == nil || doc.Root == nil {
		return []schema.Diagnostic{{Message: "missing schema"}}
	}
	var diags []schema.Diagnostic
	for _, model := range models {
		target, err := resolveTarget(doc, model.Target)
		if err != nil {
			diags = append(diags, schema.Diagnostic{Path: model.Name, Message: err.Error()})
			continue
		}
		checkObject(doc, target, model.Fields, model.Name, &diags)
	}
	schema.SortDiagnostics(diags)
	return diags
}

func resolveTarget(doc *schema.Document, target string) (*schema.Schema, error) {
	target = strings.TrimSpace(target)
	if target == "" || target == "#" {
		return doc.Root, nil
	}
	if strings.HasPrefix(target, "#/") || target == "#" {
		s, ok := doc.Lookup(target)
		if !ok {
			return nil, fmt.Errorf("unknown schema pointer %s", target)
		}
		return s, nil
	}
	for _, prefix := range []string{"#/$defs/", "#/definitions/"} {
		ptr := prefix + schema.EscapePointer(target)
		if s, ok := doc.Lookup(ptr); ok {
			return s, nil
		}
	}
	return nil, fmt.Errorf("unknown model target %s", target)
}

func checkObject(doc *schema.Document, sch *schema.Schema, fields []Field, path string, diags *[]schema.Diagnostic) {
	eff, err := doc.Deref(sch)
	if err != nil {
		*diags = append(*diags, schema.Diagnostic{Path: path, Message: err.Error()})
		return
	}
	if eff.Boolean != nil {
		if !*eff.Boolean {
			*diags = append(*diags, schema.Diagnostic{Path: path, Message: "model is rejected by false schema"})
		}
		return
	}
	if len(eff.Types) > 0 && !contains(eff.Types, "object") {
		*diags = append(*diags, schema.Diagnostic{Path: path, Message: "model is an object but schema expects " + strings.Join(eff.Types, " or ")})
		return
	}
	byName := map[string]Field{}
	for _, field := range fields {
		if _, ok := byName[field.Name]; ok {
			*diags = append(*diags, schema.Diagnostic{Path: path + "." + field.Name, Message: "duplicate field"})
			continue
		}
		byName[field.Name] = field
		ps, ok := eff.Properties[field.Name]
		fieldPath := path + "." + field.Name
		if !ok {
			if eff.Additional != nil && eff.Additional.Forbidden {
				*diags = append(*diags, schema.Diagnostic{Path: fieldPath, Message: "additional property is not allowed"})
				continue
			}
			if eff.Additional != nil && eff.Additional.Schema != nil {
				checkField(doc, eff.Additional.Schema, field, fieldPath, diags)
			}
			continue
		}
		checkField(doc, ps, field, fieldPath, diags)
	}
	for _, name := range eff.Required {
		field, ok := byName[name]
		if !ok || field.Optional {
			*diags = append(*diags, schema.Diagnostic{Path: path + "." + name, Message: "missing required field"})
		}
	}
}

func checkField(doc *schema.Document, prop *schema.Schema, field Field, path string, diags *[]schema.Diagnostic) {
	eff, err := doc.Deref(prop)
	if err != nil {
		*diags = append(*diags, schema.Diagnostic{Path: path, Message: err.Error()})
		return
	}
	if field.Type != "" && !compatible(field.Type, eff.Types) {
		*diags = append(*diags, schema.Diagnostic{Path: path, Message: fmt.Sprintf("type %s is not compatible with schema type %s", field.Type, typeList(eff.Types))})
	}
	if field.Ref != "" {
		want, err := resolveTarget(doc, field.Ref)
		if err != nil {
			*diags = append(*diags, schema.Diagnostic{Path: path, Message: err.Error()})
		} else if prop.Ref != want.Pointer && eff.Pointer != want.Pointer {
			*diags = append(*diags, schema.Diagnostic{Path: path, Message: fmt.Sprintf("ref %s does not match schema", field.Ref)})
		}
	}
	if len(field.Fields) > 0 || (field.Type == "object" && field.Ref == "") {
		checkObject(doc, prop, field.Fields, path, diags)
	}
	if field.Items != nil {
		if eff.Items == nil {
			*diags = append(*diags, schema.Diagnostic{Path: path, Message: "schema does not declare items"})
		} else {
			checkField(doc, eff.Items, *field.Items, path+"[]", diags)
		}
	}
}

func compatible(modelType string, schemaTypes []string) bool {
	if modelType == "" || len(schemaTypes) == 0 {
		return true
	}
	for _, typ := range schemaTypes {
		if typ == modelType || (modelType == "integer" && typ == "number") {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func typeList(types []string) string {
	if len(types) == 0 {
		return "unspecified"
	}
	cp := append([]string(nil), types...)
	sort.Strings(cp)
	return strings.Join(cp, "|")
}
