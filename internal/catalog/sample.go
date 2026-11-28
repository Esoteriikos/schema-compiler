package catalog

// Sample is a small ontology for people, organizations, and datasets.
func Sample() *Graph {
	g := &Graph{}
	g.AddNode(Node{
		ID:     "onto:Ontology",
		Labels: []string{"Ontology"},
		Props: map[string]string{
			"name":        "Sample catalog ontology",
			"description": "Links people, organizations, datasets, and schemas.",
			"kind":        "ontology",
		},
	})
	classes := []struct {
		id, name, desc string
	}{
		{"onto:Entity", "Entity", "Something the catalog can describe."},
		{"onto:Person", "Person", "A person."},
		{"onto:Organization", "Organization", "An organization or team."},
		{"onto:Dataset", "Dataset", "A published dataset."},
		{"onto:SchemaDocument", "SchemaDocument", "A schema that describes a dataset."},
		{"onto:Concept", "Concept", "A subject or tag."},
	}
	for _, class := range classes {
		g.AddNode(Node{
			ID:     class.id,
			Labels: []string{"Class"},
			Props: map[string]string{
				"name":        class.name,
				"description": class.desc,
				"kind":        "class",
			},
		})
		g.AddEdge("onto:Ontology", class.id, "DECLARES", nil)
	}
	for _, id := range []string{"onto:Person", "onto:Organization", "onto:Dataset", "onto:SchemaDocument", "onto:Concept"} {
		g.AddEdge(id, "onto:Entity", "SUBCLASS_OF", nil)
	}
	props := []struct {
		name, from, to, desc string
	}{
		{"worksFor", "onto:Person", "onto:Organization", "Person works for an organization."},
		{"employs", "onto:Organization", "onto:Person", "Organization employs a person."},
		{"publishes", "onto:Organization", "onto:Dataset", "Organization publishes a dataset."},
		{"conformsTo", "onto:Dataset", "onto:SchemaDocument", "Dataset conforms to a schema."},
		{"about", "onto:Dataset", "onto:Concept", "Dataset is about a concept."},
		{"knows", "onto:Person", "onto:Person", "Person knows a person."},
	}
	for _, prop := range props {
		id := "onto:prop:" + prop.name
		g.AddNode(Node{
			ID:     id,
			Labels: []string{"Property"},
			Props: map[string]string{
				"name":        prop.name,
				"description": prop.desc,
				"kind":        "property",
			},
		})
		g.AddEdge("onto:Ontology", id, "DECLARES", nil)
		g.AddEdge(id, prop.from, "DOMAIN", map[string]string{"via": prop.name})
		g.AddEdge(id, prop.to, "RANGE", map[string]string{"via": prop.name})
	}
	return g
}
