package catalog

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/Esoteriikos/schema-compiler/internal/schema"
)

func TestSampleOntologyLinks(t *testing.T) {
	script, err := Sample().Script()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"worksFor", "DOMAIN", "RANGE", "onto:Person", "onto:Organization", "SUBCLASS_OF", "DECLARES"} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q\n%s", want, script)
		}
	}
	assertCypherSane(t, script)
}

func TestSchemaLinksAlignWithSample(t *testing.T) {
	doc := mustSchema(t, `{
		"title": "CatalogRecord",
		"type": "object",
		"properties": {"dataset": {"$ref": "#/$defs/Dataset"}},
		"$defs": {
			"Dataset": {
				"title": "Dataset",
				"type": "object",
				"required": ["owner", "name"],
				"properties": {
					"owner": {"$ref": "#/$defs/Organization"},
					"name": {"type": "string", "description": "O'Brien \\ team"}
				}
			},
			"Organization": {"title": "Organization", "type": "object", "properties": {"name": {"type": "string"}}}
		}
	}`)
	script, err := Build(doc, true).Script()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"REFERENCES", "HAS_DATATYPE", "REQUIRES", "ALIGNS_WITH", "onto:Dataset", "O\\'Brien"} {
		if !strings.Contains(script, want) {
			t.Fatalf("missing %q\n%s", want, script)
		}
	}
	assertCypherSane(t, script)
}

func TestScriptEscapesQuotes(t *testing.T) {
	g := &Graph{}
	g.AddNode(Node{
		ID:     "class:x",
		Labels: []string{"Class"},
		Props:  map[string]string{"name": "O'Brien", "description": `a \ b`},
	})
	script, err := g.Script()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `'O\'Brien'`) || !strings.Contains(script, `'a \\ b'`) {
		t.Fatal(script)
	}
	assertCypherSane(t, script)
}

func TestPropertyScriptsStayBalanced(t *testing.T) {
	const seed = 20261126
	rng := rand.New(rand.NewPCG(seed, seed^0x44))
	chunks := []string{`it's`, `a \ path`, `quote "here"`, "line\nbreak", "ok"}
	for i := 0; i < 25; i++ {
		desc := chunks[rng.IntN(len(chunks))] + fmt.Sprintf(" %d", i)
		raw := fmt.Sprintf(`{"title":"Dataset","description":%q,"type":"object","properties":{"name":{"type":"string","description":%q}}}`, desc, desc)
		doc, err := schema.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err, raw)
		}
		script, err := Build(doc, i%2 == 0).Script()
		if err != nil {
			t.Fatal(err)
		}
		assertCypherSane(t, script)
		if !strings.Contains(script, "MERGE") {
			t.Fatal(script)
		}
	}
}

func assertCypherSane(t *testing.T, script string) {
	t.Helper()
	inString := false
	escaped := false
	parens := 0
	for _, r := range script {
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				inString = false
			}
			continue
		}
		switch r {
		case '\'':
			inString = true
		case '(':
			parens++
		case ')':
			parens--
			if parens < 0 {
				t.Fatalf("unbalanced parenthesis\n%s", script)
			}
		}
	}
	if inString || escaped || parens != 0 {
		t.Fatalf("unbalanced cypher in=%v escape=%v parens=%d\n%s", inString, escaped, parens, script)
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
