package cli

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommands(t *testing.T) {
	schemaPath := repoPath("examples", "catalog.schema.json")
	valid := repoPath("examples", "catalog.valid.json")
	invalid := repoPath("examples", "catalog.invalid.json")
	model := repoPath("examples", "catalog.model.json")
	badModel := repoPath("examples", "catalog.model.bad.json")

	if code, _, stderr := run("validate", "-schema", schemaPath, "-data", valid); code != 0 {
		t.Fatalf("validate valid: %d %s", code, stderr)
	}
	code, _, stderr := run("validate", "-schema", schemaPath, "-data", invalid)
	if code != 1 || !strings.Contains(stderr, "uuid") || !strings.Contains(stderr, "required") {
		t.Fatalf("validate invalid: %d %s", code, stderr)
	}
	if code, _, stderr := run("check", "-schema", schemaPath, "-model", model); code != 0 {
		t.Fatalf("check: %d %s", code, stderr)
	}
	code, _, stderr = run("check", "-schema", schemaPath, "-model", badModel)
	if code != 1 || !strings.Contains(stderr, "not compatible") {
		t.Fatalf("bad model: %d %s", code, stderr)
	}
	code, stdout, stderr := run("emit", "-schema", schemaPath)
	if code != 0 || !strings.Contains(stdout, "def transform_Dataset(") {
		t.Fatalf("emit: %d %s %s", code, stderr, stdout[:min(200, len(stdout))])
	}
	code, stdout, stderr = run("index", "-schema", schemaPath, "-neo4j=")
	if code != 0 || !strings.Contains(stdout, "worksFor") || !strings.Contains(stdout, "ALIGNS_WITH") {
		t.Fatalf("index: %d %s", code, stderr)
	}
	if code, _, _ := run("nope"); code != 2 {
		t.Fatal(code)
	}
}

func run(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func repoPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	all := append([]string{filepath.Dir(file), "..", ".."}, parts...)
	return filepath.Join(all...)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
