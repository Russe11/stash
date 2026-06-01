package manager

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

type GenerateSpriteTask struct {
	Scene               models.Scene
	Overwrite           bool
	fileNamingAlgorithm models.HashAlgorithm
}

func (t *GenerateSpriteTask) GetDescription() string {
	return fmt.Sprintf("Generating sprites and trick-play for %s", t.Scene.Path)
}

func (t *GenerateSpriteTask) Start(ctx context.Context) {
	if !t.required() {
		return
	}

	ffprobe := instance.FFProbe
	videoFile, err := ffprobe.NewVideoFile(t.Scene.Path)
	if err != nil {
		logger.Errorf("error reading video file: %s", err.Error())
		return
	}

	sceneHash := t.Scene.GetHash(t.fileNamingAlgorithm)
	imagePath := instance.Paths.Scene.GetSpriteImageFilePath(sceneHash)
	vttPath := instance.Paths.Scene.GetSpriteVttFilePath(sceneHash)

	cfg := DefaultSpriteGeneratorConfig
	cfg.SpriteSize = instance.Config.GetSpriteScreenshotSize()

	if instance.Config.GetUseCustomSpriteInterval() {
		cfg.MinimumSprites = instance.Config.GetMinimumSprites()
		cfg.MaximumSprites = instance.Config.GetMaximumSprites()
		cfg.SpriteInterval = instance.Config.GetSpriteInterval()
	}

	generator, err := NewSpriteGenerator(*videoFile, sceneHash, imagePath, vttPath, cfg)

	if err != nil {
		logger.Errorf("error creating sprite generator: %s", err.Error())
		return
	}
	generator.Overwrite = t.Overwrite

	if err := generator.Generate(); err != nil {
		logger.Errorf("error generating sprite: %s", err.Error())
		logErrorOutput(err)
		return
	}

	// Trick-play (HLS I-frame) artifact for native scrub previews — generated at the same sprite
	// cadence (bounded by the density floor). Bundled here so it's on-by-default with no new generate
	// option; a sprite failure above already returned, so the sprite is preserved if this fails.
	trickplayManifestPath := instance.Paths.Scene.GetTrickplayManifestFilePath(sceneHash)
	trickplayMediaPath := instance.Paths.Scene.GetTrickplayMediaFilePath(sceneHash)

	trickplayGenerator, err := NewTrickplayGenerator(*videoFile, trickplayManifestPath, trickplayMediaPath, cfg, instance.Config.GetSpriteScreenshotSize())
	if err != nil {
		logger.Errorf("error creating trickplay generator: %s", err.Error())
		return
	}
	trickplayGenerator.Overwrite = t.Overwrite

	if err := trickplayGenerator.Generate(); err != nil {
		logger.Errorf("error generating trickplay: %s", err.Error())
		logErrorOutput(err)
		return
	}
}

// required returns true if the sprite needs to be generated
func (t GenerateSpriteTask) required() bool {
	if t.Scene.Path == "" {
		return false
	}

	if t.Overwrite {
		return true
	}

	sceneHash := t.Scene.GetHash(t.fileNamingAlgorithm)
	// Trick-play is bundled here, so the task is also required when only it is missing — this is how a
	// library that already has sprites backfills trick-play on the next generate pass.
	return !t.doesSpriteExist(sceneHash) || !t.doesTrickplayExist(sceneHash)
}

func (t *GenerateSpriteTask) doesSpriteExist(sceneChecksum string) bool {
	if sceneChecksum == "" {
		return false
	}

	imageExists, _ := fsutil.FileExists(instance.Paths.Scene.GetSpriteImageFilePath(sceneChecksum))
	vttExists, _ := fsutil.FileExists(instance.Paths.Scene.GetSpriteVttFilePath(sceneChecksum))
	return imageExists && vttExists
}

func (t *GenerateSpriteTask) doesTrickplayExist(sceneChecksum string) bool {
	if sceneChecksum == "" {
		return false
	}

	manifestExists, _ := fsutil.FileExists(instance.Paths.Scene.GetTrickplayManifestFilePath(sceneChecksum))
	mediaExists, _ := fsutil.FileExists(instance.Paths.Scene.GetTrickplayMediaFilePath(sceneChecksum))
	return manifestExists && mediaExists
}
