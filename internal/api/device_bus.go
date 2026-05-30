package api

import "time"

// This file holds the hand-written GraphQL models for the deviceBus surface (presence + commands).
// They are bound via gqlgen.yml so the resolvers can reference them before the exec is generated and
// so the autobind list (which includes pkg/plugin) cannot grab a similarly-named registry struct.
// Every id is a string (the GraphQL ID scalar) — device ids are client-generated UUIDs, never the
// SQLite int ids used elsewhere. NOTHING in this surface is persisted: presence and commands live in
// the in-memory broadcasters/registry only (see pkg/plugin/device_bus.go), carry ids not titles, and
// are scoped to the authenticated server session.

// DeviceKind enumerates the kind of device on the bus.
type DeviceKind string

const (
	DeviceKindTv      DeviceKind = "TV"
	DeviceKindDesktop DeviceKind = "DESKTOP"
	DeviceKindMobile  DeviceKind = "MOBILE"
	DeviceKindOther   DeviceKind = "OTHER"
)

// AllDeviceKind is the set of valid DeviceKind values (used by gqlgen enum (un)marshalling).
var AllDeviceKind = []DeviceKind{DeviceKindTv, DeviceKindDesktop, DeviceKindMobile, DeviceKindOther}

func (e DeviceKind) IsValid() bool {
	switch e {
	case DeviceKindTv, DeviceKindDesktop, DeviceKindMobile, DeviceKindOther:
		return true
	}
	return false
}

func (e DeviceKind) String() string { return string(e) }

// DeviceCapability enumerates what a device will do: PLAY (be a target) / CONTROL (drive others).
type DeviceCapability string

const (
	DeviceCapabilityPlay    DeviceCapability = "PLAY"
	DeviceCapabilityControl DeviceCapability = "CONTROL"
)

var AllDeviceCapability = []DeviceCapability{DeviceCapabilityPlay, DeviceCapabilityControl}

func (e DeviceCapability) IsValid() bool {
	switch e {
	case DeviceCapabilityPlay, DeviceCapabilityControl:
		return true
	}
	return false
}

func (e DeviceCapability) String() string { return string(e) }

// PresenceChange enumerates the kind of a DevicePresenceEvent.
type PresenceChange string

const (
	PresenceChangeOnline   PresenceChange = "ONLINE"
	PresenceChangeOffline  PresenceChange = "OFFLINE"
	PresenceChangePlayback PresenceChange = "PLAYBACK"
)

var AllPresenceChange = []PresenceChange{PresenceChangeOnline, PresenceChangeOffline, PresenceChangePlayback}

func (e PresenceChange) IsValid() bool {
	switch e {
	case PresenceChangeOnline, PresenceChangeOffline, PresenceChangePlayback:
		return true
	}
	return false
}

func (e PresenceChange) String() string { return string(e) }

// DeviceCommandType enumerates the cross-device command verbs.
type DeviceCommandType string

const (
	DeviceCommandTypePlay    DeviceCommandType = "PLAY"
	DeviceCommandTypePause   DeviceCommandType = "PAUSE"
	DeviceCommandTypeResume  DeviceCommandType = "RESUME"
	DeviceCommandTypeSeek    DeviceCommandType = "SEEK"
	DeviceCommandTypeStop    DeviceCommandType = "STOP"
	DeviceCommandTypeEnqueue DeviceCommandType = "ENQUEUE"
	DeviceCommandTypeNext    DeviceCommandType = "NEXT"
	DeviceCommandTypePrev    DeviceCommandType = "PREV"
)

var AllDeviceCommandType = []DeviceCommandType{
	DeviceCommandTypePlay, DeviceCommandTypePause, DeviceCommandTypeResume, DeviceCommandTypeSeek,
	DeviceCommandTypeStop, DeviceCommandTypeEnqueue, DeviceCommandTypeNext, DeviceCommandTypePrev,
}

func (e DeviceCommandType) IsValid() bool {
	switch e {
	case DeviceCommandTypePlay, DeviceCommandTypePause, DeviceCommandTypeResume, DeviceCommandTypeSeek,
		DeviceCommandTypeStop, DeviceCommandTypeEnqueue, DeviceCommandTypeNext, DeviceCommandTypePrev:
		return true
	}
	return false
}

func (e DeviceCommandType) String() string { return string(e) }

// PlaybackState is the GraphQL payload for a device's live playback signal.
type PlaybackState struct {
	// SceneID is the scene currently playing (id only, no title), if any.
	SceneID *string `json:"sceneId"`
	// PositionSeconds is the current playback offset.
	PositionSeconds float64 `json:"positionSeconds"`
	// Paused is true when playback is paused.
	Paused bool `json:"paused"`
	// UpdatedAt is when the device last reported this state.
	UpdatedAt time.Time `json:"updatedAt"`
}

// Device is the GraphQL payload for a device registered on the presence bus.
type Device struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Kind         DeviceKind         `json:"kind"`
	Capabilities []DeviceCapability `json:"capabilities"`
	// Online is true when LastSeen is within the presence TTL.
	Online bool `json:"online"`
	// LastSeen is when the device last registered / heartbeated.
	LastSeen time.Time `json:"lastSeen"`
	// Playback is the device's most recent reported playback state, if any.
	Playback *PlaybackState `json:"playback"`
}

// DevicePresenceEvent is the GraphQL payload for the devicePresence subscription.
type DevicePresenceEvent struct {
	Device *Device        `json:"device"`
	Kind   PresenceChange `json:"kind"`
}

// DeviceCommandEvent is the GraphQL payload for the deviceCommands subscription.
type DeviceCommandEvent struct {
	// FromDeviceID is the controller that sent the command.
	FromDeviceID string            `json:"fromDeviceId"`
	Type         DeviceCommandType `json:"type"`
	SceneID      *string           `json:"sceneId"`
	SceneIds     []string          `json:"sceneIds"`
	StartSeconds *float64          `json:"startSeconds"`
	SeekSeconds  *float64          `json:"seekSeconds"`
}

// DeviceInput is the registration payload for registerDevice.
type DeviceInput struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Kind         DeviceKind         `json:"kind"`
	Capabilities []DeviceCapability `json:"capabilities"`
}

// PlaybackStateInput is the playback report for updatePlaybackState.
type PlaybackStateInput struct {
	DeviceID        string  `json:"deviceId"`
	SceneID         *string `json:"sceneId"`
	PositionSeconds float64 `json:"positionSeconds"`
	Paused          bool    `json:"paused"`
}

// DeviceCommandInput is the command payload for sendDeviceCommand.
type DeviceCommandInput struct {
	TargetDeviceID string            `json:"targetDeviceId"`
	Type           DeviceCommandType `json:"type"`
	SceneID        *string           `json:"sceneId"`
	SceneIds       []string          `json:"sceneIds"`
	StartSeconds   *float64          `json:"startSeconds"`
	SeekSeconds    *float64          `json:"seekSeconds"`
}
