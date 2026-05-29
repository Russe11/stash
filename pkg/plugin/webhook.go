package plugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

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
// from leaking goroutines forever.
var webhookClient = &http.Client{Timeout: 10 * time.Second}

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

// postWebhook sends a single event to one URL. Best-effort: it returns an error for logging but
// callers do not let a webhook failure affect the originating mutation.
func postWebhook(client *http.Client, url string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "stash-webhook")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		logger.Warnf("webhook: %s returned status %d", url, resp.StatusCode)
	}
	return nil
}

// dispatchWebhooks fires an HTTP POST of the event to every configured URL, each on its own
// goroutine so a slow endpoint never blocks (or fails) the mutation that triggered it. A nil/empty
// url list is a no-op, so the common "no webhooks configured" case costs nothing.
func dispatchWebhooks(urls []string, hookType hook.TriggerEnum, id int) {
	if len(urls) == 0 {
		return
	}

	event := buildWebhookEvent(hookType, id)
	body, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("webhook: marshaling %s event: %v", event.Type, err)
		return
	}

	for _, raw := range urls {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		go func(url string) {
			if err := postWebhook(webhookClient, url, body); err != nil {
				logger.Warnf("webhook: POST to %s failed: %v", url, err)
			}
		}(url)
	}
}
