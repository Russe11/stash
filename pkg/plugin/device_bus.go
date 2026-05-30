package plugin

import (
	"sync"
	"time"
)

// This file implements the deviceBus: an ephemeral, in-memory presence registry plus two
// broadcasters (presence + commands) that mirror EntityEventBroadcaster. NOTHING here is persisted
// to SQLite — presence and commands live only in process memory, scoped to the authenticated
// server session, and the payloads carry ids (never titles). The GraphQL-facing models live in
// internal/api/device_bus.go; the subscription/mutation/query resolvers translate between the two.

// Presence TTL / sweep constants. A device that hasn't heartbeated within DevicePresenceTTL is
// considered offline; the sweeper evicts it (and emits OFFLINE) on the next DevicePresenceSweep
// tick. Clients heartbeat via registerDevice ~every 20s, comfortably inside the 45s TTL.
const (
	DevicePresenceTTL   = 45 * time.Second
	DevicePresenceSweep = 15 * time.Second
)

// deviceEventBuffer is the per-subscriber channel depth for both bus broadcasters. A slow
// subscriber that fills its buffer has further events dropped (non-blocking fan-out) rather than
// blocking the mutation that produced them — same policy as EntityEventBroadcaster.
const deviceEventBuffer = 256

// DevicePlayback is the registry's view of a device's live playback signal (ids only, no titles).
type DevicePlayback struct {
	SceneID         string // empty when nothing is playing
	HasScene        bool   // distinguishes "no scene" from sceneId == ""
	PositionSeconds float64
	Paused          bool
	UpdatedAt       time.Time
}

// DeviceRecord is the registry's source-of-truth entry for one device. It is a value copied out on
// read so callers can never mutate the registry's internal state.
type DeviceRecord struct {
	ID           string
	Name         string
	Kind         string   // "TV" | "DESKTOP" | "MOBILE" | "OTHER"
	Capabilities []string // "PLAY" | "CONTROL"
	LastSeen     time.Time
	Playback     *DevicePlayback
}

// clone returns a deep copy so the registry's stored pointer/slice state can't leak to callers.
func (d DeviceRecord) clone() DeviceRecord {
	out := d
	if d.Capabilities != nil {
		out.Capabilities = append([]string(nil), d.Capabilities...)
	}
	if d.Playback != nil {
		pb := *d.Playback
		out.Playback = &pb
	}
	return out
}

// DevicePresenceChange enumerates what a DevicePresenceEvent describes.
type DevicePresenceChange string

const (
	PresenceOnline   DevicePresenceChange = "ONLINE"
	PresenceOffline  DevicePresenceChange = "OFFLINE"
	PresencePlayback DevicePresenceChange = "PLAYBACK"
)

// DevicePresenceEvent is what the presence broadcaster fans out: the affected device's current
// record plus the kind of change.
type DevicePresenceEvent struct {
	Device DeviceRecord
	Change DevicePresenceChange
}

// DeviceCommand is what the command broadcaster fans out. TargetDeviceID is used only for routing
// (the resolver delivers a command to the subscriber whose deviceId == TargetDeviceID); it is not
// re-sent to the client. Ids only — no titles.
type DeviceCommand struct {
	TargetDeviceID string
	FromDeviceID   string
	Type           string
	SceneID        *string
	SceneIDs       []string
	StartSeconds   *float64
	SeekSeconds    *float64
}

// ---------------------------------------------------------------------------
// Presence broadcaster — mirrors EntityEventBroadcaster.
// ---------------------------------------------------------------------------

// DevicePresenceBroadcaster is the in-process pub/sub fan-out for presence/playback changes.
// Fan-out is non-blocking: a full subscriber buffer drops the event for that subscriber only.
type DevicePresenceBroadcaster struct {
	mu          sync.Mutex
	subscribers map[int]chan DevicePresenceEvent
	nextID      int
}

func NewDevicePresenceBroadcaster() *DevicePresenceBroadcaster {
	return &DevicePresenceBroadcaster{subscribers: make(map[int]chan DevicePresenceEvent)}
}

// Subscribe registers a subscriber and returns its receive channel plus an unsubscribe func that
// the caller MUST invoke on context cancellation / disconnect to avoid leaking the channel.
func (b *DevicePresenceBroadcaster) Subscribe() (<-chan DevicePresenceEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan DevicePresenceEvent, deviceEventBuffer)
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

// Publish fans an event out to every current subscriber (non-blocking — drops for a full buffer).
func (b *DevicePresenceBroadcaster) Publish(event DevicePresenceEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber buffer is full; drop this event for it rather than block.
		}
	}
}

// subscriberCount reports the number of live subscribers (used in tests).
func (b *DevicePresenceBroadcaster) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}

// SubscriberCountForTest exposes the live subscriber count to tests in other packages (the resolver
// leak check). Not part of the production API.
func (b *DevicePresenceBroadcaster) SubscriberCountForTest() int { return b.subscriberCount() }

// ---------------------------------------------------------------------------
// Command broadcaster — mirrors EntityEventBroadcaster, with per-target fan-out.
// ---------------------------------------------------------------------------

// commandSubscriber pairs a delivery channel with the deviceId it listens for, so the broadcaster
// can route a command only to the subscriber(s) whose deviceId == cmd.TargetDeviceID.
type commandSubscriber struct {
	deviceID string
	ch       chan DeviceCommand
}

// DeviceCommandBroadcaster fans commands out per target: a subscriber registers with its own
// deviceId and PublishTo delivers a command only to subscribers whose deviceId matches the
// command's TargetDeviceID. Non-blocking, buffered, drop-on-slow (a full target buffer drops the
// command for that target only). The resolver still double-checks the target id defensively.
type DeviceCommandBroadcaster struct {
	mu          sync.Mutex
	subscribers map[int]commandSubscriber
	nextID      int
}

func NewDeviceCommandBroadcaster() *DeviceCommandBroadcaster {
	return &DeviceCommandBroadcaster{subscribers: make(map[int]commandSubscriber)}
}

// Subscribe registers a subscriber for the given deviceId and returns its receive channel plus an
// unsubscribe func the caller MUST invoke on disconnect to avoid leaking the channel.
func (b *DeviceCommandBroadcaster) Subscribe(deviceID string) (<-chan DeviceCommand, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan DeviceCommand, deviceEventBuffer)
	b.subscribers[id] = commandSubscriber{deviceID: deviceID, ch: ch}

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if existing, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(existing.ch)
		}
	}

	return ch, unsubscribe
}

// PublishTo delivers a command only to the subscriber(s) whose deviceId == cmd.TargetDeviceID and
// reports whether at least one such live subscriber received it (delivery is non-blocking; a target
// whose buffer is full counts as NOT delivered). Returns false when the target has no subscriber —
// which is how sendDeviceCommand tells a controller the target isn't listening.
func (b *DeviceCommandBroadcaster) PublishTo(cmd DeviceCommand) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	delivered := false
	for _, sub := range b.subscribers {
		if sub.deviceID != cmd.TargetDeviceID {
			continue
		}
		select {
		case sub.ch <- cmd:
			delivered = true
		default:
			// Target buffer is full; drop this command for it rather than block.
		}
	}
	return delivered
}

func (b *DeviceCommandBroadcaster) subscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers)
}

// SubscriberCountForTest exposes the live subscriber count to tests in other packages (the resolver
// leak check). Not part of the production API.
func (b *DeviceCommandBroadcaster) SubscriberCountForTest() int { return b.subscriberCount() }

// ---------------------------------------------------------------------------
// Device registry — ephemeral, TTL-evicted, in-memory.
// ---------------------------------------------------------------------------

// DeviceRegistry holds the in-memory presence map (keyed by deviceId) and owns the presence
// broadcaster. It NEVER touches SQLite. A background sweeper evicts devices whose lastSeen is older
// than the TTL and emits OFFLINE for each. The registry uses an injectable clock so tests can drive
// TTL eviction deterministically.
type DeviceRegistry struct {
	mu      sync.Mutex
	devices map[string]*DeviceRecord
	ttl     time.Duration
	now     func() time.Time

	presence *DevicePresenceBroadcaster

	stop     chan struct{}
	stopOnce sync.Once
}

// NewDeviceRegistry returns a registry with the given TTL and presence broadcaster. now defaults to
// time.Now if nil (tests pass a controllable clock).
func NewDeviceRegistry(ttl time.Duration, presence *DevicePresenceBroadcaster, now func() time.Time) *DeviceRegistry {
	if now == nil {
		now = time.Now
	}
	return &DeviceRegistry{
		devices:  make(map[string]*DeviceRecord),
		ttl:      ttl,
		now:      now,
		presence: presence,
		stop:     make(chan struct{}),
	}
}

// Register upserts a device and refreshes its lastSeen (the heartbeat). It returns the device's
// current record and emits an ONLINE presence event only when the device was newly added or had
// previously gone stale (so heartbeats from an already-online device don't spam ONLINE events).
func (r *DeviceRegistry) Register(rec DeviceRecord) DeviceRecord {
	r.mu.Lock()
	now := r.now()

	existing, ok := r.devices[rec.ID]
	wasOnline := ok && !r.isStaleLocked(existing, now)

	if ok {
		// Upsert mutable fields; preserve the prior playback state across heartbeats.
		existing.Name = rec.Name
		existing.Kind = rec.Kind
		existing.Capabilities = append([]string(nil), rec.Capabilities...)
		existing.LastSeen = now
	} else {
		stored := rec.clone()
		stored.LastSeen = now
		stored.Playback = nil
		existing = &stored
		r.devices[rec.ID] = existing
	}

	out := existing.clone()
	r.mu.Unlock()

	if !wasOnline {
		r.emit(out, PresenceOnline)
	}
	return out
}

// Unregister removes a device (graceful exit) and emits OFFLINE. Returns true if it was present.
func (r *DeviceRegistry) Unregister(deviceID string) bool {
	r.mu.Lock()
	rec, ok := r.devices[deviceID]
	if !ok {
		r.mu.Unlock()
		return false
	}
	out := rec.clone()
	delete(r.devices, deviceID)
	r.mu.Unlock()

	r.emit(out, PresenceOffline)
	return true
}

// UpdatePlayback records a device's live playback state and emits PLAYBACK. Returns false if the
// device is not registered (a report with no prior registerDevice is ignored). Refreshes lastSeen
// so an actively-playing device stays online even between heartbeats.
func (r *DeviceRegistry) UpdatePlayback(deviceID string, pb DevicePlayback) bool {
	r.mu.Lock()
	rec, ok := r.devices[deviceID]
	if !ok {
		r.mu.Unlock()
		return false
	}
	now := r.now()
	pb.UpdatedAt = now
	stored := pb
	rec.Playback = &stored
	rec.LastSeen = now
	out := rec.clone()
	r.mu.Unlock()

	r.emit(out, PresencePlayback)
	return true
}

// Online returns the devices currently online (lastSeen within the TTL), as cloned records.
func (r *DeviceRegistry) Online() []DeviceRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	out := make([]DeviceRecord, 0, len(r.devices))
	for _, rec := range r.devices {
		if !r.isStaleLocked(rec, now) {
			out = append(out, rec.clone())
		}
	}
	return out
}

// IsOnline reports whether a device exists and is within the TTL (used by the resolver to set the
// Device.online flag consistently with Online()).
func (r *DeviceRegistry) IsOnline(deviceID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.devices[deviceID]
	if !ok {
		return false
	}
	return !r.isStaleLocked(rec, r.now())
}

// isStaleLocked reports whether a record's lastSeen is older than the TTL. Caller holds r.mu.
func (r *DeviceRegistry) isStaleLocked(rec *DeviceRecord, now time.Time) bool {
	return now.Sub(rec.LastSeen) > r.ttl
}

// Sweep evicts every device whose lastSeen is older than the TTL and emits OFFLINE for each. It is
// safe to call directly (tests do) or on a ticker (StartSweeper).
func (r *DeviceRegistry) Sweep() {
	r.mu.Lock()
	now := r.now()
	var evicted []DeviceRecord
	for id, rec := range r.devices {
		if r.isStaleLocked(rec, now) {
			evicted = append(evicted, rec.clone())
			delete(r.devices, id)
		}
	}
	r.mu.Unlock()

	for _, rec := range evicted {
		r.emit(rec, PresenceOffline)
	}
}

// StartSweeper runs Sweep on a ticker until StopSweeper is called. Call once at server startup.
func (r *DeviceRegistry) StartSweeper(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.Sweep()
			case <-r.stop:
				return
			}
		}
	}()
}

// StopSweeper stops the background sweeper (idempotent). Used in tests and on shutdown.
func (r *DeviceRegistry) StopSweeper() {
	r.stopOnce.Do(func() { close(r.stop) })
}

// emit publishes a presence event to the broadcaster (if one is attached). The device record's
// online flag is implicit in the change kind (ONLINE/PLAYBACK => online; OFFLINE => offline); the
// resolver maps that onto the GraphQL Device.online field.
func (r *DeviceRegistry) emit(rec DeviceRecord, change DevicePresenceChange) {
	if r.presence == nil {
		return
	}
	r.presence.Publish(DevicePresenceEvent{Device: rec, Change: change})
}

// ---------------------------------------------------------------------------
// Process-wide singletons — mirror the EntityEvents singleton.
// ---------------------------------------------------------------------------

// DevicePresence is the process-wide presence broadcaster (subscribed by the devicePresence
// resolver, published by the registry). DeviceCommands is the process-wide command broadcaster
// (subscribed by deviceCommands, published by sendDeviceCommand). DeviceBusRegistry is the
// process-wide presence registry. All are in-memory only.
var (
	DevicePresence    = NewDevicePresenceBroadcaster()
	DeviceCommands    = NewDeviceCommandBroadcaster()
	DeviceBusRegistry = NewDeviceRegistry(DevicePresenceTTL, DevicePresence, nil)
)
