package urlbuilders

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
)

type SceneURLBuilder struct {
	BaseURL   string
	SceneID   string
	UpdatedAt string
}

func NewSceneURLBuilder(baseURL string, scene *models.Scene) SceneURLBuilder {
	return SceneURLBuilder{
		BaseURL:   baseURL,
		SceneID:   strconv.Itoa(scene.ID),
		UpdatedAt: strconv.FormatInt(scene.UpdatedAt.Unix(), 10),
	}
}

// appendAPIKey parses rawURL and, when apiKey is non-empty, sets the apikey query parameter,
// preserving any existing query parameters. This is the single apikey-append pattern reused by every
// scene media URL builder so the query-param auth channel works uniformly across stream, funscript,
// screenshot, preview, sprite, vtt, caption and heatmap URLs (Apple clients authenticate media by the
// apikey query param). When apiKey is empty the URL is returned unchanged.
func appendAPIKey(rawURL string, apiKey string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		// shouldn't happen
		panic(err)
	}

	if apiKey != "" {
		v := u.Query()
		v.Set("apikey", apiKey)
		u.RawQuery = v.Encode()
	}
	return u
}

func (b SceneURLBuilder) GetStreamURL(apiKey string) *url.URL {
	return appendAPIKey(fmt.Sprintf("%s/scene/%s/stream", b.BaseURL, b.SceneID), apiKey)
}

func (b SceneURLBuilder) GetStreamPreviewURL(apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+b.SceneID+"/preview", apiKey).String()
}

func (b SceneURLBuilder) GetStreamPreviewImageURL(apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+b.SceneID+"/webp", apiKey).String()
}

func (b SceneURLBuilder) GetSpriteVTTURL(checksum string, apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+checksum+"_thumbs.vtt", apiKey).String()
}

func (b SceneURLBuilder) GetSpriteURL(checksum string, apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+checksum+"_sprite.jpg", apiKey).String()
}

func (b SceneURLBuilder) GetScreenshotURL(apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+b.SceneID+"/screenshot?t="+b.UpdatedAt, apiKey).String()
}

func (b SceneURLBuilder) GetFunscriptURL(apiKey string) *url.URL {
	return appendAPIKey(fmt.Sprintf("%s/scene/%s/funscript", b.BaseURL, b.SceneID), apiKey)
}

func (b SceneURLBuilder) GetCaptionURL(apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+b.SceneID+"/caption", apiKey).String()
}

func (b SceneURLBuilder) GetInteractiveHeatmapURL(apiKey string) string {
	return appendAPIKey(b.BaseURL+"/scene/"+b.SceneID+"/interactive_heatmap", apiKey).String()
}
