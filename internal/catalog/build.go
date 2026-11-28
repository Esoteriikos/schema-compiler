package catalog

import (
	"sort"
	"strings"

	"github.com/Esoteriikos/schema-compiler/internal/schema"
)

type classRef struct {
	node   Node
	schema *schema.Schema
}

// FromSchema indexes object schemas, properties, ranges, and $ref links.
func FromSchema(doc *schema.Document) *Graph {
	g := &Graph{}
	if doc == nil || doc.Root == nil {
		return g
	}
	schemaName := first(doc.Root.Title, "root")
	schemaKey := first(doc.Root.ID, doc.Root.Title, "root")
	sid := "jsonschema:" + schemaKey
	g.AddNode(Node{
		ID:     sid,
		Labels: []string{"JsonSchema"},
		Props: map[string]string{
			"name":        schemaName,
			"title":       doc.Root.Title,
			"description": doc.Root.Description,
			"schema_id":   doc.Root.ID,
			"kind":        "schema",
		},
	})

	var classes []classRef
	collectClasses(doc.Root, schemaName, &classes)
	seen := map[string]bool{}
	var unique []classRef
	for _, class := range classes {
		if seen[class.node.ID] {
			continue
		}
		seen[class.node.ID] = true
		unique = append(unique, class)
		g.AddNode(class.node)
		g.AddEdge(sid, class.node.ID, "DEFINES", nil)
	}
	for _, class := range unique {
		indexProperties(doc, g, class)
	}
	return g
}

// Build merges schema classes with the sample ontology and aligns shared names.
func Build(doc *schema.Document, includeSample bool) *Graph {
	var parts []*Graph
	if doc != nil {
		parts = append(parts, FromSchema(doc))
	}
	var sample *Graph
	if includeSample {
		sample = Sample()
		parts = append(parts, sample)
	}
	g := Merge(parts...)
	if doc != nil && sample != nil {
		align(g, sample)
	}
	return g
}

func collectClasses(s *schema.Schema, fallback string, out *[]classRef) {
	if s == nil {
		return
	}
	if isClassSchema(s) {
		name := first(s.Title, fallback, "Anonymous")
		*out = append(*out, classRef{
			node: Node{
				ID:     "schemaClass:" + s.Pointer,
				Labels: []string{"Class"},
				Props: map[string]string{
					"name":        name,
					"title":       s.Title,
					"description": s.Description,
					"pointer":     s.Pointer,
					"kind":        "class",
				},
			},
			schema: s,
		})
	}
	names := make([]string, 0, len(s.Defs))
	for name := range s.Defs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		collectClasses(s.Defs[name], name, out)
	}
}

func indexProperties(doc *schema.Document, g *Graph, class classRef) {
	eff, err := doc.Deref(class.schema)
	if err != nil || eff == nil {
		eff = class.schema
	}
	names := make([]string, 0, len(eff.Properties))
	for name := range eff.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	required := map[string]bool{}
	for _, name := range eff.Required {
		required[name] = true
	}
	for _, name := range names {
		ps := eff.Properties[name]
		pid := class.node.ID + "/prop/" + name
		g.AddNode(Node{
			ID:     pid,
			Labels: []string{"Property"},
			Props: map[string]string{
				"name":        name,
				"description": ps.Description,
				"pointer":     ps.Pointer,
				"kind":        "property",
			},
		})
		g.AddEdge(class.node.ID, pid, "HAS_PROPERTY", map[string]string{"via": name})
		if required[name] {
			g.AddEdge(class.node.ID, pid, "REQUIRES", map[string]string{"via": name})
		}
		target := ps
		if ps.Ref != "" {
			if resolved, ok := doc.Lookup(ps.Ref); ok {
				target = resolved
			}
		}
		if ps.Ref != "" && isClassSchema(target) {
			g.AddEdge(pid, "schemaClass:"+target.Pointer, "REFERENCES", map[string]string{"via": name})
			continue
		}
		dtype := typeName(target)
		did := "dtype:" + dtype
		g.AddNode(Node{
			ID:     did,
			Labels: []string{"Datatype"},
			Props: map[string]string{
				"name": dtype,
				"kind": "datatype",
			},
		})
		g.AddEdge(pid, did, "HAS_DATATYPE", map[string]string{"via": name})
	}
}

func align(g *Graph, sample *Graph) {
	byName := map[string]string{}
	for _, node := range sample.Nodes {
		if len(node.Labels) == 1 && node.Labels[0] == "Class" {
			byName[node.Props["name"]] = node.ID
		}
	}
	current := append([]Node(nil), g.Nodes...)
	for _, node := range current {
		if len(node.Labels) == 1 && node.Labels[0] == "Class" && strings.HasPrefix(node.ID, "schemaClass:") {
			if id, ok := byName[node.Props["name"]]; ok {
				g.AddEdge(node.ID, id, "ALIGNS_WITH", nil)
			}
		}
	}
}

func isClassSchema(s *schema.Schema) bool {
	if s == nil || s.Boolean != nil {
		return false
	}
	if s.Ref != "" && len(s.Properties) == 0 && !hasType(s, "object") {
		return false
	}
	return hasType(s, "object") || len(s.Properties) > 0
}

func hasType(s *schema.Schema, want string) bool {
	for _, typ := range s.Types {
		if typ == want {
			return true
		}
	}
	return false
}

func typeName(s *schema.Schema) string {
	if s == nil || len(s.Types) == 0 {
		return "unspecified"
	}
	return strings.Join(s.Types, "|")
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
