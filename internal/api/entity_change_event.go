package api

import "time"

// EntityChangeEvent is the GraphQL payload for the entityChanged subscription: a metadata-only
// notification (kind + id + operation + time) that a tracked entity was created, updated, or
// destroyed. It mirrors the outbound webhook payload and carries no titles/paths (privacy + small
// payload); clients refetch the entity by id (or prune it on destroy). Bound via gqlgen.yml so the
// subscription resolver can reference it before the exec is generated.
type EntityChangeEvent struct {
	// Entity is the entity kind, e.g. "Scene", "Tag", "Performer".
	Entity string `json:"entity"`
	// ID is the affected entity's id.
	ID string `json:"id"`
	// Operation is "Create", "Update", or "Destroy".
	Operation string `json:"operation"`
	// Time is when the change occurred.
	Time time.Time `json:"time"`
}
