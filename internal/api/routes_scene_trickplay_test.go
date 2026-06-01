package api

import (
	"strings"
	"testing"
)

func TestBuildHLSMasterPlaylistHasBothVariants(t *testing.T) {
	pl := string(buildHLSMasterPlaylist(1920, 1080, 8_000_000, 160, false,
		"stream.m3u8?type=media&resolution=720", "trickplay.m3u8"))

	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-VERSION:7",
		"#EXT-X-STREAM-INF:BANDWIDTH=8000000,RESOLUTION=1920x1080",
		"stream.m3u8?type=media&resolution=720",
		"#EXT-X-I-FRAME-STREAM-INF:",
		`CODECS="avc1.42E01E"`,
		`URI="trickplay.m3u8"`,
	} {
		if !strings.Contains(pl, want) {
			t.Errorf("master playlist missing %q\n--- got ---\n%s", want, pl)
		}
	}
}

func TestBuildHLSMasterPlaylistToleratesMissingDimensions(t *testing.T) {
	// Unknown width/height/bitrate must not emit a broken RESOLUTION and must still carry both variants.
	pl := string(buildHLSMasterPlaylist(0, 0, 0, 160, false, "stream.m3u8?type=media", "trickplay.m3u8"))
	if strings.Contains(pl, "RESOLUTION=") {
		t.Errorf("did not expect RESOLUTION with unknown dimensions:\n%s", pl)
	}
	if !strings.Contains(pl, "#EXT-X-STREAM-INF:BANDWIDTH=") || !strings.Contains(pl, "#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=") {
		t.Errorf("expected both variants even without dimensions:\n%s", pl)
	}
}

func TestTrickplayDimensionsPreserveAspect(t *testing.T) {
	// Landscape 1920x1080, longest side 160 -> 160x90.
	if w, h := trickplayDimensions(1920, 1080, 160, false); w != 160 || h != 90 {
		t.Errorf("landscape: expected 160x90, got %dx%d", w, h)
	}
	// Portrait 1080x1920, longest side 160 -> 90x160.
	if w, h := trickplayDimensions(1080, 1920, 160, true); w != 90 || h != 160 {
		t.Errorf("portrait: expected 90x160, got %dx%d", w, h)
	}
	// Unknown dimensions -> (0,0) so RESOLUTION is omitted.
	if w, h := trickplayDimensions(0, 0, 160, false); w != 0 || h != 0 {
		t.Errorf("unknown: expected 0x0, got %dx%d", w, h)
	}
}

func TestIframeBandwidthHasFloor(t *testing.T) {
	if got := iframeBandwidth(8_000_000); got != 400_000 {
		t.Errorf("expected 400000, got %d", got)
	}
	if got := iframeBandwidth(100); got != 50_000 {
		t.Errorf("expected floor 50000, got %d", got)
	}
}
