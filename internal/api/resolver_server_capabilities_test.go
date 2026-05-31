package api

import (
	"reflect"
	"testing"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/manager/config"
)

// TestServerCapabilitiesResolver asserts the serverCapabilities query surfaces the NG build
// constants verbatim. Clients gate all fork-only behaviour on this response, so it must mirror
// build.Edition / build.NGAPIVersion / build.NGFeatures() exactly, and advertise the configured
// deletedSince retention horizon.
func TestServerCapabilitiesResolver(t *testing.T) {
	// The resolver reads the retention horizon from config; an empty config exercises the default.
	config.InitializeEmpty()

	r := &Resolver{}
	caps, err := r.Query().ServerCapabilities(testCtx)
	if err != nil {
		t.Fatalf("ServerCapabilities: %v", err)
	}
	if caps == nil {
		t.Fatal("ServerCapabilities returned nil")
	}

	if caps.Edition != build.Edition {
		t.Errorf("edition = %q, want %q", caps.Edition, build.Edition)
	}
	if caps.Edition != "ng" {
		t.Errorf("edition = %q, want %q", caps.Edition, "ng")
	}
	if caps.APIVersion != build.NGAPIVersion {
		t.Errorf("apiVersion = %d, want %d", caps.APIVersion, build.NGAPIVersion)
	}
	if caps.APIVersion < 1 {
		t.Errorf("apiVersion = %d, want >= 1", caps.APIVersion)
	}
	if !reflect.DeepEqual(caps.Features, build.NGFeatures()) {
		t.Errorf("features = %v, want %v", caps.Features, build.NGFeatures())
	}
	if !slicesContain(caps.Features, "viewerPreferences") {
		t.Errorf("features missing %q; got %v", "viewerPreferences", caps.Features)
	}
	if caps.DeletedSinceRetentionDays != config.DefaultDeletedSinceRetentionDays {
		t.Errorf("deletedSinceRetentionDays = %d, want default %d", caps.DeletedSinceRetentionDays, config.DefaultDeletedSinceRetentionDays)
	}
}

func slicesContain(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
