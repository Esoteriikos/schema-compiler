// Package schema parses and validates a practical subset of JSON Schema.
package schema

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Diagnostic is one schema or instance problem.
// Schema issues use JSON Pointer paths. Instance issues use a JSONPath-like path.
type Diagnostic struct {
	Path    string
	Message string
}

func (d Diagnostic) Error() string {
	if d.Path == "" {
		return d.Message
	}
	return d.Path + ": " + d.Message
}

// ErrorList is the set of problems found while compiling a schema.
type ErrorList []Diagnostic

func (e ErrorList) Error() string {
	if len(e) == 0 {
		return "schema error"
	}
	parts := make([]string, len(e))
	for i, d := range e {
		parts[i] = d.Error()
	}
	return strings.Join(parts, "\n")
}

// Schema is one compiled schema node. Boolean is set for boolean schemas.
type Schema struct {
	Pointer     string
	Boolean     *bool
	ID          string
	SchemaURI   string
	Title       string
	Description string
	Ref         string

	Types []string

	Properties    map[string]*Schema
	Required      []string
	Additional    *Additional
	MinProperties *int
	MaxProperties *int

	Items       *Schema
	MinItems    *int
	MaxItems    *int
	UniqueItems bool

	Minimum          *float64
	Maximum          *float64
	ExclusiveMinimum *float64
	ExclusiveMaximum *float64

	MinLength *int
	MaxLength *int
	Pattern   string
	Format    string

	Enum     []any
	HasEnum  bool
	Const    any
	HasConst bool

	Defs  map[string]*Schema
	OneOf []*Schema
	AnyOf []*Schema
	AllOf []*Schema
	Not   *Schema

	pattern *regexp.Regexp
}

// Additional is the compiled form of additionalProperties.
// Forbidden is true when the keyword is false.
// Schema is set when the keyword is a schema.
// A nil Additional means additional properties are allowed.
type Additional struct {
	Forbidden bool
	Schema    *Schema
}

// Document is a compiled schema and an index of every node by JSON Pointer.
type Document struct {
	Root  *Schema
	index map[string]*Schema
}

// Lookup returns the schema compiled at a local JSON Pointer.
func (d *Document) Lookup(pointer string) (*Schema, bool) {
	if d == nil || d.index == nil {
		return nil, false
	}
	s, ok := d.index[pointer]
	return s, ok
}

// Deref follows local $ref links until it reaches a non-ref schema.
func (d *Document) Deref(s *Schema) (*Schema, error) {
	if d == nil || s == nil {
		return nil, fmt.Errorf("missing schema")
	}
	seen := map[string]bool{}
	cur := s
	for cur != nil && cur.Ref != "" {
		if seen[cur.Ref] {
			return nil, fmt.Errorf("cyclic $ref %s", cur.Ref)
		}
		seen[cur.Ref] = true
		next, ok := d.index[cur.Ref]
		if !ok {
			return nil, fmt.Errorf("unresolved $ref %s", cur.Ref)
		}
		cur = next
	}
	if cur == nil {
		return nil, fmt.Errorf("missing schema")
	}
	return cur, nil
}

// SortDiagnostics orders diagnostics so CLI output and tests stay stable.
func SortDiagnostics(d []Diagnostic) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].Path != d[j].Path {
			return d[i].Path < d[j].Path
		}
		return d[i].Message < d[j].Message
	})
}

// EscapePointer escapes one JSON Pointer reference token.
func EscapePointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

// ChildPointer appends a token to a JSON Pointer.
func ChildPointer(parent, token string) string {
	esc := EscapePointer(token)
	if parent == "" || parent == "#" {
		return "#/" + esc
	}
	return parent + "/" + esc
}
