// Package catalog turns schemas and a sample ontology into Neo4j statements.
package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Node is one graph vertex. Labels come from a fixed vocabulary.
type Node struct {
	ID     string
	Labels []string
	Props  map[string]string
}

// Edge connects two node ids. Props are string parameters, never labels.
type Edge struct {
	From  string
	To    string
	Type  string
	Props map[string]string
}

// Statement is one parameterized Cypher statement.
type Statement struct {
	Query  string
	Params map[string]any
}

// Graph is an unordered catalog fragment. Script and Statements sort it.
type Graph struct {
	Nodes    []Node
	Edges    []Edge
	nodeSeen map[string]bool
	edgeSeen map[string]bool
}

// AddNode inserts a node once, keyed by id.
func (g *Graph) AddNode(n Node) {
	if g.nodeSeen == nil {
		g.nodeSeen = map[string]bool{}
	}
	if n.ID == "" || g.nodeSeen[n.ID] {
		return
	}
	g.nodeSeen[n.ID] = true
	if n.Props == nil {
		n.Props = map[string]string{}
	}
	g.Nodes = append(g.Nodes, n)
}

// AddEdge inserts a relationship once.
func (g *Graph) AddEdge(from, to, typ string, props map[string]string) {
	if g.edgeSeen == nil {
		g.edgeSeen = map[string]bool{}
	}
	via := ""
	if props != nil {
		via = props["via"]
	}
	key := from + "\x00" + typ + "\x00" + to + "\x00" + via
	if from == "" || to == "" || typ == "" || g.edgeSeen[key] {
		return
	}
	g.edgeSeen[key] = true
	g.Edges = append(g.Edges, Edge{From: from, To: to, Type: typ, Props: props})
}

// Merge copies nodes and edges into a new graph.
func Merge(parts ...*Graph) *Graph {
	out := &Graph{}
	for _, part := range parts {
		if part == nil {
			continue
		}
		for _, node := range part.Nodes {
			out.AddNode(node)
		}
		for _, edge := range part.Edges {
			out.AddEdge(edge.From, edge.To, edge.Type, edge.Props)
		}
	}
	return out
}

// Statements returns MERGE/MATCH statements in stable order.
func (g *Graph) Statements() ([]Statement, error) {
	if g == nil {
		return nil, nil
	}
	var out []Statement
	nodes := append([]Node(nil), g.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	for _, node := range nodes {
		stmt, err := nodeStatement(node)
		if err != nil {
			return nil, err
		}
		out = append(out, stmt)
	}
	edges := append([]Edge(nil), g.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Props["via"] < edges[j].Props["via"]
	})
	for _, edge := range edges {
		stmt, err := edgeStatement(edge)
		if err != nil {
			return nil, err
		}
		out = append(out, stmt)
	}
	return out, nil
}

// Script renders Statements as cypher-shell text with escaped literals.
func (g *Graph) Script() (string, error) {
	statements, err := g.Statements()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, stmt := range statements {
		b.WriteString(render(stmt))
		b.WriteString(";\n")
	}
	return b.String(), nil
}

func nodeStatement(n Node) (Statement, error) {
	if len(n.Labels) != 1 {
		return Statement{}, fmt.Errorf("node %s needs one label", n.ID)
	}
	if err := safeIdent(n.Labels[0]); err != nil {
		return Statement{}, err
	}
	params := map[string]any{"id": n.ID}
	var sets []string
	for _, key := range []string{"description", "kind", "name", "pointer", "schema_id", "title"} {
		if err := safeIdent(key); err != nil {
			return Statement{}, err
		}
		if v := n.Props[key]; v != "" {
			params[key] = v
			sets = append(sets, "n."+key+" = $"+key)
		}
	}
	q := fmt.Sprintf("MERGE (n:%s {id: $id})", n.Labels[0])
	if len(sets) > 0 {
		q += " SET " + strings.Join(sets, ", ")
	}
	return Statement{Query: q, Params: params}, nil
}

func edgeStatement(e Edge) (Statement, error) {
	if err := safeIdent(e.Type); err != nil {
		return Statement{}, err
	}
	params := map[string]any{"src": e.From, "dst": e.To}
	q := fmt.Sprintf("MATCH (src {id: $src}), (dst {id: $dst}) MERGE (src)-[rel:%s]->(dst)", e.Type)
	if via := e.Props["via"]; via != "" {
		params["via"] = via
		q += " SET rel.via = $via"
	}
	return Statement{Query: q, Params: params}, nil
}

func render(stmt Statement) string {
	q := stmt.Query
	keys := make([]string, 0, len(stmt.Params))
	for key := range stmt.Params {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		q = strings.ReplaceAll(q, "$"+key, cypherLiteral(stmt.Params[key]))
	}
	return q
}

func cypherLiteral(v any) string {
	s, ok := v.(string)
	if !ok {
		return "null"
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

func safeIdent(s string) error {
	if !identRe.MatchString(s) {
		return fmt.Errorf("unsafe cypher identifier %q", s)
	}
	return nil
}
