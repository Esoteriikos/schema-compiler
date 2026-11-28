// Package neo4j commits Cypher over Neo4j's transactional HTTP API.
package neo4j

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Statement is one parameterized Cypher query.
type Statement struct {
	Query  string
	Params map[string]any
}

// Client talks to a Neo4j HTTP endpoint.
type Client struct {
	BaseURL  string
	User     string
	Password string
	Database string
	HTTP     *http.Client
}

type txRequest struct {
	Statements []txStatement `json:"statements"`
}

type txStatement struct {
	Statement  string         `json:"statement"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type txResponse struct {
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// Commit runs statements in batches against /db/{database}/tx/commit.
func (c *Client) Commit(ctx context.Context, statements []Statement) error {
	if c == nil {
		return fmt.Errorf("missing neo4j client")
	}
	if len(statements) == 0 {
		return nil
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("neo4j URL is required")
	}
	httpc := c.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: 30 * time.Second}
	}
	db := c.Database
	if db == "" {
		db = "neo4j"
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/db/" + url.PathEscape(db) + "/tx/commit"
	const batch = 50
	for start := 0; start < len(statements); start += batch {
		end := start + batch
		if end > len(statements) {
			end = len(statements)
		}
		if err := c.commitBatch(ctx, httpc, endpoint, statements[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) commitBatch(ctx context.Context, httpc *http.Client, endpoint string, statements []Statement) error {
	body := txRequest{Statements: make([]txStatement, len(statements))}
	for i, stmt := range statements {
		body.Statements[i] = txStatement{Statement: stmt.Query, Parameters: stmt.Params}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "schemac")
	if c.User != "" || c.Password != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("neo4j HTTP %s: %s", resp.Status, trimBody(payload))
	}
	var parsed txResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return fmt.Errorf("neo4j response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		parts := make([]string, len(parsed.Errors))
		for i, item := range parsed.Errors {
			parts[i] = strings.TrimSpace(item.Code + " " + item.Message)
		}
		return fmt.Errorf("neo4j rejected statements: %s", strings.Join(parts, "; "))
	}
	return nil
}

func trimBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
