package generate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
)

// TrickplayMediaURI is the (per-scene, relative) URI the trick-play playlist points its byte-range
// segments at. The serving layer resolves it to the scene's trick-play media route (and injects an
// apikey when needed). It is constant because the route is already scoped by scene id.
const TrickplayMediaURI = "trickplay.ts"

// TrickplayOptions configures trick-play (HLS I-frame) artifact generation.
type TrickplayOptions struct {
	// Interval is the seconds between successive I-frames (one thumbnail per interval).
	Interval float64
	// Size is the longest-dimension pixel size of each I-frame (aspect ratio preserved).
	Size int
	// IsPortrait selects whether Size constrains height (portrait) or width (landscape).
	IsPortrait bool
}

// Trickplay generates the HLS I-frame ("trick-play") artifact native players use to show scrub
// thumbnails: a single keyframe-only MPEG-TS (mediaOutput) plus an EXT-X-I-FRAMES-ONLY playlist
// (manifestOutput) that byte-ranges into it.
//
// ffmpeg's `single_file+iframes_only` muxer writes the .ts correctly (frames concatenated) but emits
// a playlist whose byte-range offsets are all `@0` — so we keep ffmpeg's .ts and rewrite the playlist
// ourselves with correct cumulative offsets (see rewriteTrickplayManifest).
func (g Generator) Trickplay(ctx context.Context, input string, opts TrickplayOptions, manifestOutput, mediaOutput string) error {
	lockCtx := g.LockManager.ReadLock(ctx, input)
	defer lockCtx.Cancel()

	tmpDir, err := os.MkdirTemp("", "stash-trickplay-")
	if err != nil {
		return fmt.Errorf("creating trickplay temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpTS := filepath.Join(tmpDir, "trickplay.ts")
	tmpM3U8 := filepath.Join(tmpDir, "trickplay.m3u8")

	if err := g.generate(lockCtx, trickplayArgs(input, opts, tmpTS, tmpM3U8)); err != nil {
		return err
	}

	ffmpegPlaylist, err := os.ReadFile(tmpM3U8)
	if err != nil {
		return fmt.Errorf("reading generated trickplay playlist: %w", err)
	}

	fixed, err := rewriteTrickplayManifest(ffmpegPlaylist, TrickplayMediaURI)
	if err != nil {
		return fmt.Errorf("rewriting trickplay playlist: %w", err)
	}

	if stat, err := os.Stat(tmpTS); err != nil || stat.Size() == 0 {
		return fmt.Errorf("trickplay produced no media output")
	}

	if err := fsutil.SafeMove(tmpTS, mediaOutput); err != nil {
		return fmt.Errorf("moving trickplay media to %s: %w", mediaOutput, err)
	}
	if err := os.WriteFile(manifestOutput, fixed, 0644); err != nil {
		return fmt.Errorf("writing trickplay manifest to %s: %w", manifestOutput, err)
	}

	return nil
}

func trickplayArgs(input string, opts TrickplayOptions, tsPath, m3u8Path string) ffmpeg.Args {
	scale := fmt.Sprintf("scale=%d:-2", opts.Size)
	if opts.IsPortrait {
		scale = fmt.Sprintf("scale=-2:%d", opts.Size)
	}
	fps := 1.0 / opts.Interval

	var args ffmpeg.Args
	args = args.Overwrite()
	args = args.Input(input)
	args = append(args,
		"-an", "-sn",
		"-vf", fmt.Sprintf("%s,fps=%.6f", scale, fps),
		// Every output frame is an IDR keyframe so each playlist segment is a single seekable I-frame.
		// Pinned to H.264 baseline@3.0 (CODECS "avc1.42E01E") so the master playlist can declare an
		// exact, constant CODECS for the I-frame variant — required for AVPlayer/tvOS trick-play.
		"-c:v", "libx264", "-preset", "veryfast",
		"-profile:v", "baseline", "-level", "3.0",
		"-g", "1", "-keyint_min", "1", "-sc_threshold", "0",
		"-pix_fmt", "yuv420p",
		"-f", "hls",
		"-hls_time", strconv.FormatFloat(opts.Interval, 'f', -1, 64),
		"-hls_list_size", "0",
		"-hls_flags", "single_file+iframes_only",
		"-hls_segment_type", "mpegts",
		"-hls_playlist_type", "vod",
		"-hls_segment_filename", tsPath,
		m3u8Path,
	)
	return args
}

// rewriteTrickplayManifest takes the playlist ffmpeg emits for `single_file+iframes_only` (whose
// byte-range offsets are all `@0`) and rewrites it with correct cumulative offsets and a stable media
// URI. ffmpeg writes the per-frame lengths correctly and concatenates the frames in order, so the
// nth frame lives at offset = sum of all preceding lengths.
func rewriteTrickplayManifest(ffmpegPlaylist []byte, mediaURI string) ([]byte, error) {
	type seg struct {
		extinf string
		length int64
	}
	var segs []seg

	scanner := bufio.NewScanner(bytes.NewReader(ffmpegPlaylist))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var pendingExtinf string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			pendingExtinf = line
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			// form: #EXT-X-BYTERANGE:<length>@<offset>
			spec := strings.TrimPrefix(line, "#EXT-X-BYTERANGE:")
			lenStr := spec
			if at := strings.IndexByte(spec, '@'); at >= 0 {
				lenStr = spec[:at]
			}
			length, err := strconv.ParseInt(strings.TrimSpace(lenStr), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing byterange length %q: %w", lenStr, err)
			}
			segs = append(segs, seg{extinf: pendingExtinf, length: length})
			pendingExtinf = ""
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("no byte-range segments in trickplay playlist")
	}

	// Target duration is the ceiling of the longest segment duration.
	var maxDur float64
	for _, s := range segs {
		if d := parseExtinfSeconds(s.extinf); d > maxDur {
			maxDur = d
		}
	}

	var buf bytes.Buffer
	fmt.Fprint(&buf, "#EXTM3U\n")
	fmt.Fprint(&buf, "#EXT-X-VERSION:4\n")
	fmt.Fprintf(&buf, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(maxDur)))
	fmt.Fprint(&buf, "#EXT-X-MEDIA-SEQUENCE:0\n")
	fmt.Fprint(&buf, "#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprint(&buf, "#EXT-X-I-FRAMES-ONLY\n")

	var offset int64
	for _, s := range segs {
		if s.extinf != "" {
			fmt.Fprintf(&buf, "%s\n", s.extinf)
		}
		fmt.Fprintf(&buf, "#EXT-X-BYTERANGE:%d@%d\n", s.length, offset)
		fmt.Fprintf(&buf, "%s\n", mediaURI)
		offset += s.length
	}
	fmt.Fprint(&buf, "#EXT-X-ENDLIST\n")

	return buf.Bytes(), nil
}

func parseExtinfSeconds(extinf string) float64 {
	spec := strings.TrimPrefix(extinf, "#EXTINF:")
	spec = strings.TrimSuffix(strings.TrimSpace(spec), ",")
	if comma := strings.IndexByte(spec, ','); comma >= 0 {
		spec = spec[:comma]
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(spec), 64)
	return d
}
