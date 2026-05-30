package api

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/pkg/plugin"
)

// EntityChanged streams metadata-only entity change events (create/update/destroy) to the client.
// It registers a subscriber on the process-wide broadcaster that the post-commit choke point
// publishes to, optionally filters by entity kind (`types`), and tears the subscription down when
// the client disconnects (context cancellation), so no subscriber channel is leaked.
func (r *subscriptionResolver) EntityChanged(ctx context.Context, types []string) (<-chan *EntityChangeEvent, error) {
	// Build a set of wanted entity kinds. Nil/empty means "all kinds".
	var wanted map[string]bool
	if len(types) > 0 {
		wanted = make(map[string]bool, len(types))
		for _, t := range types {
			wanted[t] = true
		}
	}

	source, unsubscribe := plugin.EntityEvents.Subscribe()
	out := make(chan *EntityChangeEvent, 1)

	go func() {
		defer unsubscribe()
		defer close(out)

		for {
			select {
			case event, ok := <-source:
				if !ok {
					return
				}
				if wanted != nil && !wanted[event.Entity] {
					continue
				}
				ec := &EntityChangeEvent{
					Entity:    event.Entity,
					ID:        strconv.Itoa(event.ID),
					Operation: event.Operation,
					Time:      event.Time,
				}
				select {
				case out <- ec:
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
