package plugin

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

// Bounded retry for transient webhook failures (transport errors and 5xx). A
// webhook-only consumer would otherwise silently desync on a momentary blip.
// These are vars (not consts) so tests can shrink the backoff.
var (
	webhookMaxAttempts  = 3
	webhookRetryBackoff = 500 * time.Millisecond
)

// signatureHeader carries the HMAC-SHA256 of the body when a webhook_secret is
// configured, so a receiver can authenticate that stash (not an impostor) sent it.
const signatureHeader = "X-Stash-Signature"

// WebhookEvent is the JSON payload POSTed to each configured webhook URL when a tracked entity is
// created, updated, or destroyed. It lets external systems (home automation, chat bots, the Stash
// clients themselves) react to library changes without polling.
type WebhookEvent struct {
	// Type is the full hook trigger, e.g. "Scene.Update.Post".
	Type string `json:"type"`
	// Entity is the entity kind, parsed from Type, e.g. "Scene".
	Entity string `json:"entity"`
	// Operation is the operation, parsed from Type: "Create", "Update", or "Destroy".
	Operation string `json:"operation"`
	// ID is the affected entity's id.
	ID int `json:"id"`
	// Time is when the event fired, RFC3339 UTC.
	Time string `json:"time"`
}

// webhookClient is shared so connections are reused; the timeout keeps a slow/blackholed endpoint
// from leaking goroutines forever. Redirects are not followed: a webhook target must not be able
// to bounce the server to a different (possibly internal) URL — a classic SSRF pivot. Returning
// ErrUseLastResponse makes Do() surface the 3xx itself instead of chasing the Location.
var webhookClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// validateWebhookURL rejects a webhook target that isn't a plain http(s) URL with a host, returning
// the normalized URL string on success. It deliberately does NOT block private/loopback hosts: this
// is a LAN-first, single-user server whose most common webhook target is local home automation on a
// private IP, and webhook_urls is operator-configured (low attacker-influence). What it does harden
// is the scheme — rejecting file://, gopher://, and other SSRF-prone schemes that have no legitimate
// webhook use — paired with the no-redirect policy on webhookClient above.
func validateWebhookURL(url string) (string, error) {
	u, err := neturl.Parse(url)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %w", url, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("url %q: unsupported scheme %q (only http/https)", url, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("url %q: missing host", url)
	}
	return u.String(), nil
}

// buildWebhookEvent constructs the payload for a hook trigger, splitting the trigger string into its
// entity and operation parts (e.g. "Scene.Update.Post" -> entity "Scene", operation "Update").
func buildWebhookEvent(hookType hook.TriggerEnum, id int) WebhookEvent {
	t := hookType.String()
	var entity, operation string
	parts := strings.Split(t, ".")
	if len(parts) >= 2 {
		entity = parts[0]
		operation = parts[1]
	}
	return WebhookEvent{
		Type:      t,
		Entity:    entity,
		Operation: operation,
		ID:        id,
		Time:      time.Now().UTC().Format(time.RFC3339),
	}
}

// signWebhook returns the "sha256=<hex>" HMAC of body keyed by secret, for the
// X-Stash-Signature header. An empty secret yields an empty signature (unsigned).
func signWebhook(secret string, body []byte) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// postWebhook sends a single event to one URL, optionally signed. It returns an error only for
// transient failures the caller should retry — a transport error or a 5xx response. A 4xx is a
// permanent rejection (logged, not retried); a 2xx/3xx is success.
func postWebhook(client *http.Client, url string, body []byte, signature string) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "stash-webhook")
	if signature != "" {
		req.Header.Set(signatureHeader, signature)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("webhook %s returned status %d", url, resp.StatusCode)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		logger.Warnf("webhook: %s returned status %d (not retrying)", url, resp.StatusCode)
	}
	return nil
}

// postWebhookWithRetry calls postWebhook up to webhookMaxAttempts times, backing off linearly
// between tries, until it succeeds or attempts are exhausted. It returns the last error.
func postWebhookWithRetry(client *http.Client, url string, body []byte, signature string) error {
	var lastErr error
	for attempt := 1; attempt <= webhookMaxAttempts; attempt++ {
		if lastErr = postWebhook(client, url, body, signature); lastErr == nil {
			return nil
		}
		if attempt < webhookMaxAttempts {
			time.Sleep(webhookRetryBackoff * time.Duration(attempt))
		}
	}
	return lastErr
}

// dispatchWebhooks fires an HTTP POST of the event to every configured URL, each on its own
// goroutine so a slow endpoint never blocks (or fails) the mutation that triggered it. When secret
// is non-empty each request carries an HMAC-SHA256 X-Stash-Signature so receivers can authenticate
// the sender. A nil/empty url list is a no-op, so the common "no webhooks configured" case costs
// nothing.
func dispatchWebhooks(urls []string, secret string, hookType hook.TriggerEnum, id int) {
	if len(urls) == 0 {
		return
	}

	event := buildWebhookEvent(hookType, id)
	body, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("webhook: marshaling %s event: %v", event.Type, err)
		return
	}
	signature := signWebhook(secret, body)

	for _, raw := range urls {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		url, err := validateWebhookURL(trimmed)
		if err != nil {
			logger.Warnf("webhook: skipping configured url: %v", err)
			continue
		}
		go func(url string) {
			if err := postWebhookWithRetry(webhookClient, url, body, signature); err != nil {
				logger.Warnf("webhook: POST to %s failed after %d attempts: %v", url, webhookMaxAttempts, err)
			}
		}(url)
	}
}
