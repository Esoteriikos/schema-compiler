package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	knownTypes = map[string]bool{
		"object": true, "array": true, "string": true, "number": true,
		"integer": true, "boolean": true, "null": true,
	}
)

// Parse compiles a JSON Schema document.
// Only local $ref fragments are accepted. Cycles, broken pointers, and
// keywords whose JSON type is wrong are rejected.
func Parse(data []byte) (*Document, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON value")
	}

	var diags []Diagnostic
	root := compile(raw, "#", &diags)
	doc := &Document{Root: root, index: map[string]*Schema{}}
	indexSchema(root, doc.index)
	bindRefs(doc, root, &diags)
	findCycles(doc, &diags)
	if len(diags) > 0 {
		SortDiagnostics(diags)
		return nil, ErrorList(diags)
	}
	return doc, nil
}

func compile(v any, ptr string, diags *[]Diagnostic) *Schema {
	switch t := v.(type) {
	case bool:
		b := t
		return &Schema{Pointer: ptr, Boolean: &b}
	case map[string]any:
		return compileObject(t, ptr, diags)
	default:
		fail(diags, ptr, "schema must be an object or boolean")
		return &Schema{Pointer: ptr}
	}
}

func compileObject(obj map[string]any, ptr string, diags *[]Diagnostic) *Schema {
	s := &Schema{Pointer: ptr}
	defKey := ""
	if _, ok := obj["$defs"]; ok {
		defKey = "$defs"
	}
	if _, ok := obj["definitions"]; ok {
		if defKey != "" {
			fail(diags, ptr, "schema cannot set both $defs and definitions")
		} else {
			defKey = "definitions"
		}
	}

	for key, val := range obj {
		switch key {
		case "$id":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "$id must be a string")
				continue
			}
			s.ID = text
		case "$schema":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "$schema must be a string")
				continue
			}
			s.SchemaURI = text
		case "title":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "title must be a string")
				continue
			}
			s.Title = text
		case "description":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "description must be a string")
				continue
			}
			s.Description = text
		case "$ref":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "$ref must be a string")
				continue
			}
			if strings.TrimSpace(text) == "" {
				fail(diags, ChildPointer(ptr, key), "$ref must not be empty")
				continue
			}
			s.Ref = text
		case "type":
			compileType(val, ChildPointer(ptr, key), s, diags)
		case "properties":
			props, ok := val.(map[string]any)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "properties must be an object")
				continue
			}
			s.Properties = make(map[string]*Schema, len(props))
			base := ChildPointer(ptr, "properties")
			for name, child := range props {
				s.Properties[name] = compile(child, ChildPointer(base, name), diags)
			}
		case "required":
			compileRequired(val, ChildPointer(ptr, key), s, diags)
		case "additionalProperties":
			compileAdditional(val, ChildPointer(ptr, key), s, diags)
		case "minProperties":
			n, ok := nonNegative(val, ChildPointer(ptr, key), "minProperties", diags)
			if ok {
				s.MinProperties = intPtr(n)
			}
		case "maxProperties":
			n, ok := nonNegative(val, ChildPointer(ptr, key), "maxProperties", diags)
			if ok {
				s.MaxProperties = intPtr(n)
			}
		case "items":
			if _, isArr := val.([]any); isArr {
				fail(diags, ChildPointer(ptr, key), "items must be a schema")
				continue
			}
			s.Items = compile(val, ChildPointer(ptr, "items"), diags)
		case "minItems":
			n, ok := nonNegative(val, ChildPointer(ptr, key), "minItems", diags)
			if ok {
				s.MinItems = intPtr(n)
			}
		case "maxItems":
			n, ok := nonNegative(val, ChildPointer(ptr, key), "maxItems", diags)
			if ok {
				s.MaxItems = intPtr(n)
			}
		case "uniqueItems":
			b, ok := val.(bool)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "uniqueItems must be a boolean")
				continue
			}
			s.UniqueItems = b
		case "minimum":
			f, ok := asFloat(val)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "minimum must be a number")
				continue
			}
			s.Minimum = floatPtr(f)
		case "maximum":
			f, ok := asFloat(val)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "maximum must be a number")
				continue
			}
			s.Maximum = floatPtr(f)
		case "exclusiveMinimum":
			f, ok := asFloat(val)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "exclusiveMinimum must be a number")
				continue
			}
			s.ExclusiveMinimum = floatPtr(f)
		case "exclusiveMaximum":
			f, ok := asFloat(val)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "exclusiveMaximum must be a number")
				continue
			}
			s.ExclusiveMaximum = floatPtr(f)
		case "minLength":
			n, ok := nonNegative(val, ChildPointer(ptr, key), "minLength", diags)
			if ok {
				s.MinLength = intPtr(n)
			}
		case "maxLength":
			n, ok := nonNegative(val, ChildPointer(ptr, key), "maxLength", diags)
			if ok {
				s.MaxLength = intPtr(n)
			}
		case "pattern":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "pattern must be a string")
				continue
			}
			re, err := regexp.Compile(text)
			if err != nil {
				fail(diags, ChildPointer(ptr, key), "pattern is not a valid regular expression")
				continue
			}
			s.Pattern = text
			s.pattern = re
		case "format":
			text, ok := val.(string)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "format must be a string")
				continue
			}
			s.Format = text
		case "enum":
			arr, ok := val.([]any)
			if !ok {
				fail(diags, ChildPointer(ptr, key), "enum must be an array")
				continue
			}
			s.HasEnum = true
			s.Enum = append([]any(nil), arr...)
		case "const":
			s.HasConst = true
			s.Const = val
		case "oneOf", "anyOf", "allOf":
			compileSchemaArray(key, val, ptr, s, diags)
		case "not":
			s.Not = compile(val, ChildPointer(ptr, "not"), diags)
		case "$defs", "definitions":
			if key != defKey {
				continue
			}
			defs, ok := val.(map[string]any)
			if !ok {
				fail(diags, ChildPointer(ptr, key), key+" must be an object")
				continue
			}
			s.Defs = make(map[string]*Schema, len(defs))
			base := ChildPointer(ptr, key)
			for name, child := range defs {
				s.Defs[name] = compile(child, ChildPointer(base, name), diags)
			}
		default:
			// Unknown keywords are annotations.
		}
	}
	checkBounds(s, ptr, diags)
	return s
}

func compileType(val any, ptr string, s *Schema, diags *[]Diagnostic) {
	switch t := val.(type) {
	case string:
		if !knownTypes[t] {
			fail(diags, ptr, fmt.Sprintf("unknown type %q", t))
			return
		}
		s.Types = []string{t}
	case []any:
		if len(t) == 0 {
			fail(diags, ptr, "type must list at least one type")
			return
		}
		seen := map[string]bool{}
		for _, item := range t {
			name, ok := item.(string)
			if !ok {
				fail(diags, ptr, "type entries must be strings")
				continue
			}
			if !knownTypes[name] {
				fail(diags, ptr, fmt.Sprintf("unknown type %q", name))
				continue
			}
			if seen[name] {
				fail(diags, ptr, fmt.Sprintf("duplicate type %q", name))
				continue
			}
			seen[name] = true
			s.Types = append(s.Types, name)
		}
	default:
		fail(diags, ptr, "type must be a string or array of strings")
	}
}

func compileRequired(val any, ptr string, s *Schema, diags *[]Diagnostic) {
	arr, ok := val.([]any)
	if !ok {
		fail(diags, ptr, "required must be an array")
		return
	}
	seen := map[string]bool{}
	for _, item := range arr {
		name, ok := item.(string)
		if !ok {
			fail(diags, ptr, "required entries must be strings")
			continue
		}
		if seen[name] {
			fail(diags, ptr, fmt.Sprintf("duplicate required property %q", name))
			continue
		}
		seen[name] = true
		s.Required = append(s.Required, name)
	}
}

func compileAdditional(val any, ptr string, s *Schema, diags *[]Diagnostic) {
	if b, ok := val.(bool); ok {
		if !b {
			s.Additional = &Additional{Forbidden: true}
		}
		return
	}
	if _, isArr := val.([]any); isArr {
		fail(diags, ptr, "additionalProperties must be a boolean or schema")
		return
	}
	if _, ok := val.(map[string]any); !ok {
		fail(diags, ptr, "additionalProperties must be a boolean or schema")
		return
	}
	s.Additional = &Additional{Schema: compile(val, ptr, diags)}
}

func compileSchemaArray(key string, val any, ptr string, s *Schema, diags *[]Diagnostic) {
	arr, ok := val.([]any)
	if !ok {
		fail(diags, ChildPointer(ptr, key), key+" must be an array of schemas")
		return
	}
	out := make([]*Schema, 0, len(arr))
	base := ChildPointer(ptr, key)
	for i, item := range arr {
		out = append(out, compile(item, fmt.Sprintf("%s/%d", base, i), diags))
	}
	switch key {
	case "oneOf":
		s.OneOf = out
	case "anyOf":
		s.AnyOf = out
	case "allOf":
		s.AllOf = out
	}
}

func checkBounds(s *Schema, ptr string, diags *[]Diagnostic) {
	if s.Minimum != nil && s.Maximum != nil && *s.Minimum > *s.Maximum {
		fail(diags, ptr, "minimum is greater than maximum")
	}
	if s.ExclusiveMinimum != nil && s.ExclusiveMaximum != nil && *s.ExclusiveMinimum > *s.ExclusiveMaximum {
		fail(diags, ptr, "exclusiveMinimum is greater than exclusiveMaximum")
	}
	if s.MinLength != nil && s.MaxLength != nil && *s.MinLength > *s.MaxLength {
		fail(diags, ptr, "minLength is greater than maxLength")
	}
	if s.MinItems != nil && s.MaxItems != nil && *s.MinItems > *s.MaxItems {
		fail(diags, ptr, "minItems is greater than maxItems")
	}
	if s.MinProperties != nil && s.MaxProperties != nil && *s.MinProperties > *s.MaxProperties {
		fail(diags, ptr, "minProperties is greater than maxProperties")
	}
}

func nonNegative(v any, ptr, key string, diags *[]Diagnostic) (int, bool) {
	n, ok := asInt(v)
	if !ok {
		fail(diags, ptr, key+" must be an integer")
		return 0, false
	}
	if n < 0 {
		fail(diags, ptr, key+" must be >= 0")
		return 0, false
	}
	return n, true
}

func fail(diags *[]Diagnostic, path, msg string) {
	*diags = append(*diags, Diagnostic{Path: path, Message: msg})
}

func indexSchema(s *Schema, idx map[string]*Schema) {
	if s == nil || s.Pointer == "" {
		return
	}
	idx[s.Pointer] = s
	for _, child := range s.Properties {
		indexSchema(child, idx)
	}
	if s.Additional != nil {
		indexSchema(s.Additional.Schema, idx)
	}
	indexSchema(s.Items, idx)
	for _, child := range s.Defs {
		indexSchema(child, idx)
	}
	for _, child := range s.OneOf {
		indexSchema(child, idx)
	}
	for _, child := range s.AnyOf {
		indexSchema(child, idx)
	}
	for _, child := range s.AllOf {
		indexSchema(child, idx)
	}
	indexSchema(s.Not, idx)
}

func bindRefs(doc *Document, s *Schema, diags *[]Diagnostic) {
	if s == nil {
		return
	}
	if s.Ref != "" {
		ptr, err := localRef(s.Ref, doc.Root.ID)
		if err != nil {
			fail(diags, s.Pointer, err.Error())
		} else if _, ok := doc.index[ptr]; !ok {
			fail(diags, s.Pointer, "unresolved $ref "+ptr)
		} else {
			s.Ref = ptr
		}
	}
	for _, child := range s.Properties {
		bindRefs(doc, child, diags)
	}
	if s.Additional != nil {
		bindRefs(doc, s.Additional.Schema, diags)
	}
	bindRefs(doc, s.Items, diags)
	for _, child := range s.Defs {
		bindRefs(doc, child, diags)
	}
	for _, child := range s.OneOf {
		bindRefs(doc, child, diags)
	}
	for _, child := range s.AnyOf {
		bindRefs(doc, child, diags)
	}
	for _, child := range s.AllOf {
		bindRefs(doc, child, diags)
	}
	bindRefs(doc, s.Not, diags)
}

func localRef(ref, docID string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("$ref must not be empty")
	}
	if ref == "#" || strings.HasPrefix(ref, "#/") {
		return ref, nil
	}
	if i := strings.IndexByte(ref, '#'); i >= 0 && docID != "" && ref[:i] == docID {
		frag := ref[i:]
		if frag == "#" || strings.HasPrefix(frag, "#/") {
			return frag, nil
		}
	}
	return "", fmt.Errorf("only local $ref fragments are supported")
}

func findCycles(doc *Document, diags *[]Diagnostic) {
	if doc == nil || doc.Root == nil {
		return
	}
	reported := map[string]bool{}
	var walk func(*Schema)
	walk = func(s *Schema) {
		if s == nil {
			return
		}
		if s.Ref != "" && !reported[s.Pointer] {
			if cycle := refCycle(doc, s, map[string]bool{}); cycle != "" {
				fail(diags, s.Pointer, "cyclic $ref "+cycle)
				reported[s.Pointer] = true
			}
		}
		for _, child := range childSchemas(s) {
			walk(child)
		}
	}
	walk(doc.Root)
}

// refCycle reports a pointer when $ref links form a loop.
// Recursive schemas that point at an ancestor without a ref chain are allowed.
func refCycle(doc *Document, s *Schema, stack map[string]bool) string {
	if s == nil || s.Ref == "" {
		return ""
	}
	if stack[s.Ref] {
		return s.Ref
	}
	target, ok := doc.index[s.Ref]
	if !ok || target.Ref == "" {
		return ""
	}
	stack[s.Ref] = true
	return refCycle(doc, target, stack)
}

func childSchemas(s *Schema) []*Schema {
	var out []*Schema
	for _, child := range s.Properties {
		out = append(out, child)
	}
	if s.Additional != nil && s.Additional.Schema != nil {
		out = append(out, s.Additional.Schema)
	}
	if s.Items != nil {
		out = append(out, s.Items)
	}
	for _, child := range s.Defs {
		out = append(out, child)
	}
	out = append(out, s.OneOf...)
	out = append(out, s.AnyOf...)
	out = append(out, s.AllOf...)
	if s.Not != nil {
		out = append(out, s.Not)
	}
	return out
}
