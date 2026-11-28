package neo4j

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommitSendsStatements(t *testing.T) {
	var got txRequest
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/db/neo4j/tx/commit" {
			t.Errorf("path %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{}],"errors":[]}`))
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, User: "neo4j", Password: "secret", Database: "neo4j"}
	err := client.Commit(context.Background(), []Statement{{
		Query:  "MERGE (n:Class {id: $id})",
		Params: map[string]any{"id": "onto:Person"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("neo4j:secret"))
	if auth != wantAuth {
		t.Fatalf("auth %q", auth)
	}
	if len(got.Statements) != 1 || got.Statements[0].Parameters["id"] != "onto:Person" {
		t.Fatalf("body %+v", got)
	}
}

func TestCommitReportsNeo4jErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"code":"Neo.ClientError.Statement.SyntaxError","message":"bad cypher"}]}`))
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, User: "neo4j", Password: "secret"}
	err := client.Commit(context.Background(), []Statement{{Query: "NOPE"}})
	if err == nil || !strings.Contains(err.Error(), "bad cypher") {
		t.Fatal(err)
	}
}

func TestCommitEmptyIsNoop(t *testing.T) {
	if err := (&Client{BaseURL: "http://example.invalid"}).Commit(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
