# schema-compiler

Local JSON Schema compiler. It parses a practical subset of JSON Schema, validates instances, type-checks structural models, emits Python transform modules, and indexes schema structure plus a sample ontology as Neo4j statements.

## Build

```text
go test ./...
go build -o schemac.exe ./cmd/schemac
```

## Commands

```text
schemac validate -schema examples/catalog.schema.json -data examples/catalog.valid.json
schemac check -schema examples/catalog.schema.json -model examples/catalog.model.json
schemac emit -schema examples/catalog.schema.json -out examples/transform_catalog.py
schemac index -schema examples/catalog.schema.json -out examples/catalog.cypher
```

`validate` checks a JSON document. `check` compares a model file with the schema. `emit` writes a Python module whose `transform_*` functions copy a valid value and raise `TransformError` otherwise. `index` prints Cypher for the schema and the built-in sample ontology.

To commit those statements to Neo4j's HTTP API:

```text
schemac index -schema examples/catalog.schema.json -neo4j http://localhost:7474 -user neo4j -password password
```

`NEO4J_URI`, `NEO4J_USER`, `NEO4J_PASSWORD`, and `NEO4J_DATABASE` are also read. `-sample=false` skips the built-in ontology. With no schema, `index` still emits the sample links.

## Schema subset

The parser accepts boolean schemas and these keywords:

`$id`, `$schema`, `$ref`, `$defs`, `definitions`, `title`, `description`, `type`, `properties`, `required`, `additionalProperties`, `minProperties`, `maxProperties`, `items`, `minItems`, `maxItems`, `uniqueItems`, `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `minLength`, `maxLength`, `pattern`, `format`, `enum`, `const`, `oneOf`, `anyOf`, `allOf`, `not`.

Local `$ref` fragments are resolved, including a `$ref` whose base matches `$id`. External refs are rejected. A `$ref` chain that loops is rejected. A schema may still refer to an ancestor, so tree-shaped data is valid. `items` is one schema, not a tuple. Unknown keywords are ignored.

Formats that are checked: `email`, `uuid`, `date`, `date-time`, and `uri`. Other formats are annotations.

`minimum` must not exceed `maximum`, and the same pairing is checked for lengths, item counts, and property counts.

## Models

A model file lists structural fields and the schema type they claim to inhabit.

```json
{
  "name": "Dataset",
  "target": "Dataset",
  "fields": [
    {"name": "id", "type": "string"},
    {"name": "owner", "type": "object", "ref": "Organization"},
    {"name": "tags", "type": "array", "optional": true, "items": {"type": "string"}}
  ]
}
```

`target` and `ref` accept a `$defs` name or a JSON Pointer. An integer field may inhabit a number schema. A number field may not inhabit an integer schema. Required schema properties must be present and not optional. `additionalProperties: false` rejects extra fields.

## Python transforms

Generated modules validate and return a normalized copy. Object schemas copy declared properties. `oneOf` returns the single matching branch. `anyOf` returns the first matching branch. `allOf` checks every branch. Patterns are compiled with Go's RE2 engine in the validator and with Python's `re` in the generated module, so patterns should be valid in both.

## Graph catalog

Object schemas and `$defs` become `:Class` nodes. Properties become `:Property` nodes, with `:REQUIRES`, `:HAS_DATATYPE`, or `:REFERENCES` edges. The sample ontology adds `Person`, `Organization`, `Dataset`, and links such as `worksFor`, `publishes`, and `conformsTo`. Classes with the same name get an `:ALIGNS_WITH` edge. User text is passed as Cypher parameters or escaped literals, not as labels.

## Tests

`go test ./...` runs the unit tests and property tests. The property tests generate schemas and instances: conforming values pass, a wrong JSON type fails, and known-bad schemas are rejected. Random byte strings must not panic the parser. Further checks cover model incompatibility, balanced Cypher under quoted descriptions, and the Neo4j HTTP client against a local test server. When Python is on `PATH`, the emitter tests execute the generated module.
