package paths

import (
	"path/filepath"

	"github.com/stashapp/stash/pkg/fsutil"
)

type scenePaths struct {
	generatedPaths
}

func newScenePaths(p Paths) *scenePaths {
	sp := scenePaths{
		generatedPaths: *p.Generated,
	}
	return &sp
}

func (sp *scenePaths) GetLegacyScreenshotPath(checksum string) string {
	return filepath.Join(sp.Screenshots, checksum+".jpg")
}

func (sp *scenePaths) GetTranscodePath(checksum string) string {
	return filepath.Join(sp.Transcodes, checksum+".mp4")
}

func (sp *scenePaths) GetStreamPath(scenePath string, checksum string) string {
	transcodePath := sp.GetTranscodePath(checksum)
	transcodeExists, _ := fsutil.FileExists(transcodePath)
	if transcodeExists {
		return transcodePath
	}
	return scenePath
}

func (sp *scenePaths) GetVideoPreviewPath(checksum string) string {
	return filepath.Join(sp.Screenshots, checksum+".mp4")
}

func (sp *scenePaths) GetWebpPreviewPath(checksum string) string {
	return filepath.Join(sp.Screenshots, checksum+".webp")
}

func (sp *scenePaths) GetSpriteImageFilePath(checksum string) string {
	return filepath.Join(sp.Vtt, checksum+"_sprite.jpg")
}

func (sp *scenePaths) GetSpriteVttFilePath(checksum string) string {
	return filepath.Join(sp.Vtt, checksum+"_thumbs.vtt")
}

// GetTrickplayManifestFilePath is the HLS I-frame ("trick-play") playlist used for native scrub
// thumbnails. Generated alongside the sprite (same cadence) and served as an EXT-X-I-FRAME-STREAM-INF
// variant of the scene's HLS master playlist.
func (sp *scenePaths) GetTrickplayManifestFilePath(checksum string) string {
	return filepath.Join(sp.Vtt, checksum+"_trickplay.m3u8")
}

// GetTrickplayMediaFilePath is the single keyframe-only MPEG-TS the trick-play playlist byte-ranges
// into (one I-frame per range).
func (sp *scenePaths) GetTrickplayMediaFilePath(checksum string) string {
	return filepath.Join(sp.Vtt, checksum+"_trickplay.ts")
}

func (sp *scenePaths) GetInteractiveHeatmapPath(checksum string) string {
	return filepath.Join(sp.InteractiveHeatmap, checksum+".png")
}
