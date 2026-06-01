package manager

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/scene/generate"
)

const (
	// DefaultTrickplaySize is the longest-dimension pixel size of each trick-play I-frame. Matches the
	// sprite default so scrub previews and sprites are visually consistent.
	DefaultTrickplaySize = 160

	// DefaultTrickplayMaxInterval is the density floor: trick-play I-frames are never spaced further
	// apart than this (seconds), so long videos stay smoothly scrubbable even though the sprite cadence
	// is duration-independent.
	DefaultTrickplayMaxInterval = 10.0
)

// TrickplayGenerator generates the HLS I-frame ("trick-play") artifact for a scene at the sprite
// cadence, bounded by the density floor (see DefaultTrickplayMaxInterval).
type TrickplayGenerator struct {
	VideoFile          ffmpeg.VideoFile
	ManifestOutputPath string
	MediaOutputPath    string
	Interval           float64
	Size               int

	Overwrite bool

	g *generate.Generator
}

// NewTrickplayGenerator derives the I-frame interval from the same sprite cadence (spriteCfg), then
// caps it at the density floor so long videos aren't under-sampled.
func NewTrickplayGenerator(videoFile ffmpeg.VideoFile, manifestOutput string, mediaOutput string, spriteCfg SpriteGeneratorConfig, size int) (*TrickplayGenerator, error) {
	exists, err := fsutil.FileExists(videoFile.Path)
	if !exists {
		return nil, err
	}

	if videoFile.VideoStreamDuration <= 0 {
		return nil, fmt.Errorf("video %s: duration(%.3f) invalid, skipping trickplay creation", videoFile.Path, videoFile.VideoStreamDuration)
	}

	interval := math.Min(calculateSpriteInterval(videoFile, spriteCfg), DefaultTrickplayMaxInterval)
	if interval <= 0 {
		return nil, errors.New("invalid trickplay interval")
	}

	if size <= 0 {
		size = DefaultTrickplaySize
	}

	return &TrickplayGenerator{
		VideoFile:          videoFile,
		ManifestOutputPath: manifestOutput,
		MediaOutputPath:    mediaOutput,
		Interval:           interval,
		Size:               size,
		g: &generate.Generator{
			Encoder:      instance.FFMpeg,
			FFMpegConfig: instance.Config,
			LockManager:  instance.ReadLockManager,
			ScenePaths:   instance.Paths.Scene,
		},
	}, nil
}

func (g *TrickplayGenerator) Generate() error {
	if !g.Overwrite && g.exists() {
		return nil
	}

	logger.Infof("[generator] generating trickplay for %s", g.VideoFile.Path)

	return g.g.Trickplay(context.TODO(), g.VideoFile.Path, generate.TrickplayOptions{
		Interval:   g.Interval,
		Size:       g.Size,
		IsPortrait: g.VideoFile.Height > g.VideoFile.Width,
	}, g.ManifestOutputPath, g.MediaOutputPath)
}

func (g *TrickplayGenerator) exists() bool {
	manifestExists, _ := fsutil.FileExists(g.ManifestOutputPath)
	mediaExists, _ := fsutil.FileExists(g.MediaOutputPath)
	return manifestExists && mediaExists
}
