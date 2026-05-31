package api

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/stashapp/stash/pkg/plugin"
)

const (
	maxDeviceBusIDLength          = 128
	maxDeviceBusNameLength        = 128
	maxDeviceBusRegistryDevices   = 128
	maxDeviceBusSceneIDLength     = 128
	maxDeviceBusSceneIDs          = 256
	deviceCapabilityPlayString    = string(DeviceCapabilityPlay)
	deviceCapabilityControlString = string(DeviceCapabilityControl)
)

// This file holds the deviceBus mutations and the devices query. All of it operates on the
// in-memory presence registry / command broadcaster (pkg/plugin) — NOTHING is persisted to SQLite.

// RegisterDevice upserts a device on the presence bus and refreshes its lastSeen (the heartbeat).
// It returns the device's current record (online, since it just registered). In-memory only.
func (r *mutationResolver) RegisterDevice(ctx context.Context, input DeviceInput) (*Device, error) {
	if err := validateDeviceInput(input); err != nil {
		return nil, err
	}
	if !plugin.DeviceBusRegistry.Has(input.ID) && plugin.DeviceBusRegistry.Size() >= maxDeviceBusRegistryDevices {
		return nil, fmt.Errorf("device registry is full")
	}

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
	if err := validateDeviceID("deviceId", deviceID); err != nil {
		return false, err
	}
	return plugin.DeviceBusRegistry.Unregister(deviceID), nil
}

// UpdatePlaybackState records a device's live playback signal (emits PLAYBACK). Returns false if
// the device is not registered. Distinct from the persisted server-side resume (sceneSaveActivity)
// — this signal is in-memory only and never written to SQLite.
func (r *mutationResolver) UpdatePlaybackState(ctx context.Context, input PlaybackStateInput) (bool, error) {
	if err := validatePlaybackStateInput(input); err != nil {
		return false, err
	}
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
//
// FromDeviceID is the controller's own registered device id; the server stamps it onto the relayed
// DeviceCommandEvent.fromDeviceId so the target can run per-controller TOFU (design §9). The
// controller must be online and registered with CONTROL, and the target must be online and
// registered with PLAY; otherwise the command is not relayed.
func (r *mutationResolver) SendDeviceCommand(ctx context.Context, input DeviceCommandInput) (bool, error) {
	if input.FromDeviceID == "" {
		// No controller identity — the relayed event would carry fromDeviceId == "" and every client
		// would drop it at decode. Reject rather than emit an undeliverable command.
		return false, nil
	}
	if err := validateDeviceCommandInput(input); err != nil {
		return false, err
	}
	controller, ok := plugin.DeviceBusRegistry.OnlineDevice(input.FromDeviceID)
	if !ok || !deviceRecordHasCapability(controller, deviceCapabilityControlString) {
		return false, nil
	}
	target, ok := plugin.DeviceBusRegistry.OnlineDevice(input.TargetDeviceID)
	if !ok || !deviceRecordHasCapability(target, deviceCapabilityPlayString) {
		return false, nil
	}

	cmd := plugin.DeviceCommand{
		FromDeviceID:   input.FromDeviceID,
		TargetDeviceID: input.TargetDeviceID,
		Type:           string(input.Type),
		SceneID:        input.SceneID,
		SceneIDs:       input.SceneIds,
		StartSeconds:   input.StartSeconds,
		SeekSeconds:    input.SeekSeconds,
	}
	delivered := plugin.DeviceCommands.PublishTo(cmd)
	return delivered, nil
}

func validateDeviceInput(input DeviceInput) error {
	if err := validateDeviceID("id", input.ID); err != nil {
		return err
	}
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("device name is required")
	}
	if len(input.Name) > maxDeviceBusNameLength {
		return fmt.Errorf("device name is too long")
	}
	if !input.Kind.IsValid() {
		return fmt.Errorf("invalid device kind")
	}
	if len(input.Capabilities) > len(AllDeviceCapability) {
		return fmt.Errorf("too many device capabilities")
	}
	seen := make(map[DeviceCapability]struct{}, len(input.Capabilities))
	for _, cap := range input.Capabilities {
		if !cap.IsValid() {
			return fmt.Errorf("invalid device capability")
		}
		if _, ok := seen[cap]; ok {
			return fmt.Errorf("duplicate device capability")
		}
		seen[cap] = struct{}{}
	}
	return nil
}

func validatePlaybackStateInput(input PlaybackStateInput) error {
	if err := validateDeviceID("deviceId", input.DeviceID); err != nil {
		return err
	}
	if err := validateOptionalSceneID("sceneId", input.SceneID); err != nil {
		return err
	}
	if !finiteNonNegative(input.PositionSeconds) {
		return fmt.Errorf("positionSeconds must be finite and non-negative")
	}
	return nil
}

func validateDeviceCommandInput(input DeviceCommandInput) error {
	if err := validateDeviceID("fromDeviceId", input.FromDeviceID); err != nil {
		return err
	}
	if err := validateDeviceID("targetDeviceId", input.TargetDeviceID); err != nil {
		return err
	}
	if !input.Type.IsValid() {
		return fmt.Errorf("invalid device command type")
	}
	if err := validateOptionalSceneID("sceneId", input.SceneID); err != nil {
		return err
	}
	if len(input.SceneIds) > maxDeviceBusSceneIDs {
		return fmt.Errorf("too many sceneIds")
	}
	for _, id := range input.SceneIds {
		if err := validateSceneID("sceneIds", id); err != nil {
			return err
		}
	}
	if input.StartSeconds != nil && !finiteNonNegative(*input.StartSeconds) {
		return fmt.Errorf("startSeconds must be finite and non-negative")
	}
	if input.SeekSeconds != nil && !finiteNonNegative(*input.SeekSeconds) {
		return fmt.Errorf("seekSeconds must be finite and non-negative")
	}

	switch input.Type {
	case DeviceCommandTypePlay:
		if input.SceneID == nil {
			return fmt.Errorf("sceneId is required for PLAY")
		}
		if len(input.SceneIds) != 0 || input.SeekSeconds != nil {
			return fmt.Errorf("invalid payload for PLAY")
		}
	case DeviceCommandTypeEnqueue:
		if len(input.SceneIds) == 0 {
			return fmt.Errorf("sceneIds is required for ENQUEUE")
		}
		if input.SceneID != nil || input.StartSeconds != nil || input.SeekSeconds != nil {
			return fmt.Errorf("invalid payload for ENQUEUE")
		}
	case DeviceCommandTypeSeek:
		if input.SeekSeconds == nil {
			return fmt.Errorf("seekSeconds is required for SEEK")
		}
		if input.SceneID != nil || len(input.SceneIds) != 0 || input.StartSeconds != nil {
			return fmt.Errorf("invalid payload for SEEK")
		}
	default:
		if input.SceneID != nil || len(input.SceneIds) != 0 || input.StartSeconds != nil || input.SeekSeconds != nil {
			return fmt.Errorf("invalid payload for %s", input.Type)
		}
	}
	return nil
}

func validateDeviceID(field, id string) error {
	if id == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(id) > maxDeviceBusIDLength {
		return fmt.Errorf("%s is too long", field)
	}
	return nil
}

func validateOptionalSceneID(field string, id *string) error {
	if id == nil {
		return nil
	}
	return validateSceneID(field, *id)
}

func validateSceneID(field, id string) error {
	if id == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	if len(id) > maxDeviceBusSceneIDLength {
		return fmt.Errorf("%s is too long", field)
	}
	return nil
}

func finiteNonNegative(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func deviceRecordHasCapability(rec plugin.DeviceRecord, want string) bool {
	for _, cap := range rec.Capabilities {
		if cap == want {
			return true
		}
	}
	return false
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
