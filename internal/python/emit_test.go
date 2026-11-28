package python

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Esoteriikos/schema-compiler/internal/schema"
)

func TestEmitIsDeterministicAndNamed(t *testing.T) {
	doc := mustSchema(t, `{
		"title": "Person",
		"type": "object",
		"additionalProperties": false,
		"required": ["name"],
		"properties": {
			"name": {"type": "string", "minLength": 1, "pattern": "^[A-Za-z]+$"},
			"age": {"type": "integer", "minimum": 0}
		},
		"$defs": {"Note": {"type": "string"}}
	}`)
	first, err := Emit(doc)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Emit(doc)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("emit was not deterministic")
	}
	for _, want := range []string{"def transform(", "def transform_Person(", "def transform_Note(", "minLength", "TransformError"} {
		if !strings.Contains(first, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestEmitPythonBehavior(t *testing.T) {
	bin := pythonBin()
	if bin == "" {
		t.Skip("python is not installed")
	}
	doc := mustSchema(t, `{
		"$defs": {
			"Person": {
				"type": "object",
				"additionalProperties": false,
				"required": ["name"],
				"properties": {
					"name": {"type": "string", "minLength": 1},
					"role": {"enum": ["admin", "user"]}
				}
			}
		},
		"oneOf": [
			{"$ref": "#/$defs/Person"},
			{"type": "integer", "minimum": 1, "maximum": 3}
		]
	}`)
	src, err := Emit(doc)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "transform_mod.py")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
import importlib.util, json, pathlib, sys
path = pathlib.Path(sys.argv[1])
spec = importlib.util.spec_from_file_location("transform_mod", path)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
cases = [
    ({"name": "Ada", "role": "admin"}, True),
    (2, True),
    ({"name": "", "role": "admin"}, False),
    ({"name": "Ada", "role": "root"}, False),
    ({"name": "Ada", "extra": 1}, False),
    (0, False),
    ("x", False),
]
for value, ok in cases:
    try:
        out = mod.transform(value)
    except mod.TransformError:
        if ok:
            raise SystemExit("rejected " + json.dumps(value))
        continue
    if not ok:
        raise SystemExit("accepted " + json.dumps(value))
    if out != value:
        raise SystemExit("changed " + json.dumps(value))
print("ok")
`
	cmd := exec.Command(bin, "-c", script, path)
	if bin == "py" {
		cmd = exec.Command(bin, "-3", "-c", script, path)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python failed: %v\n%s\n%s", err, out, src)
	}
}

func TestEmitCatalogExample(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "catalog.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := schema.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	src, err := Emit(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"transform_CatalogRecord", "transform_Dataset", "transform_Organization", "transform_Person"} {
		if !strings.Contains(src, "def "+name+"(") {
			t.Fatalf("missing %s", name)
		}
	}
	bin := pythonBin()
	if bin == "" {
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_transform.py")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "catalog.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inst any
	if err := json.Unmarshal(data, &inst); err != nil {
		t.Fatal(err)
	}
	script := `
import importlib.util, json, pathlib, sys
path, raw = sys.argv[1], pathlib.Path(sys.argv[2]).read_text(encoding="utf-8")
spec = importlib.util.spec_from_file_location("catalog_transform", path)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
value = json.loads(raw)
out = mod.transform(value)
if out != value:
    raise SystemExit(json.dumps(out))
bad = json.loads(raw)
bad["dataset"]["quality"] = 1000
try:
    mod.transform(bad)
except mod.TransformError:
    print("ok")
else:
    raise SystemExit("accepted invalid quality")
`
	cmd := exec.Command(bin, "-c", script, path, filepath.Join("..", "..", "examples", "catalog.valid.json"))
	if bin == "py" {
		cmd = exec.Command(bin, "-3", "-c", script, path, filepath.Join("..", "..", "examples", "catalog.valid.json"))
	}
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python failed: %v\n%s", err, out)
	}
}

func pythonBin() string {
	for _, name := range []string{"python", "python3", "py"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func mustSchema(t *testing.T, raw string) *schema.Document {
	t.Helper()
	doc, err := schema.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
