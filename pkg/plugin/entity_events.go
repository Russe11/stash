package plugin

import (
	"sync"
	"time"

	"github.com/stashapp/stash/pkg/plugin/hook"
)

// EntityEvent is the metadata-only payload broadcast to in-process subscribers (the
// entityChanged GraphQL subscription) whenever a tracked entity is created, updated, or destroyed.
// It deliberately mirrors the outbound WebhookEvent: kind + id + operation + time only — NO titles,
// paths, or other content (privacy + a small, cheap-to-fan-out payload).
type EntityEvent struct {
	// Entity is the entity kind, e.g. "Scene", "Tag", "Performer".
	Entity string
	// ID is the affected entity's id.
	ID int
	// Operation is the operation: "Create", "Update", or "Destroy".
	Operation string
	// Time is when the event fired.
	Time time.Time
}

// entityEventBuffer is the per-subscriber channel depth. A slow subscriber that fills its buffer
// has further events dropped (see broadcast) rather than blocking the mutation that produced them.
const entityEventBuffer = 256

// EntityEventBroadcaster is a tiny in-process pub/sub fan-out: subscribers register a channel, the
// post-commit choke point publishes every entity change, and each live subscriber receives a copy.
// Fan-out is non-blocking — a full subscriber buffer drops the event for that subscriber only, so a
// slow/stalled consumer can never block (or slow) the originating mutation transaction.
type EntityEventBroadcaster struct {
	mu          sync.Mutex
	subscribers map[int]chan EntityEvent
	nextID      int
}

// NewEntityEventBroadcaster returns an empty broadcaster ready to accept subscribers.
func NewEntityEventBroadcaster() *EntityEventBroadcaster {
	return &EntityEventBroadcaster{
		subscribers: make(map[int]chan EntityEvent),
	}
}

// Subscribe registers a new subscriber and returns its receive channel plus an unsubscribe func.
// The caller must invoke unsubscribe (e.g. on context cancellation / client disconnect) to release
// the channel; failing to do so leaks one buffered channel per orphaned subscriber.
func (b *EntityEventBroadcaster) Subscribe() (<-chan EntityEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan EntityEvent, entityEventBuffer)
	b.subscribers[id] = ch

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing)
		}
	}

	return ch, unsubscribe
}

// Publish fans an event out to every current subscriber. Delivery is non-blocking: if a
// subscriber's buffer is full the event is dropped for that subscriber (never blocking the caller).
func (b *EntityEventBroadcaster) Publish(event EntityEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber buffer is full; drop this event for it rather than block the mutation.
		}
	}
}

// subscriberCount reports the number of live subscribers (used in tests).
func (b *EntityEventBroadcaster) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}

// EntityEvents is the process-wide broadcaster the post-commit choke point publishes to and the
// entityChanged subscription resolver subscribes from. It is a package-level singleton so the
// single ExecutePostHooks tap and the API-layer resolver share one instance without threading it
// through every call site.
var EntityEvents = NewEntityEventBroadcaster()

// buildEntityEvent derives the metadata-only subscriber payload from a hook trigger, reusing
// the same entity/operation split as the webhook event (e.g. "Scene.Update.Post" -> entity
// "Scene", operation "Update").
func buildEntityEvent(hookType hook.TriggerEnum, id int) EntityEvent {
	we := buildWebhookEvent(hookType, id)
	return EntityEvent{
		Entity:    we.Entity,
		ID:        we.ID,
		Operation: we.Operation,
		Time:      time.Now().UTC(),
	}
}
