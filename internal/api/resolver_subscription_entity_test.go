package api

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/plugin"
)

// recvEventWithin returns the next event from ch or fails if none arrives within d.
func recvEventWithin(t *testing.T, ch <-chan *EntityChangeEvent, d time.Duration) *EntityChangeEvent {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for an entityChanged event")
		return nil
	}
}

// TestEntityChangedTypeFilter: with a types filter, only matching entity kinds are delivered; the
// non-matching kind is silently skipped (not buffered ahead of the wanted one).
func TestEntityChangedTypeFilter(t *testing.T) {
	// Isolate from the package-level singleton so concurrent publishes don't interfere.
	saved := plugin.EntityEvents
	plugin.EntityEvents = plugin.NewEntityEventBroadcaster()
	t.Cleanup(func() { plugin.EntityEvents = saved })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &subscriptionResolver{}
	out, err := r.EntityChanged(ctx, []string{"Scene"})
	if err != nil {
		t.Fatalf("EntityChanged returned error: %v", err)
	}

	// A Tag event should be filtered out; a Scene event should pass through.
	plugin.EntityEvents.Publish(plugin.EntityEvent{Entity: "Tag", ID: 1, Operation: "Update", Time: time.Now().UTC()})
	plugin.EntityEvents.Publish(plugin.EntityEvent{Entity: "Scene", ID: 2, Operation: "Create", Time: time.Now().UTC()})

	got := recvEventWithin(t, out, time.Second)
	if got.Entity != "Scene" || got.ID != "2" || got.Operation != "Create" {
		t.Errorf("received %+v, want Scene/2/Create", got)
	}
}

// TestEntityChangedNoFilterReceivesAll: a nil types filter streams every kind, and the id is
// surfaced as a string (the GraphQL ID scalar).
func TestEntityChangedNoFilterReceivesAll(t *testing.T) {
	saved := plugin.EntityEvents
	plugin.EntityEvents = plugin.NewEntityEventBroadcaster()
	t.Cleanup(func() { plugin.EntityEvents = saved })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &subscriptionResolver{}
	out, err := r.EntityChanged(ctx, nil)
	if err != nil {
		t.Fatalf("EntityChanged returned error: %v", err)
	}

	plugin.EntityEvents.Publish(plugin.EntityEvent{Entity: "Performer", ID: 5, Operation: "Destroy", Time: time.Now().UTC()})

	got := recvEventWithin(t, out, time.Second)
	if got.Entity != "Performer" || got.ID != "5" || got.Operation != "Destroy" {
		t.Errorf("received %+v, want Performer/5/Destroy", got)
	}
}

// TestEntityChangedCleanupOnCancel: cancelling the context tears the subscription down (channel
// closes and the broadcaster drops the subscriber).
func TestEntityChangedCleanupOnCancel(t *testing.T) {
	saved := plugin.EntityEvents
	b := plugin.NewEntityEventBroadcaster()
	plugin.EntityEvents = b
	t.Cleanup(func() { plugin.EntityEvents = saved })

	ctx, cancel := context.WithCancel(context.Background())

	r := &subscriptionResolver{}
	out, err := r.EntityChanged(ctx, nil)
	if err != nil {
		t.Fatalf("EntityChanged returned error: %v", err)
	}

	cancel()

	// The output channel must close once the goroutine observes cancellation.
	select {
	case _, ok := <-out:
		if ok {
			// drain any in-flight then expect close
			select {
			case _, ok2 := <-out:
				if ok2 {
					t.Error("expected output channel to close after context cancel")
				}
			case <-time.After(time.Second):
				t.Error("output channel did not close after context cancel")
			}
		}
	case <-time.After(time.Second):
		t.Error("output channel did not close after context cancel")
	}
}
