package api

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/plugin"
)

// recvPresenceEventWithin returns the next presence event from ch or fails if none arrives in d.
func recvPresenceEventWithin(t *testing.T, ch <-chan *DevicePresenceEvent, d time.Duration) *DevicePresenceEvent {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for a devicePresence event")
		return nil
	}
}

// recvCommandEventWithin returns the next command event from ch or fails if none arrives in d.
func recvCommandEventWithin(t *testing.T, ch <-chan *DeviceCommandEvent, d time.Duration) *DeviceCommandEvent {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for a deviceCommands event")
		return nil
	}
}

// isolateDeviceBus swaps the process-wide deviceBus singletons for fresh in-memory ones so tests
// don't interfere with each other (or with a running server), and restores them on cleanup. The
// registry it installs uses a real clock but a tiny TTL is irrelevant here (eviction is exercised in
// the pkg/plugin tests); these resolver tests drive register/unregister/playback directly.
func isolateDeviceBus(t *testing.T) {
	t.Helper()
	savedP, savedC, savedR := plugin.DevicePresence, plugin.DeviceCommands, plugin.DeviceBusRegistry
	pb := plugin.NewDevicePresenceBroadcaster()
	plugin.DevicePresence = pb
	plugin.DeviceCommands = plugin.NewDeviceCommandBroadcaster()
	plugin.DeviceBusRegistry = plugin.NewDeviceRegistry(plugin.DevicePresenceTTL, pb, nil)
	t.Cleanup(func() {
		plugin.DevicePresence, plugin.DeviceCommands, plugin.DeviceBusRegistry = savedP, savedC, savedR
	})
}

// TestDevicePresenceSubscriptionReceivesOnline: registering a device through the mutation resolver
// drives an ONLINE event onto the devicePresence subscription, with the device mapped to the
// GraphQL payload (online=true, ids preserved).
func TestDevicePresenceSubscriptionReceivesOnline(t *testing.T) {
	isolateDeviceBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := &subscriptionResolver{}
	out, err := sub.DevicePresence(ctx)
	if err != nil {
		t.Fatalf("DevicePresence returned error: %v", err)
	}

	mut := &mutationResolver{}
	dev, err := mut.RegisterDevice(ctx, DeviceInput{
		ID:           "dev-1",
		Name:         "Living Room TV",
		Kind:         DeviceKindTv,
		Capabilities: []DeviceCapability{DeviceCapabilityPlay, DeviceCapabilityControl},
	})
	if err != nil {
		t.Fatalf("RegisterDevice returned error: %v", err)
	}
	if !dev.Online || dev.ID != "dev-1" {
		t.Errorf("RegisterDevice returned %+v, want online dev-1", dev)
	}

	ev := recvPresenceEventWithin(t, out, time.Second)
	if ev.Kind != PresenceChangeOnline {
		t.Errorf("kind = %v, want ONLINE", ev.Kind)
	}
	if ev.Device == nil || ev.Device.ID != "dev-1" || !ev.Device.Online {
		t.Errorf("device = %+v, want online dev-1", ev.Device)
	}
	if len(ev.Device.Capabilities) != 2 {
		t.Errorf("capabilities = %v, want 2", ev.Device.Capabilities)
	}
}

// TestDevicePresencePlaybackEvent: updatePlaybackState drives a PLAYBACK event carrying the scene id
// (not a title) onto the subscription.
func TestDevicePresencePlaybackEvent(t *testing.T) {
	isolateDeviceBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := &subscriptionResolver{}
	out, err := sub.DevicePresence(ctx)
	if err != nil {
		t.Fatalf("DevicePresence error: %v", err)
	}

	mut := &mutationResolver{}
	if _, err := mut.RegisterDevice(ctx, DeviceInput{ID: "dev-1", Name: "TV", Kind: DeviceKindTv}); err != nil {
		t.Fatalf("RegisterDevice error: %v", err)
	}
	_ = recvPresenceEventWithin(t, out, time.Second) // ONLINE

	scene := "scene-42"
	ok, err := mut.UpdatePlaybackState(ctx, PlaybackStateInput{
		DeviceID:        "dev-1",
		SceneID:         &scene,
		PositionSeconds: 88.0,
		Paused:          true,
	})
	if err != nil || !ok {
		t.Fatalf("UpdatePlaybackState = (%v, %v), want (true, nil)", ok, err)
	}

	ev := recvPresenceEventWithin(t, out, time.Second)
	if ev.Kind != PresenceChangePlayback {
		t.Fatalf("kind = %v, want PLAYBACK", ev.Kind)
	}
	if ev.Device.Playback == nil || ev.Device.Playback.SceneID == nil || *ev.Device.Playback.SceneID != "scene-42" {
		t.Errorf("playback = %+v, want scene-42", ev.Device.Playback)
	}
	if ev.Device.Playback.PositionSeconds != 88.0 || !ev.Device.Playback.Paused {
		t.Errorf("playback state = %+v, want 88s/paused", ev.Device.Playback)
	}
}

// TestDeviceCommandsTargetFiltering: sendDeviceCommand reaches only the subscriber whose deviceId ==
// targetDeviceId; a subscriber for another device receives nothing.
func TestDeviceCommandsTargetFiltering(t *testing.T) {
	isolateDeviceBus(t)

	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	sub := &subscriptionResolver{}
	outA, err := sub.DeviceCommands(ctxA, "dev-A")
	if err != nil {
		t.Fatalf("DeviceCommands(dev-A) error: %v", err)
	}
	outB, err := sub.DeviceCommands(ctxB, "dev-B")
	if err != nil {
		t.Fatalf("DeviceCommands(dev-B) error: %v", err)
	}

	mut := &mutationResolver{}
	scene := "scene-9"
	start := 12.5
	delivered, err := mut.SendDeviceCommand(ctxA, DeviceCommandInput{
		TargetDeviceID: "dev-A",
		Type:           DeviceCommandTypePlay,
		SceneID:        &scene,
		StartSeconds:   &start,
	})
	if err != nil || !delivered {
		t.Fatalf("SendDeviceCommand = (%v, %v), want (true, nil)", delivered, err)
	}

	ev := recvCommandEventWithin(t, outA, time.Second)
	if ev.Type != DeviceCommandTypePlay || ev.SceneID == nil || *ev.SceneID != "scene-9" {
		t.Errorf("dev-A command = %+v, want PLAY scene-9", ev)
	}
	if ev.StartSeconds == nil || *ev.StartSeconds != 12.5 {
		t.Errorf("startSeconds = %v, want 12.5", ev.StartSeconds)
	}

	// dev-B must receive nothing.
	select {
	case e := <-outB:
		t.Errorf("dev-B wrongly received %+v", e)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestSendDeviceCommandNoTargetReturnsFalse: a command to a device with no live subscriber reports
// not-delivered.
func TestSendDeviceCommandNoTargetReturnsFalse(t *testing.T) {
	isolateDeviceBus(t)

	mut := &mutationResolver{}
	delivered, err := mut.SendDeviceCommand(context.Background(), DeviceCommandInput{
		TargetDeviceID: "nobody",
		Type:           DeviceCommandTypeStop,
	})
	if err != nil {
		t.Fatalf("SendDeviceCommand error: %v", err)
	}
	if delivered {
		t.Error("SendDeviceCommand to a target with no subscriber = true, want false")
	}
}

// TestDevicePresenceCleanupOnCancel: cancelling the subscription context closes the output channel
// and drops the broadcaster subscriber (no leak).
func TestDevicePresenceCleanupOnCancel(t *testing.T) {
	isolateDeviceBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	sub := &subscriptionResolver{}
	out, err := sub.DevicePresence(ctx)
	if err != nil {
		t.Fatalf("DevicePresence error: %v", err)
	}

	cancel()

	select {
	case _, ok := <-out:
		if ok {
			select {
			case _, ok2 := <-out:
				if ok2 {
					t.Error("expected output channel to close after cancel")
				}
			case <-time.After(time.Second):
				t.Error("output channel did not close after cancel")
			}
		}
	case <-time.After(time.Second):
		t.Error("output channel did not close after cancel")
	}
}

// TestDeviceCommandsCleanupOnCancel: cancelling a deviceCommands subscription closes the channel and
// (via the unsubscribe) drops the command-broadcaster subscriber.
func TestDeviceCommandsCleanupOnCancel(t *testing.T) {
	isolateDeviceBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	sub := &subscriptionResolver{}
	out, err := sub.DeviceCommands(ctx, "dev-A")
	if err != nil {
		t.Fatalf("DeviceCommands error: %v", err)
	}
	if got := plugin.DeviceCommands.SubscriberCountForTest(); got != 1 {
		t.Fatalf("command subscribers = %d, want 1", got)
	}

	cancel()

	select {
	case _, ok := <-out:
		if ok {
			select {
			case _, ok2 := <-out:
				if ok2 {
					t.Error("expected output channel to close after cancel")
				}
			case <-time.After(time.Second):
				t.Error("output channel did not close after cancel")
			}
		}
	case <-time.After(time.Second):
		t.Error("output channel did not close after cancel")
	}

	// Give the resolver goroutine a moment to run its deferred unsubscribe.
	deadline := time.Now().Add(time.Second)
	for plugin.DeviceCommands.SubscriberCountForTest() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := plugin.DeviceCommands.SubscriberCountForTest(); got != 0 {
		t.Errorf("command subscribers after cancel = %d, want 0 (leak)", got)
	}
}

// TestDevicesQueryReturnsOnline: the devices query returns the online devices mapped to GraphQL.
func TestDevicesQueryReturnsOnline(t *testing.T) {
	isolateDeviceBus(t)

	ctx := context.Background()
	mut := &mutationResolver{}
	if _, err := mut.RegisterDevice(ctx, DeviceInput{ID: "dev-1", Name: "TV", Kind: DeviceKindTv}); err != nil {
		t.Fatalf("RegisterDevice error: %v", err)
	}
	if _, err := mut.RegisterDevice(ctx, DeviceInput{ID: "dev-2", Name: "Mac", Kind: DeviceKindDesktop}); err != nil {
		t.Fatalf("RegisterDevice error: %v", err)
	}

	q := &queryResolver{}
	devices, err := q.Devices(ctx)
	if err != nil {
		t.Fatalf("Devices error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("Devices returned %d, want 2", len(devices))
	}
	for _, d := range devices {
		if !d.Online {
			t.Errorf("device %s online=false, want true", d.ID)
		}
	}
}

// TestDeviceBusIsEphemeral_NoSQLite asserts the entire deviceBus path is in-memory only. The
// resolvers are driven with a ZERO-VALUE *Resolver (nil repository, nil txnManager, nil database):
// every deviceBus mutation/query/subscription completes successfully, proving the path never touches
// SQLite. If any of these started persisting (e.g. via r.repository / r.withTxn), it would nil-panic
// here. This is the "ephemeral — nothing written to SQLite" guard required by the design (§9, §10).
func TestDeviceBusIsEphemeral_NoSQLite(t *testing.T) {
	isolateDeviceBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Zero-value resolvers: no DB, no repository, no transaction manager wired.
	mut := &mutationResolver{}
	q := &queryResolver{}
	sub := &subscriptionResolver{}

	// Subscriptions (would need no DB) — and they must not panic.
	if _, err := sub.DevicePresence(ctx); err != nil {
		t.Fatalf("DevicePresence touched something it shouldn't: %v", err)
	}
	cmdOut, err := sub.DeviceCommands(ctx, "dev-1")
	if err != nil {
		t.Fatalf("DeviceCommands error: %v", err)
	}

	// Full mutation surface with no DB available.
	if _, err := mut.RegisterDevice(ctx, DeviceInput{ID: "dev-1", Name: "TV", Kind: DeviceKindTv}); err != nil {
		t.Fatalf("RegisterDevice persisted/failed: %v", err)
	}
	scene := "scene-1"
	if _, err := mut.UpdatePlaybackState(ctx, PlaybackStateInput{DeviceID: "dev-1", SceneID: &scene, PositionSeconds: 1}); err != nil {
		t.Fatalf("UpdatePlaybackState persisted/failed: %v", err)
	}
	if _, err := mut.SendDeviceCommand(ctx, DeviceCommandInput{TargetDeviceID: "dev-1", Type: DeviceCommandTypePlay, SceneID: &scene}); err != nil {
		t.Fatalf("SendDeviceCommand persisted/failed: %v", err)
	}
	// Drain the command we just sent so the buffer assertion elsewhere isn't affected.
	_ = recvCommandEventWithin(t, cmdOut, time.Second)

	if _, err := q.Devices(ctx); err != nil {
		t.Fatalf("Devices persisted/failed: %v", err)
	}
	if _, err := mut.UnregisterDevice(ctx, "dev-1"); err != nil {
		t.Fatalf("UnregisterDevice persisted/failed: %v", err)
	}
}
