// Package cli is the schemac command line.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Esoteriikos/schema-compiler/internal/catalog"
	"github.com/Esoteriikos/schema-compiler/internal/neo4j"
	"github.com/Esoteriikos/schema-compiler/internal/python"
	"github.com/Esoteriikos/schema-compiler/internal/schema"
	"github.com/Esoteriikos/schema-compiler/internal/typecheck"
)

// Run executes schemac and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stderr, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	switch args[0] {
	case "validate":
		return validateCmd(args[1:], stdout, stderr)
	case "check":
		return checkCmd(args[1:], stdout, stderr)
	case "emit":
		return emitCmd(args[1:], stdout, stderr)
	case "index":
		return indexCmd(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

const usage = `schemac validates JSON Schema, type-checks models, emits Python transforms,
and indexes ontology links for Neo4j.

Usage:
  schemac validate -schema FILE -data FILE
  schemac check -schema FILE -model FILE
  schemac emit -schema FILE [-out FILE]
  schemac index [-schema FILE] [-sample=true] [-out FILE]
                [-neo4j URL] [-user USER] [-password PASS] [-database DB]

The index command includes the sample ontology unless -sample=false.
Neo4j settings also come from NEO4J_URI, NEO4J_USER, NEO4J_PASSWORD,
and NEO4J_DATABASE.
`

func validateCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	schemaPath := fs.String("schema", "", "JSON Schema file")
	dataPath := fs.String("data", "", "JSON instance file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *schemaPath == "" || *dataPath == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "validate requires -schema and -data")
		return 2
	}
	doc, code := loadSchema(*schemaPath, stderr)
	if code != 0 {
		return code
	}
	raw, err := os.ReadFile(*dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	diags, err := doc.Validate(raw)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return finishDiags(diags, stderr)
}

func checkCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	schemaPath := fs.String("schema", "", "JSON Schema file")
	modelPath := fs.String("model", "", "model file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *schemaPath == "" || *modelPath == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "check requires -schema and -model")
		return 2
	}
	doc, code := loadSchema(*schemaPath, stderr)
	if code != 0 {
		return code
	}
	raw, err := os.ReadFile(*modelPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	models, err := typecheck.ParseModels(raw)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return finishDiags(typecheck.Check(doc, models), stderr)
}

func emitCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	schemaPath := fs.String("schema", "", "JSON Schema file")
	outPath := fs.String("out", "", "Python file to write")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *schemaPath == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "emit requires -schema")
		return 2
	}
	doc, code := loadSchema(*schemaPath, stderr)
	if code != 0 {
		return code
	}
	src, err := python.Emit(doc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *outPath == "" {
		fmt.Fprint(stdout, src)
		return 0
	}
	if err := os.WriteFile(*outPath, []byte(src), 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintf(stderr, "wrote %s\n", *outPath)
	return 0
}

func indexCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(stderr)
	schemaPath := fs.String("schema", "", "JSON Schema file")
	outPath := fs.String("out", "", "Cypher file to write")
	includeSample := fs.Bool("sample", true, "include the sample ontology")
	neoURL := fs.String("neo4j", os.Getenv("NEO4J_URI"), "Neo4j HTTP base URL")
	user := fs.String("user", os.Getenv("NEO4J_USER"), "Neo4j user")
	password := fs.String("password", os.Getenv("NEO4J_PASSWORD"), "Neo4j password")
	database := fs.String("database", envOr("NEO4J_DATABASE", "neo4j"), "Neo4j database")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected index arguments")
		return 2
	}
	var doc *schema.Document
	if *schemaPath != "" {
		loaded, code := loadSchema(*schemaPath, stderr)
		if code != 0 {
			return code
		}
		doc = loaded
	}
	if doc == nil && !*includeSample {
		fmt.Fprintln(stderr, "index needs a schema or the sample ontology")
		return 2
	}
	graph := catalog.Build(doc, *includeSample)
	script, err := graph.Script()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	statements, err := graph.Statements()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *outPath != "" {
		if err := os.WriteFile(*outPath, []byte(script), 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		fmt.Fprintf(stderr, "wrote %s\n", *outPath)
	}
	if *neoURL != "" {
		if *user == "" || *password == "" {
			fmt.Fprintln(stderr, "neo4j indexing requires -user and -password")
			return 2
		}
		client := &neo4j.Client{
			BaseURL:  *neoURL,
			User:     *user,
			Password: *password,
			Database: *database,
		}
		batch := make([]neo4j.Statement, len(statements))
		for i, stmt := range statements {
			batch[i] = neo4j.Statement{Query: stmt.Query, Params: stmt.Params}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.Commit(ctx, batch); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stderr, "indexed %d statements\n", len(batch))
		return 0
	}
	if *outPath == "" {
		fmt.Fprint(stdout, script)
	}
	return 0
}

func loadSchema(path string, stderr io.Writer) (*schema.Document, int) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return nil, 2
	}
	doc, err := schema.Parse(raw)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return nil, 2
	}
	return doc, 0
}

func finishDiags(diags []schema.Diagnostic, stderr io.Writer) int {
	if len(diags) == 0 {
		return 0
	}
	for _, d := range diags {
		fmt.Fprintln(stderr, d.Error())
	}
	return 1
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
