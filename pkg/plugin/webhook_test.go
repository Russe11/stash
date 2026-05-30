package plugin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/plugin/hook"
)

func TestBuildWebhookEvent(t *testing.T) {
	e := buildWebhookEvent(hook.SceneUpdatePost, 42)
	if e.Type != "Scene.Update.Post" {
		t.Errorf("type = %q, want Scene.Update.Post", e.Type)
	}
	if e.Entity != "Scene" {
		t.Errorf("entity = %q, want Scene", e.Entity)
	}
	if e.Operation != "Update" {
		t.Errorf("operation = %q, want Update", e.Operation)
	}
	if e.ID != 42 {
		t.Errorf("id = %d, want 42", e.ID)
	}
	if e.Time == "" {
		t.Error("time should not be empty")
	}
}

func TestPostWebhookDeliversJSON(t *testing.T) {
	received := make(chan WebhookEvent, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		var e WebhookEvent
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &e)
		received <- e
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body, _ := json.Marshal(buildWebhookEvent(hook.TagDestroyPost, 7))
	if err := postWebhook(srv.Client(), srv.URL, body); err != nil {
		t.Fatalf("postWebhook: %v", err)
	}

	select {
	case e := <-received:
		if e.Entity != "Tag" || e.Operation != "Destroy" || e.ID != 7 {
			t.Errorf("received %+v, want Tag/Destroy/7", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not received")
	}
}

func TestDispatchWebhooksFanOutAndSkipsBlanks(t *testing.T) {
	received := make(chan WebhookEvent, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e WebhookEvent
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &e)
		received <- e
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// two real URLs -> two deliveries; the blank entry is skipped (not 3 deliveries)
	dispatchWebhooks([]string{srv.URL, "   ", srv.URL}, hook.SceneCreatePost, 99)

	for i := 0; i < 2; i++ {
		select {
		case e := <-received:
			if e.Type != "Scene.Create.Post" || e.ID != 99 {
				t.Errorf("received %+v, want Scene.Create.Post/99", e)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("expected 2 webhook deliveries")
		}
	}
	// ensure the blank URL did not produce a third delivery
	select {
	case e := <-received:
		t.Errorf("unexpected extra delivery: %+v", e)
	case <-time.After(200 * time.Millisecond):
		// good — exactly two
	}
}

func TestDispatchWebhooksEmptyIsNoOp(t *testing.T) {
	// must not panic and must return immediately when nothing is configured
	dispatchWebhooks(nil, hook.SceneCreatePost, 1)
	dispatchWebhooks([]string{}, hook.SceneCreatePost, 1)
}

func TestValidateWebhookURL(t *testing.T) {
	// LAN/loopback hosts are allowed by design (this is a LAN-first server).
	valid := []string{
		"http://192.168.1.10:8123/api/webhook",
		"https://example.com/hook",
		"http://localhost:9999/x",
	}
	for _, u := range valid {
		got, err := validateWebhookURL(u)
		if err != nil || got == "" {
			t.Errorf("validateWebhookURL(%q) = (%q, %v); want accepted", u, got, err)
		}
	}

	// Non-http(s) schemes and host-less URLs are rejected (SSRF-prone / unusable).
	invalid := []string{
		"file:///etc/passwd",
		"gopher://internal/x",
		"ftp://host/file",
		"http://",     // missing host
		"justastring", // no scheme/host
		"://nohost",   // malformed
	}
	for _, u := range invalid {
		if _, err := validateWebhookURL(u); err == nil {
			t.Errorf("validateWebhookURL(%q) = nil error; want rejection", u)
		}
	}
}

// TestWebhookClientDoesNotFollowRedirects proves the SSRF-pivot guard: a target
// that 302-redirects must not cause the server to fetch the redirect Location.
func TestWebhookClientDoesNotFollowRedirects(t *testing.T) {
	hitInternal := make(chan struct{}, 1)
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitInternal <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer internal.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	body, _ := json.Marshal(buildWebhookEvent(hook.SceneUpdatePost, 1))
	// 302 is < 400, so postWebhook returns nil; the point is it must not chase the Location.
	if err := postWebhook(webhookClient, redirector.URL, body); err != nil {
		t.Fatalf("postWebhook: %v", err)
	}
	select {
	case <-hitInternal:
		t.Fatal("redirect was followed to the internal target — SSRF pivot not blocked")
	case <-time.After(300 * time.Millisecond):
		// good — redirect not followed
	}
}

func TestDispatchWebhooksSkipsInvalidScheme(t *testing.T) {
	received := make(chan WebhookEvent, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e WebhookEvent
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &e)
		received <- e
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// the file:// URL is dropped; only the valid http URL delivers
	dispatchWebhooks([]string{"file:///etc/passwd", srv.URL}, hook.SceneCreatePost, 5)

	select {
	case e := <-received:
		if e.ID != 5 {
			t.Errorf("received id %d, want 5", e.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a delivery from the valid url")
	}
	select {
	case e := <-received:
		t.Errorf("unexpected second delivery: %+v", e)
	case <-time.After(200 * time.Millisecond):
		// good — the invalid-scheme url produced nothing
	}
}
