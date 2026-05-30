package api

import (
	"context"

	"github.com/stashapp/stash/pkg/plugin"
)

// toDevice converts a registry record into the GraphQL Device payload. online is passed explicitly
// so it stays consistent with the change kind (ONLINE/PLAYBACK => true, OFFLINE => false) rather
// than being re-derived against a TTL the resolver doesn't own.
func toDevice(rec plugin.DeviceRecord, online bool) *Device {
	d := &Device{
		ID:           rec.ID,
		Name:         rec.Name,
		Kind:         DeviceKind(rec.Kind),
		Capabilities: make([]DeviceCapability, 0, len(rec.Capabilities)),
		Online:       online,
		LastSeen:     rec.LastSeen,
	}
	for _, c := range rec.Capabilities {
		d.Capabilities = append(d.Capabilities, DeviceCapability(c))
	}
	if rec.Playback != nil {
		pb := &PlaybackState{
			PositionSeconds: rec.Playback.PositionSeconds,
			Paused:          rec.Playback.Paused,
			UpdatedAt:       rec.Playback.UpdatedAt,
		}
		if rec.Playback.HasScene {
			sid := rec.Playback.SceneID
			pb.SceneID = &sid
		}
		d.Playback = pb
	}
	return d
}

// DevicePresence streams presence-bus changes (a device coming ONLINE, going OFFLINE, or updating
// PLAYBACK) to the client. It mirrors EntityChanged: register on the process-wide presence
// broadcaster, fan events through, and tear down on context cancellation so no channel is leaked.
func (r *subscriptionResolver) DevicePresence(ctx context.Context) (<-chan *DevicePresenceEvent, error) {
	source, unsubscribe := plugin.DevicePresence.Subscribe()
	out := make(chan *DevicePresenceEvent, 1)

	go func() {
		defer unsubscribe()
		defer close(out)

		for {
			select {
			case event, ok := <-source:
				if !ok {
					return
				}
				// OFFLINE means the device is gone/stale; ONLINE and PLAYBACK both imply online.
				online := event.Change != plugin.PresenceOffline
				ev := &DevicePresenceEvent{
					Device: toDevice(event.Device, online),
					Kind:   PresenceChange(event.Change),
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// DeviceCommands streams only the commands addressed to deviceId (sendDeviceCommand whose
// targetDeviceId == deviceId) to the target. It registers on the process-wide command broadcaster,
// discards commands not addressed to this device, and tears down on context cancellation.
func (r *subscriptionResolver) DeviceCommands(ctx context.Context, deviceID string) (<-chan *DeviceCommandEvent, error) {
	source, unsubscribe := plugin.DeviceCommands.Subscribe(deviceID)
	out := make(chan *DeviceCommandEvent, 1)

	go func() {
		defer unsubscribe()
		defer close(out)

		for {
			select {
			case cmd, ok := <-source:
				if !ok {
					return
				}
				// Per-target fan-out: only deliver commands aimed at this device.
				if cmd.TargetDeviceID != deviceID {
					continue
				}
				ev := &DeviceCommandEvent{
					FromDeviceID: cmd.FromDeviceID,
					Type:         DeviceCommandType(cmd.Type),
					SceneID:      cmd.SceneID,
					SceneIds:     cmd.SceneIDs,
					StartSeconds: cmd.StartSeconds,
					SeekSeconds:  cmd.SeekSeconds,
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}
