package plugin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	if err := postWebhook(srv.Client(), srv.URL, body, ""); err != nil {
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
	dispatchWebhooks([]string{srv.URL, "   ", srv.URL}, "", hook.SceneCreatePost, 99)

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
	dispatchWebhooks(nil, "", hook.SceneCreatePost, 1)
	dispatchWebhooks([]string{}, "", hook.SceneCreatePost, 1)
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
	if err := postWebhook(webhookClient, redirector.URL, body, ""); err != nil {
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
	dispatchWebhooks([]string{"file:///etc/passwd", srv.URL}, "", hook.SceneCreatePost, 5)

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

func TestSignWebhook(t *testing.T) {
	body := []byte(`{"type":"Scene.Update.Post"}`)

	if got := signWebhook("", body); got != "" {
		t.Errorf("no secret should yield empty signature, got %q", got)
	}

	const secret = "topsecret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := signWebhook(secret, body); got != want {
		t.Errorf("signWebhook = %q, want %q", got, want)
	}
}

func TestDispatchWebhooksSignsWhenSecretSet(t *testing.T) {
	const secret = "shared-secret"
	type capture struct {
		sig  string
		body []byte
	}
	got := make(chan capture, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- capture{sig: r.Header.Get(signatureHeader), body: body}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dispatchWebhooks([]string{srv.URL}, secret, hook.SceneUpdatePost, 3)

	select {
	case c := <-got:
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(c.body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if c.sig != want {
			t.Errorf("X-Stash-Signature = %q, want %q (HMAC over the exact body)", c.sig, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a signed delivery")
	}
}

func TestPostWebhookRetriesOn5xx(t *testing.T) {
	// shrink the backoff so the test is fast; restore after.
	defer func(b time.Duration) { webhookRetryBackoff = b }(webhookRetryBackoff)
	webhookRetryBackoff = time.Millisecond

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fail the first two attempts with a 503, then succeed
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := postWebhookWithRetry(srv.Client(), srv.URL, []byte("{}"), ""); err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 3 {
		t.Errorf("expected 3 attempts (2 failures + success), got %d", n)
	}
}

func TestPostWebhookNoRetryOn4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest) // permanent rejection
	}))
	defer srv.Close()

	// a 4xx is not retried and is not surfaced as an error (logged only)
	if err := postWebhookWithRetry(srv.Client(), srv.URL, []byte("{}"), ""); err != nil {
		t.Fatalf("4xx should not be a retryable error, got %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Errorf("expected exactly 1 attempt for a 4xx, got %d", n)
	}
}
