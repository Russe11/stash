package api

import (
	"context"

	"github.com/stashapp/stash/pkg/plugin"
)

// This file holds the deviceBus mutations and the devices query. All of it operates on the
// in-memory presence registry / command broadcaster (pkg/plugin) — NOTHING is persisted to SQLite.

// RegisterDevice upserts a device on the presence bus and refreshes its lastSeen (the heartbeat).
// It returns the device's current record (online, since it just registered). In-memory only.
func (r *mutationResolver) RegisterDevice(ctx context.Context, input DeviceInput) (*Device, error) {
	caps := make([]string, 0, len(input.Capabilities))
	for _, c := range input.Capabilities {
		caps = append(caps, string(c))
	}
	rec := plugin.DeviceBusRegistry.Register(plugin.DeviceRecord{
		ID:           input.ID,
		Name:         input.Name,
		Kind:         string(input.Kind),
		Capabilities: caps,
	})
	// A device that just registered is by definition online.
	return toDevice(rec, true), nil
}

// UnregisterDevice removes a device on graceful exit (emits OFFLINE). Returns true if it was
// present. In-memory only.
func (r *mutationResolver) UnregisterDevice(ctx context.Context, deviceID string) (bool, error) {
	return plugin.DeviceBusRegistry.Unregister(deviceID), nil
}

// UpdatePlaybackState records a device's live playback signal (emits PLAYBACK). Returns false if
// the device is not registered. Distinct from the persisted server-side resume (sceneSaveActivity)
// — this signal is in-memory only and never written to SQLite.
func (r *mutationResolver) UpdatePlaybackState(ctx context.Context, input PlaybackStateInput) (bool, error) {
	pb := plugin.DevicePlayback{
		PositionSeconds: input.PositionSeconds,
		Paused:          input.Paused,
	}
	if input.SceneID != nil {
		pb.HasScene = true
		pb.SceneID = *input.SceneID
	}
	return plugin.DeviceBusRegistry.UpdatePlayback(input.DeviceID, pb), nil
}

// SendDeviceCommand relays a command from a controller to the target device's command stream.
// Returns true if a live target subscriber received it (no subscriber == false). Ephemeral — the
// command is fanned out, never persisted.
func (r *mutationResolver) SendDeviceCommand(ctx context.Context, input DeviceCommandInput) (bool, error) {
	cmd := plugin.DeviceCommand{
		TargetDeviceID: input.TargetDeviceID,
		Type:           string(input.Type),
		SceneID:        input.SceneID,
		SceneIDs:       input.SceneIds,
		StartSeconds:   input.StartSeconds,
		SeekSeconds:    input.SeekSeconds,
	}
	// FromDeviceID is not in the input contract (the controller is implicit on a single-user
	// server); leave it empty for now. Clients identify the controller out-of-band on first command
	// (TOFU, design §9). Kept as a struct field so a future input arg can populate it without a
	// broadcaster change.
	delivered := plugin.DeviceCommands.PublishTo(cmd)
	return delivered, nil
}

// Devices returns the currently-online devices on the presence bus (lastSeen within the TTL). In
// In-memory only — backs the "send to device" picker.
func (r *queryResolver) Devices(ctx context.Context) ([]*Device, error) {
	records := plugin.DeviceBusRegistry.Online()
	out := make([]*Device, 0, len(records))
	for _, rec := range records {
		out = append(out, toDevice(rec, true))
	}
	return out, nil
}
