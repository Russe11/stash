package generate

import (
	"strings"
	"testing"
)

// ffmpegBuggyPlaylist is real output from `ffmpeg -hls_flags single_file+iframes_only` — note every
// byte-range offset is `@0` (the bug we work around) while the lengths are correct and the frames are
// concatenated in order.
const ffmpegBuggyPlaylist = `#EXTM3U
#EXT-X-VERSION:4
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-I-FRAMES-ONLY
#EXTINF:1.000000,
#EXT-X-BYTERANGE:3384@0
trickplay.ts
#EXTINF:1.000000,
#EXT-X-BYTERANGE:3196@0
trickplay.ts
#EXTINF:1.000000,
#EXT-X-BYTERANGE:3200@0
trickplay.ts
#EXT-X-ENDLIST
`

func TestRewriteTrickplayManifestFixesCumulativeOffsets(t *testing.T) {
	out, err := rewriteTrickplayManifest([]byte(ffmpegBuggyPlaylist), TrickplayMediaURI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)

	// Offsets must be the cumulative sum of preceding lengths (0, 3384, 3384+3196).
	for _, want := range []string{
		"#EXT-X-BYTERANGE:3384@0",
		"#EXT-X-BYTERANGE:3196@3384",
		"#EXT-X-BYTERANGE:3200@6580",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected playlist to contain %q\n--- got ---\n%s", want, got)
		}
	}

	// The bug (every segment @0) must be gone: only the first segment may be @0.
	if strings.Count(got, "@0\n") != 1 {
		t.Errorf("expected exactly one @0 offset (the first segment), got %d\n%s", strings.Count(got, "@0\n"), got)
	}

	// Must remain a valid I-frame playlist pointing at the media URI.
	for _, want := range []string{"#EXT-X-I-FRAMES-ONLY", "#EXT-X-ENDLIST", "\n" + TrickplayMediaURI + "\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected playlist to contain %q", want)
		}
	}
}

func TestRewriteTrickplayManifestEmptyIsError(t *testing.T) {
	_, err := rewriteTrickplayManifest([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"), TrickplayMediaURI)
	if err == nil {
		t.Fatal("expected error for playlist with no byte-range segments")
	}
}

func TestTrickplayArgsAreIFrameSingleFile(t *testing.T) {
	args := []string(trickplayArgs("in.mp4", TrickplayOptions{Interval: 10, Size: 160, IsPortrait: false}, "out.ts", "out.m3u8"))
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"single_file+iframes_only", // the I-frame muxer flags
		"-g 1",                     // every frame a keyframe
		"-profile:v baseline",      // pinned profile so CODECS is deterministic
		"-level 3.0",
		"scale=160:-2",             // landscape scale to longest dim
		"fps=0.100000",             // 1 frame / 10s interval
		"-hls_time 10",
		"out.ts",
		"out.m3u8",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected ffmpeg args to contain %q\nargs: %s", want, joined)
		}
	}
}

func TestTrickplayArgsPortraitScale(t *testing.T) {
	args := []string(trickplayArgs("in.mp4", TrickplayOptions{Interval: 5, Size: 160, IsPortrait: true}, "out.ts", "out.m3u8"))
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "scale=-2:160") {
		t.Errorf("expected portrait scale=-2:160, got: %s", joined)
	}
}
