package plugin

import (
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/plugin/hook"
)

// receiveWithin returns the next event from ch, or fails the test if none arrives within d.
func receiveWithin(t *testing.T, ch <-chan EntityEvent, d time.Duration) EntityEvent {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for an event")
		return EntityEvent{}
	}
}

// TestBroadcasterSubscribePublishReceive: a subscriber receives a published event.
func TestBroadcasterSubscribePublishReceive(t *testing.T) {
	b := NewEntityEventBroadcaster()

	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	if got := b.subscriberCount(); got != 1 {
		t.Fatalf("subscriberCount after Subscribe = %d, want 1", got)
	}

	want := EntityEvent{Entity: "Scene", ID: 42, Operation: "Update", Time: time.Now().UTC()}
	b.Publish(want)

	got := receiveWithin(t, ch, time.Second)
	if got.Entity != want.Entity || got.ID != want.ID || got.Operation != want.Operation {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

// TestBroadcasterFanOut: every live subscriber receives a copy of each event.
func TestBroadcasterFanOut(t *testing.T) {
	b := NewEntityEventBroadcaster()

	ch1, unsub1 := b.Subscribe()
	defer unsub1()
	ch2, unsub2 := b.Subscribe()
	defer unsub2()

	if got := b.subscriberCount(); got != 2 {
		t.Fatalf("subscriberCount = %d, want 2", got)
	}

	ev := EntityEvent{Entity: "Tag", ID: 7, Operation: "Create", Time: time.Now().UTC()}
	b.Publish(ev)

	if got := receiveWithin(t, ch1, time.Second); got.ID != 7 {
		t.Errorf("ch1 received id %d, want 7", got.ID)
	}
	if got := receiveWithin(t, ch2, time.Second); got.ID != 7 {
		t.Errorf("ch2 received id %d, want 7", got.ID)
	}
}

// TestBroadcasterUnsubscribeCleanup: after unsubscribing, the channel is closed and the subscriber
// is dropped, so later publishes are not delivered (and never block).
func TestBroadcasterUnsubscribeCleanup(t *testing.T) {
	b := NewEntityEventBroadcaster()

	ch, unsubscribe := b.Subscribe()
	if got := b.subscriberCount(); got != 1 {
		t.Fatalf("subscriberCount = %d, want 1", got)
	}

	unsubscribe()
	if got := b.subscriberCount(); got != 0 {
		t.Fatalf("subscriberCount after unsubscribe = %d, want 0", got)
	}

	// Channel must be closed (zero value, not ok).
	if _, ok := <-ch; ok {
		t.Error("expected channel to be closed after unsubscribe")
	}

	// Publishing after unsubscribe must not panic or block.
	b.Publish(EntityEvent{Entity: "Scene", ID: 1, Operation: "Destroy", Time: time.Now().UTC()})

	// Double-unsubscribe must be a safe no-op (no double-close panic).
	unsubscribe()
}

// TestBroadcasterNonBlockingFullBuffer: a subscriber whose buffer is full never blocks the
// publisher; excess events are dropped for that subscriber only.
func TestBroadcasterNonBlockingFullBuffer(t *testing.T) {
	b := NewEntityEventBroadcaster()
	ch, unsubscribe := b.Subscribe()
	defer unsubscribe()

	// Overfill well past the buffer; Publish must return promptly without blocking.
	done := make(chan struct{})
	go func() {
		for i := 0; i < entityEventBuffer*4; i++ {
			b.Publish(EntityEvent{Entity: "Image", ID: i, Operation: "Update", Time: time.Now().UTC()})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full subscriber buffer")
	}

	// The buffer should be saturated (some events delivered, the rest safely dropped).
	if got := len(ch); got != entityEventBuffer {
		t.Errorf("buffered events = %d, want %d (full buffer, excess dropped)", got, entityEventBuffer)
	}
}

// TestBuildEntityEventSplitsTrigger: the metadata-only event is derived from the hook trigger the
// same way the webhook payload is (entity/operation split), with no content beyond kind+id+op+time.
func TestBuildEntityEventSplitsTrigger(t *testing.T) {
	ev := buildEntityEvent(hook.SceneUpdatePost, 99)
	if ev.Entity != "Scene" {
		t.Errorf("Entity = %q, want %q", ev.Entity, "Scene")
	}
	if ev.Operation != "Update" {
		t.Errorf("Operation = %q, want %q", ev.Operation, "Update")
	}
	if ev.ID != 99 {
		t.Errorf("ID = %d, want 99", ev.ID)
	}
	if ev.Time.IsZero() {
		t.Error("Time should be set")
	}
}
