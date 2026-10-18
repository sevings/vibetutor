package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIComplete(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  hello  "}}]}`))
	}))
	defer srv.Close()

	c := NewOpenAI(srv.URL+"/v1/", "key", "test-model", 5*time.Second)
	reply, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply != "hello" {
		t.Errorf("reply = %q, want trimmed hello", reply)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer key" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody.Model != "test-model" || len(gotBody.Messages) != 1 {
		t.Errorf("body = %+v", gotBody)
	}
}

func TestOpenAIErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"model error":  {200, `{"error":{"message":"boom"}}`},
		"bad status":   {500, `{"error":{"message":"server"}}`},
		"no choices":   {200, `{"choices":[]}`},
		"invalid json": {200, `not json`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := NewOpenAI(srv.URL, "", "m", time.Second)
			if _, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "x"}}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestOpenAINoAPIKeyHeader(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := NewOpenAI(srv.URL, "", "m", time.Second)
	if _, err := c.Complete(context.Background(), []Message{{Role: RoleUser, Content: "x"}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if auth != "" {
		t.Errorf("expected no authorization header, got %q", auth)
	}
}
