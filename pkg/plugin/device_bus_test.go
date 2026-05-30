package plugin

import (
	"testing"
	"time"
)

// recvPresenceWithin returns the next presence event from ch, or fails if none arrives within d.
func recvPresenceWithin(t *testing.T, ch <-chan DevicePresenceEvent, d time.Duration) DevicePresenceEvent {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for a presence event")
		return DevicePresenceEvent{}
	}
}

// recvCommandWithin returns the next command from ch, or fails if none arrives within d.
func recvCommandWithin(t *testing.T, ch <-chan DeviceCommand, d time.Duration) DeviceCommand {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(d):
		t.Fatal("timed out waiting for a command")
		return DeviceCommand{}
	}
}

// expectNoPresence asserts no presence event arrives within d (used to verify a heartbeat does not
// re-emit ONLINE).
func expectNoPresence(t *testing.T, ch <-chan DevicePresenceEvent, d time.Duration) {
	t.Helper()
	select {
	case e := <-ch:
		t.Fatalf("expected no presence event, got %+v", e)
	case <-time.After(d):
	}
}

// newTestRegistry builds a registry with a controllable clock and its own presence broadcaster, so
// tests can drive TTL eviction deterministically without sleeping real time.
func newTestRegistry(ttl time.Duration) (*DeviceRegistry, *DevicePresenceBroadcaster, *time.Time) {
	now := time.Now()
	clock := &now
	pb := NewDevicePresenceBroadcaster()
	reg := NewDeviceRegistry(ttl, pb, func() time.Time { return *clock })
	return reg, pb, clock
}

func sampleDevice(id string) DeviceRecord {
	return DeviceRecord{
		ID:           id,
		Name:         "Living Room TV",
		Kind:         "TV",
		Capabilities: []string{"PLAY", "CONTROL"},
	}
}

// TestRegisterEmitsOnlineAndAppearsInRegistry: a fresh registration emits ONLINE and the device is
// returned by Online().
func TestRegisterEmitsOnlineAndAppearsInRegistry(t *testing.T) {
	reg, pb, _ := newTestRegistry(DevicePresenceTTL)
	ch, unsub := pb.Subscribe()
	defer unsub()

	rec := reg.Register(sampleDevice("dev-1"))
	if rec.ID != "dev-1" || rec.Name != "Living Room TV" {
		t.Fatalf("Register returned %+v", rec)
	}

	ev := recvPresenceWithin(t, ch, time.Second)
	if ev.Change != PresenceOnline || ev.Device.ID != "dev-1" {
		t.Errorf("got %+v, want ONLINE for dev-1", ev)
	}

	online := reg.Online()
	if len(online) != 1 || online[0].ID != "dev-1" {
		t.Errorf("Online() = %+v, want [dev-1]", online)
	}
	if !reg.IsOnline("dev-1") {
		t.Error("IsOnline(dev-1) = false, want true")
	}
}

// TestHeartbeatDoesNotReEmitOnline: re-registering an already-online device (a heartbeat) refreshes
// lastSeen but does NOT emit a second ONLINE event.
func TestHeartbeatDoesNotReEmitOnline(t *testing.T) {
	reg, pb, clock := newTestRegistry(DevicePresenceTTL)
	ch, unsub := pb.Subscribe()
	defer unsub()

	reg.Register(sampleDevice("dev-1"))
	_ = recvPresenceWithin(t, ch, time.Second) // the initial ONLINE

	// Advance the clock a little (still within TTL) and heartbeat again.
	*clock = clock.Add(20 * time.Second)
	reg.Register(sampleDevice("dev-1"))

	expectNoPresence(t, ch, 100*time.Millisecond)

	if !reg.IsOnline("dev-1") {
		t.Error("device should still be online after heartbeat")
	}
}

// TestTTLEvictionEmitsOffline: once lastSeen lapses past the TTL, Sweep evicts the device and emits
// OFFLINE; it no longer appears in Online().
func TestTTLEvictionEmitsOffline(t *testing.T) {
	ttl := 45 * time.Second
	reg, pb, clock := newTestRegistry(ttl)
	ch, unsub := pb.Subscribe()
	defer unsub()

	reg.Register(sampleDevice("dev-1"))
	if ev := recvPresenceWithin(t, ch, time.Second); ev.Change != PresenceOnline {
		t.Fatalf("expected ONLINE, got %+v", ev)
	}

	// Before the TTL lapses, a sweep evicts nothing.
	*clock = clock.Add(ttl - time.Second)
	reg.Sweep()
	expectNoPresence(t, ch, 100*time.Millisecond)
	if !reg.IsOnline("dev-1") {
		t.Fatal("device evicted before TTL lapsed")
	}

	// Past the TTL, the sweep evicts the device and emits OFFLINE.
	*clock = clock.Add(2 * time.Second)
	reg.Sweep()
	ev := recvPresenceWithin(t, ch, time.Second)
	if ev.Change != PresenceOffline || ev.Device.ID != "dev-1" {
		t.Errorf("got %+v, want OFFLINE for dev-1", ev)
	}

	if reg.IsOnline("dev-1") {
		t.Error("device still online after eviction")
	}
	if len(reg.Online()) != 0 {
		t.Errorf("Online() = %+v, want empty after eviction", reg.Online())
	}
}

// TestStaleDeviceExcludedFromOnline: a device whose lastSeen has lapsed is excluded from Online()
// even before the sweeper runs (online == lastSeen within TTL).
func TestStaleDeviceExcludedFromOnline(t *testing.T) {
	ttl := 45 * time.Second
	reg, _, clock := newTestRegistry(ttl)

	reg.Register(sampleDevice("dev-1"))
	*clock = clock.Add(ttl + time.Second)

	if reg.IsOnline("dev-1") {
		t.Error("stale device reported online")
	}
	if len(reg.Online()) != 0 {
		t.Errorf("Online() = %+v, want empty for stale device", reg.Online())
	}
}

// TestReRegisterAfterStaleEmitsOnline: a device that went stale and registers again emits ONLINE
// (it was effectively offline, so a new ONLINE is correct).
func TestReRegisterAfterStaleEmitsOnline(t *testing.T) {
	ttl := 45 * time.Second
	reg, pb, clock := newTestRegistry(ttl)
	ch, unsub := pb.Subscribe()
	defer unsub()

	reg.Register(sampleDevice("dev-1"))
	_ = recvPresenceWithin(t, ch, time.Second) // initial ONLINE

	*clock = clock.Add(ttl + time.Second) // go stale (not yet swept)
	reg.Register(sampleDevice("dev-1"))

	ev := recvPresenceWithin(t, ch, time.Second)
	if ev.Change != PresenceOnline {
		t.Errorf("got %+v, want ONLINE on re-register after stale", ev)
	}
}

// TestUnregisterEmitsOffline: graceful unregister removes the device and emits OFFLINE; a second
// unregister is a no-op returning false.
func TestUnregisterEmitsOffline(t *testing.T) {
	reg, pb, _ := newTestRegistry(DevicePresenceTTL)
	ch, unsub := pb.Subscribe()
	defer unsub()

	reg.Register(sampleDevice("dev-1"))
	_ = recvPresenceWithin(t, ch, time.Second) // ONLINE

	if !reg.Unregister("dev-1") {
		t.Error("Unregister(dev-1) = false, want true")
	}
	ev := recvPresenceWithin(t, ch, time.Second)
	if ev.Change != PresenceOffline || ev.Device.ID != "dev-1" {
		t.Errorf("got %+v, want OFFLINE for dev-1", ev)
	}

	if reg.Unregister("dev-1") {
		t.Error("second Unregister = true, want false (already gone)")
	}
}

// TestUpdatePlaybackEmitsPlaybackAndPersistsInRecord: a playback report emits PLAYBACK, stores the
// state on the device record, and refreshes lastSeen.
func TestUpdatePlaybackEmitsPlaybackAndPersistsInRecord(t *testing.T) {
	reg, pb, clock := newTestRegistry(DevicePresenceTTL)
	ch, unsub := pb.Subscribe()
	defer unsub()

	reg.Register(sampleDevice("dev-1"))
	_ = recvPresenceWithin(t, ch, time.Second) // ONLINE

	*clock = clock.Add(10 * time.Second)
	ok := reg.UpdatePlayback("dev-1", DevicePlayback{
		SceneID:         "scene-7",
		HasScene:        true,
		PositionSeconds: 123.5,
		Paused:          false,
	})
	if !ok {
		t.Fatal("UpdatePlayback returned false for a registered device")
	}

	ev := recvPresenceWithin(t, ch, time.Second)
	if ev.Change != PresencePlayback {
		t.Fatalf("got %+v, want PLAYBACK", ev)
	}
	if ev.Device.Playback == nil || ev.Device.Playback.SceneID != "scene-7" || ev.Device.Playback.PositionSeconds != 123.5 {
		t.Errorf("playback in event = %+v, want scene-7 @123.5", ev.Device.Playback)
	}

	online := reg.Online()
	if len(online) != 1 || online[0].Playback == nil || online[0].Playback.SceneID != "scene-7" {
		t.Errorf("Online() playback = %+v, want scene-7", online)
	}
}

// TestUpdatePlaybackUnknownDeviceIgnored: a playback report for an unregistered device is ignored
// (returns false, no event).
func TestUpdatePlaybackUnknownDeviceIgnored(t *testing.T) {
	reg, pb, _ := newTestRegistry(DevicePresenceTTL)
	ch, unsub := pb.Subscribe()
	defer unsub()

	if reg.UpdatePlayback("ghost", DevicePlayback{PositionSeconds: 1}) {
		t.Error("UpdatePlayback for unknown device = true, want false")
	}
	expectNoPresence(t, ch, 100*time.Millisecond)
}

// TestHeartbeatPreservesPlayback: a heartbeat (Register) must not wipe a device's reported playback
// state.
func TestHeartbeatPreservesPlayback(t *testing.T) {
	reg, _, clock := newTestRegistry(DevicePresenceTTL)

	reg.Register(sampleDevice("dev-1"))
	reg.UpdatePlayback("dev-1", DevicePlayback{SceneID: "scene-1", HasScene: true, PositionSeconds: 5})

	*clock = clock.Add(20 * time.Second)
	reg.Register(sampleDevice("dev-1")) // heartbeat

	online := reg.Online()
	if len(online) != 1 || online[0].Playback == nil || online[0].Playback.SceneID != "scene-1" {
		t.Errorf("playback wiped by heartbeat: %+v", online)
	}
}

// TestCommandBroadcasterTargetFiltering: PublishTo delivers a command only to the subscriber whose
// deviceId matches TargetDeviceID, and reports delivery; a command to an unsubscribed target
// returns false.
func TestCommandBroadcasterTargetFiltering(t *testing.T) {
	b := NewDeviceCommandBroadcaster()

	chA, unsubA := b.Subscribe("dev-A")
	defer unsubA()
	chB, unsubB := b.Subscribe("dev-B")
	defer unsubB()

	cmd := DeviceCommand{TargetDeviceID: "dev-A", FromDeviceID: "ctrl", Type: "PLAY"}
	if !b.PublishTo(cmd) {
		t.Error("PublishTo to a live target returned false, want true")
	}

	got := recvCommandWithin(t, chA, time.Second)
	if got.Type != "PLAY" || got.TargetDeviceID != "dev-A" {
		t.Errorf("dev-A got %+v, want PLAY/dev-A", got)
	}
	if got.FromDeviceID != "ctrl" {
		t.Errorf("dev-A got fromDeviceId %q, want %q (the broadcaster must carry the controller id through)", got.FromDeviceID, "ctrl")
	}

	// dev-B must NOT have received the command.
	select {
	case e := <-chB:
		t.Errorf("dev-B wrongly received %+v", e)
	case <-time.After(100 * time.Millisecond):
	}

	// A command to a target with no subscriber reports not-delivered.
	if b.PublishTo(DeviceCommand{TargetDeviceID: "dev-Z", Type: "STOP"}) {
		t.Error("PublishTo to an unsubscribed target = true, want false")
	}
}

// TestCommandBroadcasterFanOutToSameDevice: two subscribers for the same deviceId both receive the
// command (a device with two open connections), and delivery is reported true.
func TestCommandBroadcasterFanOutToSameDevice(t *testing.T) {
	b := NewDeviceCommandBroadcaster()
	ch1, unsub1 := b.Subscribe("dev-A")
	defer unsub1()
	ch2, unsub2 := b.Subscribe("dev-A")
	defer unsub2()

	if !b.PublishTo(DeviceCommand{TargetDeviceID: "dev-A", Type: "PAUSE"}) {
		t.Fatal("PublishTo returned false")
	}
	if got := recvCommandWithin(t, ch1, time.Second); got.Type != "PAUSE" {
		t.Errorf("ch1 got %+v", got)
	}
	if got := recvCommandWithin(t, ch2, time.Second); got.Type != "PAUSE" {
		t.Errorf("ch2 got %+v", got)
	}
}

// TestCommandBroadcasterNonBlockingFullBuffer: a target whose buffer is full never blocks the
// publisher; excess commands are dropped for that target.
func TestCommandBroadcasterNonBlockingFullBuffer(t *testing.T) {
	b := NewDeviceCommandBroadcaster()
	ch, unsub := b.Subscribe("dev-A")
	defer unsub()

	done := make(chan struct{})
	go func() {
		for i := 0; i < deviceEventBuffer*4; i++ {
			b.PublishTo(DeviceCommand{TargetDeviceID: "dev-A", Type: "SEEK"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PublishTo blocked on a full target buffer")
	}

	if got := len(ch); got != deviceEventBuffer {
		t.Errorf("buffered commands = %d, want %d (full buffer, excess dropped)", got, deviceEventBuffer)
	}
}

// TestCommandUnsubscribeCleanup: after unsubscribing, the channel closes, the subscriber is dropped,
// and a later PublishTo neither blocks nor delivers.
func TestCommandUnsubscribeCleanup(t *testing.T) {
	b := NewDeviceCommandBroadcaster()
	ch, unsub := b.Subscribe("dev-A")
	if got := b.subscriberCount(); got != 1 {
		t.Fatalf("subscriberCount = %d, want 1", got)
	}

	unsub()
	if got := b.subscriberCount(); got != 0 {
		t.Fatalf("subscriberCount after unsubscribe = %d, want 0", got)
	}
	if _, ok := <-ch; ok {
		t.Error("expected channel closed after unsubscribe")
	}
	if b.PublishTo(DeviceCommand{TargetDeviceID: "dev-A", Type: "STOP"}) {
		t.Error("PublishTo after unsubscribe delivered, want false")
	}
	unsub() // double unsubscribe must be a safe no-op
}

// TestPresenceBroadcasterNonBlockingFullBuffer: a slow presence subscriber never blocks the
// publisher; excess events are dropped for that subscriber.
func TestPresenceBroadcasterNonBlockingFullBuffer(t *testing.T) {
	b := NewDevicePresenceBroadcaster()
	ch, unsub := b.Subscribe()
	defer unsub()

	done := make(chan struct{})
	go func() {
		for i := 0; i < deviceEventBuffer*4; i++ {
			b.Publish(DevicePresenceEvent{Device: DeviceRecord{ID: "x"}, Change: PresenceOnline})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full presence subscriber buffer")
	}
	if got := len(ch); got != deviceEventBuffer {
		t.Errorf("buffered presence events = %d, want %d", got, deviceEventBuffer)
	}
}

// TestRegistryReturnsDefensiveCopies: mutating a record returned by the registry must not corrupt
// the registry's internal state (clone-on-read).
func TestRegistryReturnsDefensiveCopies(t *testing.T) {
	reg, _, _ := newTestRegistry(DevicePresenceTTL)
	reg.Register(sampleDevice("dev-1"))

	got := reg.Online()[0]
	got.Name = "HACKED"
	got.Capabilities[0] = "HACKED"

	fresh := reg.Online()[0]
	if fresh.Name == "HACKED" || fresh.Capabilities[0] == "HACKED" {
		t.Error("registry leaked mutable internal state to callers")
	}
}
