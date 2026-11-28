package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	uuidRe  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Validate checks a JSON instance against the compiled schema.
// A JSON syntax error is returned as a Go error. Schema violations are diagnostics.
func (d *Document) Validate(data []byte) ([]Diagnostic, error) {
	if d == nil || d.Root == nil {
		return nil, fmt.Errorf("missing schema")
	}
	inst, err := decodeInstance(data)
	if err != nil {
		return nil, err
	}
	diags := d.validate(d.Root, inst, "$", map[string]bool{})
	SortDiagnostics(diags)
	return diags, nil
}

func decodeInstance(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return v, nil
}

func (d *Document) validate(s *Schema, inst any, path string, stack map[string]bool) []Diagnostic {
	if s == nil {
		return []Diagnostic{{Path: path, Message: "missing schema"}}
	}
	if s.Boolean != nil {
		if *s.Boolean {
			return nil
		}
		return []Diagnostic{{Path: path, Message: "rejected by false schema"}}
	}

	var diags []Diagnostic
	if s.Ref != "" {
		if stack[s.Ref] {
			return []Diagnostic{{Path: path, Message: "cyclic $ref " + s.Ref}}
		}
		target, ok := d.index[s.Ref]
		if !ok {
			return []Diagnostic{{Path: path, Message: "unresolved $ref " + s.Ref}}
		}
		next := cloneStack(stack)
		next[s.Ref] = true
		diags = append(diags, d.validate(target, inst, path, next)...)
		if !hasLocalChecks(s) {
			return diags
		}
	}
	diags = append(diags, d.validateLocal(s, inst, path, stack)...)
	return diags
}

func hasLocalChecks(s *Schema) bool {
	return len(s.Types) > 0 || s.HasConst || s.HasEnum ||
		len(s.Properties) > 0 || len(s.Required) > 0 || s.Additional != nil ||
		s.MinProperties != nil || s.MaxProperties != nil ||
		s.Items != nil || s.MinItems != nil || s.MaxItems != nil || s.UniqueItems ||
		s.Minimum != nil || s.Maximum != nil || s.ExclusiveMinimum != nil || s.ExclusiveMaximum != nil ||
		s.MinLength != nil || s.MaxLength != nil || s.pattern != nil || s.Format != "" ||
		len(s.OneOf) > 0 || len(s.AnyOf) > 0 || len(s.AllOf) > 0 || s.Not != nil
}

func (d *Document) validateLocal(s *Schema, inst any, path string, stack map[string]bool) []Diagnostic {
	var diags []Diagnostic
	typeFailed := false
	if len(s.Types) > 0 && !matchesAnyType(inst, s.Types) {
		diags = append(diags, Diagnostic{Path: path, Message: "expected " + humanTypes(s.Types)})
		typeFailed = true
	}
	diags = append(diags, checkConstEnum(s, inst, path)...)
	if typeFailed {
		diags = append(diags, d.checkCombinators(s, inst, path, stack)...)
		return diags
	}
	switch v := inst.(type) {
	case map[string]any:
		diags = append(diags, d.checkObject(s, v, path, stack)...)
	case []any:
		diags = append(diags, d.checkArray(s, v, path, stack)...)
	case string:
		diags = append(diags, checkString(s, v, path)...)
	default:
		if f, ok := asFloat(inst); ok {
			diags = append(diags, checkNumber(s, f, path)...)
		}
	}
	diags = append(diags, d.checkCombinators(s, inst, path, stack)...)
	return diags
}

func checkConstEnum(s *Schema, inst any, path string) []Diagnostic {
	var diags []Diagnostic
	if s.HasConst && !deepEqual(s.Const, inst) {
		diags = append(diags, Diagnostic{Path: path, Message: "value does not match const"})
	}
	if s.HasEnum {
		ok := false
		for _, item := range s.Enum {
			if deepEqual(item, inst) {
				ok = true
				break
			}
		}
		if !ok {
			diags = append(diags, Diagnostic{Path: path, Message: "value is not in enum"})
		}
	}
	return diags
}

func (d *Document) checkObject(s *Schema, obj map[string]any, path string, stack map[string]bool) []Diagnostic {
	if !objectKeywords(s) {
		return nil
	}
	var diags []Diagnostic
	if s.MinProperties != nil && len(obj) < *s.MinProperties {
		diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("expected at least %d properties", *s.MinProperties)})
	}
	if s.MaxProperties != nil && len(obj) > *s.MaxProperties {
		diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("expected at most %d properties", *s.MaxProperties)})
	}
	for _, name := range s.Required {
		if _, ok := obj[name]; !ok {
			diags = append(diags, Diagnostic{Path: childPath(path, name), Message: "required property is missing"})
		}
	}
	for key, val := range obj {
		if ps, ok := s.Properties[key]; ok {
			diags = append(diags, d.validate(ps, val, childPath(path, key), stack)...)
			continue
		}
		if s.Additional != nil && s.Additional.Forbidden {
			diags = append(diags, Diagnostic{Path: childPath(path, key), Message: "additional property is not allowed"})
			continue
		}
		if s.Additional != nil && s.Additional.Schema != nil {
			diags = append(diags, d.validate(s.Additional.Schema, val, childPath(path, key), stack)...)
		}
	}
	return diags
}

func objectKeywords(s *Schema) bool {
	return len(s.Properties) > 0 || len(s.Required) > 0 || s.Additional != nil ||
		s.MinProperties != nil || s.MaxProperties != nil
}

func (d *Document) checkArray(s *Schema, arr []any, path string, stack map[string]bool) []Diagnostic {
	if s.Items == nil && s.MinItems == nil && s.MaxItems == nil && !s.UniqueItems {
		return nil
	}
	var diags []Diagnostic
	if s.MinItems != nil && len(arr) < *s.MinItems {
		diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("expected at least %d items", *s.MinItems)})
	}
	if s.MaxItems != nil && len(arr) > *s.MaxItems {
		diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("expected at most %d items", *s.MaxItems)})
	}
	if s.Items != nil {
		for i, item := range arr {
			diags = append(diags, d.validate(s.Items, item, fmt.Sprintf("%s[%d]", path, i), stack)...)
		}
	}
	if s.UniqueItems {
		for i := 0; i < len(arr); i++ {
			for j := i + 1; j < len(arr); j++ {
				if deepEqual(arr[i], arr[j]) {
					diags = append(diags, Diagnostic{Path: fmt.Sprintf("%s[%d]", path, j), Message: "duplicate item"})
				}
			}
		}
	}
	return diags
}

func checkString(s *Schema, v, path string) []Diagnostic {
	var diags []Diagnostic
	n := utf8.RuneCountInString(v)
	if s.MinLength != nil && n < *s.MinLength {
		diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("shorter than minLength %d", *s.MinLength)})
	}
	if s.MaxLength != nil && n > *s.MaxLength {
		diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("longer than maxLength %d", *s.MaxLength)})
	}
	if s.pattern != nil && s.pattern.FindStringIndex(v) == nil {
		diags = append(diags, Diagnostic{Path: path, Message: "does not match pattern"})
	}
	if s.Format != "" {
		if msg := checkFormat(s.Format, v); msg != "" {
			diags = append(diags, Diagnostic{Path: path, Message: msg})
		}
	}
	return diags
}

func checkNumber(s *Schema, f float64, path string) []Diagnostic {
	var diags []Diagnostic
	if s.Minimum != nil && f < *s.Minimum {
		diags = append(diags, Diagnostic{Path: path, Message: "below minimum"})
	}
	if s.Maximum != nil && f > *s.Maximum {
		diags = append(diags, Diagnostic{Path: path, Message: "above maximum"})
	}
	if s.ExclusiveMinimum != nil && f <= *s.ExclusiveMinimum {
		diags = append(diags, Diagnostic{Path: path, Message: "not above exclusiveMinimum"})
	}
	if s.ExclusiveMaximum != nil && f >= *s.ExclusiveMaximum {
		diags = append(diags, Diagnostic{Path: path, Message: "not below exclusiveMaximum"})
	}
	return diags
}

func (d *Document) checkCombinators(s *Schema, inst any, path string, stack map[string]bool) []Diagnostic {
	var diags []Diagnostic
	for _, sub := range s.AllOf {
		diags = append(diags, d.validate(sub, inst, path, stack)...)
	}
	if len(s.AnyOf) > 0 {
		matched := false
		for _, sub := range s.AnyOf {
			if len(d.validate(sub, inst, path, stack)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			diags = append(diags, Diagnostic{Path: path, Message: "does not match any anyOf alternative"})
		}
	}
	if len(s.OneOf) > 0 {
		matches := 0
		for _, sub := range s.OneOf {
			if len(d.validate(sub, inst, path, stack)) == 0 {
				matches++
			}
		}
		if matches != 1 {
			diags = append(diags, Diagnostic{Path: path, Message: fmt.Sprintf("expected exactly one oneOf match, found %d", matches)})
		}
	}
	if s.Not != nil && len(d.validate(s.Not, inst, path, stack)) == 0 {
		diags = append(diags, Diagnostic{Path: path, Message: "should not match schema"})
	}
	return diags
}

func matchesAnyType(inst any, types []string) bool {
	for _, typ := range types {
		if matchesType(inst, typ) {
			return true
		}
	}
	return false
}

func matchesType(inst any, typ string) bool {
	switch typ {
	case "null":
		return inst == nil
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "array":
		_, ok := inst.([]any)
		return ok
	case "object":
		_, ok := inst.(map[string]any)
		return ok
	case "number":
		_, ok := asFloat(inst)
		return ok
	case "integer":
		return isInteger(inst)
	default:
		return false
	}
}

func humanTypes(types []string) string {
	if len(types) == 0 {
		return "a value"
	}
	if len(types) == 1 {
		return types[0]
	}
	return strings.Join(types[:len(types)-1], ", ") + " or " + types[len(types)-1]
}

func checkFormat(format, v string) string {
	switch format {
	case "email":
		addr, err := mail.ParseAddress(v)
		if err != nil || addr.Address != v || !strings.Contains(v, "@") {
			return "is not a valid email"
		}
	case "uuid":
		if !uuidRe.MatchString(v) {
			return "is not a valid uuid"
		}
	case "date":
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return "is not a valid date"
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			return "is not a valid date-time"
		}
	case "uri":
		u, err := url.Parse(v)
		if err != nil || u.Scheme == "" || (u.Host == "" && u.Opaque == "") {
			return "is not a valid uri"
		}
	}
	return ""
}

func childPath(parent, key string) string {
	if identRe.MatchString(key) {
		if parent == "$" {
			return "$." + key
		}
		return parent + "." + key
	}
	esc := strings.ReplaceAll(key, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `'`, `\'`)
	return parent + "['" + esc + "']"
}

func cloneStack(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}
