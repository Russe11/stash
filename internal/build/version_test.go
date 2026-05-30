package build

import (
	"reflect"
	"testing"
)

// TestEdition pins the NG edition stamp. Clients gate all fork-only behaviour on this exact value
// (a missing serverCapabilities query / a non-"ng" edition is treated as upstream and NG features
// are disabled), so changing it is a breaking contract change and must be deliberate.
func TestEdition(t *testing.T) {
	if Edition != "ng" {
		t.Errorf("Edition = %q, want %q", Edition, "ng")
	}
}

// TestNGAPIVersion guards the NG client-facing API revision. It must be a positive integer; bump
// it (don't reset it) whenever the NG surface changes so the NG-to-NG version-skew handshake works.
func TestNGAPIVersion(t *testing.T) {
	if NGAPIVersion < 1 {
		t.Errorf("NGAPIVersion = %d, want >= 1", NGAPIVersion)
	}
}

// TestNGFeatures pins the exact advertised fork-only feature set. Clients key behaviour off these
// strings; adding/removing/renaming one is a contract change shared with every client and the
// NG-to-NG contract test, so this asserts the full set (order included) deliberately.
func TestNGFeatures(t *testing.T) {
	want := []string{"deletedSince", "moveFolder", "folderCounts", "webhooks", "similarScenes", "entityChanged"}
	got := NGFeatures()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NGFeatures() = %v, want %v", got, want)
	}
}
