package api

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/internal/static"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/file/video"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene/generate"
	"github.com/stashapp/stash/pkg/utils"
)

type SceneFinder interface {
	models.SceneGetter

	FindByChecksum(ctx context.Context, checksum string) ([]*models.Scene, error)
	FindByOSHash(ctx context.Context, oshash string) ([]*models.Scene, error)
	GetCover(ctx context.Context, sceneID int) ([]byte, error)
}

type SceneMarkerFinder interface {
	models.SceneMarkerGetter
	FindBySceneID(ctx context.Context, sceneID int) ([]*models.SceneMarker, error)
}

type SceneMarkerTagFinder interface {
	models.TagGetter
	FindBySceneMarkerID(ctx context.Context, sceneMarkerID int) ([]*models.Tag, error)
}

type CaptionFinder interface {
	GetCaptions(ctx context.Context, fileID models.FileID) ([]*models.VideoCaption, error)
}

type sceneRoutes struct {
	routes
	sceneFinder       SceneFinder
	fileGetter        models.FileGetter
	captionFinder     CaptionFinder
	sceneMarkerFinder SceneMarkerFinder
	tagFinder         SceneMarkerTagFinder
}

func (rs sceneRoutes) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/{sceneId}", func(r chi.Router) {
		r.Use(rs.SceneCtx)

		// streaming endpoints
		r.Get("/stream", rs.StreamDirect)
		r.Get("/stream.mp4", rs.StreamMp4)
		r.Get("/stream.webm", rs.StreamWebM)
		r.Get("/stream.mkv", rs.StreamMKV)
		r.Get("/stream.m3u8", rs.StreamHLS)
		r.Get("/stream.m3u8/{segment}.ts", rs.StreamHLSSegment)
		r.Get("/trickplay.m3u8", rs.TrickplayManifest)
		r.Get("/trickplay.ts", rs.TrickplayMedia)
		r.Get("/stream.mpd", rs.StreamDASH)
		r.Get("/stream.mpd/{segment}_v.webm", rs.StreamDASHVideoSegment)
		r.Get("/stream.mpd/{segment}_a.webm", rs.StreamDASHAudioSegment)

		r.Get("/screenshot", rs.Screenshot)
		r.Get("/preview", rs.Preview)
		r.Get("/webp", rs.Webp)
		r.Get("/vtt/chapter", rs.VttChapter)
		r.Get("/vtt/thumbs", rs.VttThumbs)
		r.Get("/vtt/sprite", rs.VttSprite)
		r.Get("/funscript", rs.Funscript)
		r.Get("/interactive_csv", rs.InteractiveCSV)
		r.Get("/interactive_heatmap", rs.InteractiveHeatmap)
		r.Get("/caption", rs.CaptionLang)

		r.Get("/scene_marker/{sceneMarkerId}/stream", rs.SceneMarkerStream)
		r.Get("/scene_marker/{sceneMarkerId}/preview", rs.SceneMarkerPreview)
		r.Get("/scene_marker/{sceneMarkerId}/screenshot", rs.SceneMarkerScreenshot)
	})
	r.Get("/{sceneHash}_thumbs.vtt", rs.VttThumbs)
	r.Get("/{sceneHash}_sprite.jpg", rs.VttSprite)

	return r
}

func (rs sceneRoutes) StreamDirect(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	ss := manager.SceneServer{
		TxnManager:       rs.txnManager,
		SceneCoverGetter: rs.sceneFinder,
	}
	ss.StreamSceneDirect(scene, w, r)
}

func (rs sceneRoutes) StreamMp4(w http.ResponseWriter, r *http.Request) {
	rs.streamTranscode(w, r, ffmpeg.StreamTypeMP4)
}

func (rs sceneRoutes) StreamWebM(w http.ResponseWriter, r *http.Request) {
	rs.streamTranscode(w, r, ffmpeg.StreamTypeWEBM)
}

func (rs sceneRoutes) StreamMKV(w http.ResponseWriter, r *http.Request) {
	// only allow mkv streaming if the scene container is an mkv already
	scene := r.Context().Value(sceneKey).(*models.Scene)

	pf := scene.Files.Primary()
	if pf == nil {
		return
	}

	container, err := manager.GetVideoFileContainer(pf)
	if err != nil {
		logger.Errorf("[transcode] error getting container: %v", err)
	}

	if container != ffmpeg.Matroska {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("not an mkv file")); err != nil {
			logger.Warnf("[stream] error writing to stream: %v", err)
		}
		return
	}

	rs.streamTranscode(w, r, ffmpeg.StreamTypeMKV)
}

func (rs sceneRoutes) streamTranscode(w http.ResponseWriter, r *http.Request, streamType ffmpeg.StreamFormat) {
	scene := r.Context().Value(sceneKey).(*models.Scene)

	streamManager := manager.GetInstance().StreamManager
	if streamManager == nil {
		http.Error(w, "Live transcoding disabled", http.StatusServiceUnavailable)
		return
	}

	f := scene.Files.Primary()
	if f == nil {
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Warnf("[transcode] error parsing query form: %v", err)
	}

	startTime := r.Form.Get("start")
	ss, _ := strconv.ParseFloat(startTime, 64)
	resolution := r.Form.Get("resolution")

	options := ffmpeg.TranscodeOptions{
		StreamType: streamType,
		VideoFile:  f,
		Resolution: resolution,
		StartTime:  ss,
	}

	logger.Debugf("[transcode] streaming scene %d as %s", scene.ID, streamType.MimeType)
	streamManager.ServeTranscode(w, r, options)
}

func (rs sceneRoutes) StreamHLS(w http.ResponseWriter, r *http.Request) {
	// When a scene has a trick-play artifact, stream.m3u8 returns an HLS *master* playlist that adds an
	// I-frame variant (native scrub previews). The main variant URI carries `type=media`, which routes
	// back here to the unchanged media playlist below. Scenes without the artifact, and that media
	// request, fall straight through to today's behavior — byte-identical, so the change is additive.
	if r.URL.Query().Get("type") != "media" && rs.serveHLSMasterIfTrickplay(w, r) {
		return
	}
	rs.streamManifest(w, r, ffmpeg.StreamTypeHLS, "HLS")
}

// serveHLSMasterIfTrickplay serves an HLS master playlist referencing the scene's trick-play I-frame
// variant, when that artifact exists. Returns false (serving nothing) otherwise, so the caller falls
// back to the plain media playlist.
func (rs sceneRoutes) serveHLSMasterIfTrickplay(w http.ResponseWriter, r *http.Request) bool {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())

	manifestPath := manager.GetInstance().Paths.Scene.GetTrickplayManifestFilePath(sceneHash)
	if exists, _ := fsutil.FileExists(manifestPath); !exists {
		return false
	}

	f := scene.Files.Primary()
	if f == nil {
		return false
	}

	apikey := r.URL.Query().Get("apikey")
	resolution := r.URL.Query().Get("resolution")

	// Main variant: route back to this endpoint as a media playlist (preserve resolution + apikey).
	mediaQuery := url.Values{}
	mediaQuery.Set("type", "media")
	if resolution != "" {
		mediaQuery.Set("resolution", resolution)
	}
	if apikey != "" {
		mediaQuery.Set("apikey", apikey)
	}
	mediaVariantURI := "stream.m3u8?" + mediaQuery.Encode()

	// I-frame variant: the trick-play playlist route (apikey carried explicitly for key-auth clients).
	iframeVariantURI := "trickplay.m3u8"
	if apikey != "" {
		iframeVariantURI += "?apikey=" + url.QueryEscape(apikey)
	}

	pl := buildHLSMasterPlaylist(f.Width, f.Height, f.BitRate, f.AudioCodec != "", manager.DefaultTrickplaySize, f.Height > f.Width, mediaVariantURI, iframeVariantURI)

	w.Header().Set("Content-Type", ffmpeg.MimeHLS)
	utils.ServeStaticContent(w, r, pl)
	return true
}

// TrickplayManifest serves the scene's stored I-frame playlist, injecting the apikey into the media
// URI for key-auth clients (cookie-auth clients resolve the relative URI + send their cookie).
func (rs sceneRoutes) TrickplayManifest(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())

	data, err := os.ReadFile(manager.GetInstance().Paths.Scene.GetTrickplayManifestFilePath(sceneHash))
	if err != nil {
		http.Error(w, "trickplay not generated", http.StatusNotFound)
		return
	}

	if apikey := r.URL.Query().Get("apikey"); apikey != "" {
		withKey := generate.TrickplayMediaURI + "?apikey=" + url.QueryEscape(apikey)
		data = bytes.ReplaceAll(data, []byte("\n"+generate.TrickplayMediaURI+"\n"), []byte("\n"+withKey+"\n"))
	}

	w.Header().Set("Content-Type", ffmpeg.MimeHLS)
	utils.ServeStaticContent(w, r, data)
}

// TrickplayMedia serves the keyframe-only MPEG-TS the I-frame playlist byte-ranges into. http.ServeFile
// honors Range requests, which is how the player fetches individual I-frames.
func (rs sceneRoutes) TrickplayMedia(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())

	// MPEG-TS — set explicitly since Go doesn't map the .ts extension. http.ServeContent honors a
	// pre-set Content-Type and still serves Range requests (how the player fetches each I-frame).
	w.Header().Set("Content-Type", ffmpeg.MimeMpegTS)
	utils.ServeStaticFile(w, r, manager.GetInstance().Paths.Scene.GetTrickplayMediaFilePath(sceneHash))
}

// trickplayCodec is the CODECS string for the trick-play I-frame variant. Must match generate.Trickplay's
// pinned H.264 baseline@3.0 encode.
const trickplayCodec = "avc1.42E01E"

// mainVideoCodec / aacAudioCodec declare the main variant's formats. H.264 High@5.2 is an intentional
// over-declaration (covers up to 4K so we never under-declare the live transcode's actual level); AAC-LC
// matches the transcode's `-c:a aac` output.
const (
	mainVideoCodec = "avc1.640034"
	aacAudioCodec  = "mp4a.40.2"
)

// buildHLSMasterPlaylist builds an HLS master playlist with the scene's main (live-transcoded) variant
// plus a pre-generated I-frame variant for trick-play. Pure value→value so it's unit-testable.
func buildHLSMasterPlaylist(width, height int, bitrate int64, hasAudio bool, trickSize int, isPortrait bool, mediaVariantURI, iframeVariantURI string) []byte {
	if bitrate <= 0 {
		bitrate = 4000000
	}

	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:7\n")
	buf.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")

	// CODECS on the MAIN variant is required for AVPlayer to select the I-frame track for scrub
	// thumbnails (without it the player fetches the I-frames but won't display them). The live transcode
	// emits H.264 + AAC-LC; we over-declare High@5.2 (covers up to 4K — a decoder-capability ceiling, so
	// it never under-declares an actual stream) and add AAC only when the scene has an audio track.
	mainCodecs := mainVideoCodec
	if hasAudio {
		mainCodecs += "," + aacAudioCodec
	}
	streamInf := "#EXT-X-STREAM-INF:BANDWIDTH=" + strconv.FormatInt(bitrate, 10) + `,CODECS="` + mainCodecs + `"`
	if width > 0 && height > 0 {
		streamInf += ",RESOLUTION=" + strconv.Itoa(width) + "x" + strconv.Itoa(height)
	}
	buf.WriteString(streamInf + "\n")
	buf.WriteString(mediaVariantURI + "\n")

	// CODECS is required for AVPlayer/tvOS to actually use the I-frame variant for trick-play. It must
	// match the generator's pinned encode (H.264 baseline@3.0) — see generate.Trickplay.
	iframeInf := "#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=" + strconv.FormatInt(iframeBandwidth(bitrate), 10) + `,CODECS="` + trickplayCodec + `"`
	if tw, th := trickplayDimensions(width, height, trickSize, isPortrait); tw > 0 && th > 0 {
		iframeInf += ",RESOLUTION=" + strconv.Itoa(tw) + "x" + strconv.Itoa(th)
	}
	iframeInf += `,URI="` + iframeVariantURI + `"`
	buf.WriteString(iframeInf + "\n")

	return buf.Bytes()
}

// iframeBandwidth is a coarse, conservative estimate for the I-frame variant's BANDWIDTH attribute
// (required by the spec; players don't switch to it for normal playback).
func iframeBandwidth(mainBitrate int64) int64 {
	b := mainBitrate / 20
	if b < 50000 {
		b = 50000
	}
	return b
}

// trickplayDimensions scales the source dimensions so the longest side is trickSize, preserving aspect
// (for the I-frame variant's RESOLUTION attribute). Returns (0,0) when source dimensions are unknown.
func trickplayDimensions(width, height, trickSize int, isPortrait bool) (int, int) {
	if width <= 0 || height <= 0 || trickSize <= 0 {
		return 0, 0
	}
	even := func(n int) int {
		if n%2 != 0 {
			n++
		}
		return n
	}
	if isPortrait {
		return even(int(math.Round(float64(trickSize) * float64(width) / float64(height)))), trickSize
	}
	return trickSize, even(int(math.Round(float64(trickSize) * float64(height) / float64(width))))
}

func (rs sceneRoutes) StreamDASH(w http.ResponseWriter, r *http.Request) {
	rs.streamManifest(w, r, ffmpeg.StreamTypeDASHVideo, "DASH")
}

func (rs sceneRoutes) streamManifest(w http.ResponseWriter, r *http.Request, streamType *ffmpeg.StreamType, logName string) {
	scene := r.Context().Value(sceneKey).(*models.Scene)

	streamManager := manager.GetInstance().StreamManager
	if streamManager == nil {
		http.Error(w, "Live transcoding disabled", http.StatusServiceUnavailable)
		return
	}

	f := scene.Files.Primary()
	if f == nil {
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Warnf("[transcode] error parsing query form: %v", err)
	}

	resolution := r.Form.Get("resolution")

	logger.Debugf("[transcode] returning %s manifest for scene %d", logName, scene.ID)
	streamManager.ServeManifest(w, r, streamType, f, resolution)
}

func (rs sceneRoutes) StreamHLSSegment(w http.ResponseWriter, r *http.Request) {
	rs.streamSegment(w, r, ffmpeg.StreamTypeHLS)
}

func (rs sceneRoutes) StreamDASHVideoSegment(w http.ResponseWriter, r *http.Request) {
	rs.streamSegment(w, r, ffmpeg.StreamTypeDASHVideo)
}

func (rs sceneRoutes) StreamDASHAudioSegment(w http.ResponseWriter, r *http.Request) {
	rs.streamSegment(w, r, ffmpeg.StreamTypeDASHAudio)
}

func (rs sceneRoutes) streamSegment(w http.ResponseWriter, r *http.Request, streamType *ffmpeg.StreamType) {
	scene := r.Context().Value(sceneKey).(*models.Scene)

	streamManager := manager.GetInstance().StreamManager
	if streamManager == nil {
		http.Error(w, "Live transcoding disabled", http.StatusServiceUnavailable)
		return
	}

	f := scene.Files.Primary()
	if f == nil {
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Warnf("[transcode] error parsing query form: %v", err)
	}

	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())

	segment := chi.URLParam(r, "segment")
	resolution := r.Form.Get("resolution")

	options := ffmpeg.StreamOptions{
		StreamType: streamType,
		VideoFile:  f,
		Resolution: resolution,
		Hash:       sceneHash,
		Segment:    segment,
	}

	streamManager.ServeSegment(w, r, options)
}

func (rs sceneRoutes) Screenshot(w http.ResponseWriter, r *http.Request) {
	// if default flag is set, return the default image
	if r.URL.Query().Get("default") == "true" {
		utils.ServeImage(w, r, static.ReadAll(static.DefaultSceneImage))
		return
	}

	scene := r.Context().Value(sceneKey).(*models.Scene)

	ss := manager.SceneServer{
		TxnManager:       rs.txnManager,
		SceneCoverGetter: rs.sceneFinder,
	}
	ss.ServeScreenshot(scene, w, r)
}

func (rs sceneRoutes) Preview(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	filepath := manager.GetInstance().Paths.Scene.GetVideoPreviewPath(sceneHash)

	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) Webp(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	filepath := manager.GetInstance().Paths.Scene.GetWebpPreviewPath(sceneHash)

	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) getChapterVttTitle(r *http.Request, marker *models.SceneMarker) (*string, error) {
	if marker.Title != "" {
		return &marker.Title, nil
	}

	var title string
	if err := rs.withReadTxn(r, func(ctx context.Context) error {
		qb := rs.tagFinder
		primaryTag, err := qb.Find(ctx, marker.PrimaryTagID)
		if err != nil {
			return err
		}

		title = primaryTag.Name

		tags, err := qb.FindBySceneMarkerID(ctx, marker.ID)
		if err != nil {
			return err
		}

		for _, t := range tags {
			title += ", " + t.Name
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &title, nil
}

func (rs sceneRoutes) VttChapter(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	var sceneMarkers []*models.SceneMarker
	readTxnErr := rs.withReadTxn(r, func(ctx context.Context) error {
		var err error
		sceneMarkers, err = rs.sceneMarkerFinder.FindBySceneID(ctx, scene.ID)
		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch scene markers: %v", readTxnErr)
		http.Error(w, readTxnErr.Error(), http.StatusInternalServerError)
		return
	}

	vttLines := []string{"WEBVTT", ""}
	for i, marker := range sceneMarkers {
		vttLines = append(vttLines, strconv.Itoa(i+1))
		time := utils.GetVTTTime(marker.Seconds)
		vttLines = append(vttLines, time+" --> "+time)

		vttTitle, err := rs.getChapterVttTitle(r, marker)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			logger.Warnf("read transaction error on fetch scene marker title: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		vttLines = append(vttLines, *vttTitle)
		vttLines = append(vttLines, "")
	}
	vtt := strings.Join(vttLines, "\n")

	w.Header().Set("Content-Type", "text/vtt")
	utils.ServeStaticContent(w, r, []byte(vtt))
}

func (rs sceneRoutes) VttThumbs(w http.ResponseWriter, r *http.Request) {
	scene, ok := r.Context().Value(sceneKey).(*models.Scene)
	var sceneHash string
	if ok && scene != nil {
		sceneHash = scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	} else {
		sceneHash = chi.URLParam(r, "sceneHash")
	}
	filepath := manager.GetInstance().Paths.Scene.GetSpriteVttFilePath(sceneHash)

	w.Header().Set("Content-Type", "text/vtt")
	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) VttSprite(w http.ResponseWriter, r *http.Request) {
	scene, ok := r.Context().Value(sceneKey).(*models.Scene)
	var sceneHash string
	if ok && scene != nil {
		sceneHash = scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	} else {
		sceneHash = chi.URLParam(r, "sceneHash")
	}
	filepath := manager.GetInstance().Paths.Scene.GetSpriteImageFilePath(sceneHash)

	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) Funscript(w http.ResponseWriter, r *http.Request) {
	s := r.Context().Value(sceneKey).(*models.Scene)
	filepath := video.GetFunscriptPath(s.Path)

	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) InteractiveCSV(w http.ResponseWriter, r *http.Request) {
	s := r.Context().Value(sceneKey).(*models.Scene)
	filepath := video.GetFunscriptPath(s.Path)

	// TheHandy directly only accepts interactive CSVs
	csvBytes, err := manager.ConvertFunscriptToCSV(filepath)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	utils.ServeStaticContent(w, r, csvBytes)
}

func (rs sceneRoutes) InteractiveHeatmap(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	filepath := manager.GetInstance().Paths.Scene.GetInteractiveHeatmapPath(sceneHash)

	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) Caption(w http.ResponseWriter, r *http.Request, lang string, ext string) {
	s := r.Context().Value(sceneKey).(*models.Scene)

	var captions []*models.VideoCaption
	readTxnErr := rs.withReadTxn(r, func(ctx context.Context) error {
		var err error
		primaryFile := s.Files.Primary()
		if primaryFile == nil {
			return nil
		}

		captions, err = rs.captionFinder.GetCaptions(ctx, primaryFile.Base().ID)

		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch scene captions: %v", readTxnErr)
		http.Error(w, readTxnErr.Error(), http.StatusInternalServerError)
		return
	}

	for _, caption := range captions {
		if lang != caption.LanguageCode || ext != caption.CaptionType {
			continue
		}

		sub, err := video.ReadSubs(caption.Path(s.Path))
		if err != nil {
			logger.Warnf("error while reading subs: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var buf bytes.Buffer

		err = sub.WriteToWebVTT(&buf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/vtt")
		utils.ServeStaticContent(w, r, buf.Bytes())
		return
	}
}

func (rs sceneRoutes) CaptionLang(w http.ResponseWriter, r *http.Request) {
	// serve caption based on lang query param, if provided
	if err := r.ParseForm(); err != nil {
		logger.Warnf("[caption] error parsing query form: %v", err)
	}

	l := r.Form.Get("lang")
	ext := r.Form.Get("type")
	rs.Caption(w, r, l, ext)
}

func (rs sceneRoutes) SceneMarkerStream(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	sceneMarkerID, _ := strconv.Atoi(chi.URLParam(r, "sceneMarkerId"))
	var sceneMarker *models.SceneMarker
	readTxnErr := rs.withReadTxn(r, func(ctx context.Context) error {
		var err error
		sceneMarker, err = rs.sceneMarkerFinder.Find(ctx, sceneMarkerID)
		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch scene marker: %v", readTxnErr)
		http.Error(w, readTxnErr.Error(), http.StatusInternalServerError)
		return
	}

	if sceneMarker == nil {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	filepath := manager.GetInstance().Paths.SceneMarkers.GetVideoPreviewPath(sceneHash, int(sceneMarker.Seconds))
	utils.ServeStaticFile(w, r, filepath)
}

func (rs sceneRoutes) SceneMarkerPreview(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	sceneMarkerID, _ := strconv.Atoi(chi.URLParam(r, "sceneMarkerId"))
	var sceneMarker *models.SceneMarker
	readTxnErr := rs.withReadTxn(r, func(ctx context.Context) error {
		var err error
		sceneMarker, err = rs.sceneMarkerFinder.Find(ctx, sceneMarkerID)
		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch scene marker preview: %v", readTxnErr)
		http.Error(w, readTxnErr.Error(), http.StatusInternalServerError)
		return
	}

	if sceneMarker == nil {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	filepath := manager.GetInstance().Paths.SceneMarkers.GetWebpPreviewPath(sceneHash, int(sceneMarker.Seconds))

	// If the image doesn't exist, send the placeholder
	exists, _ := fsutil.FileExists(filepath)
	if !exists {
		w.Header().Set("Content-Type", "image/png")
		utils.ServeStaticContent(w, r, utils.PendingGenerateResource)
	} else {
		utils.ServeStaticFile(w, r, filepath)
	}
}

func (rs sceneRoutes) SceneMarkerScreenshot(w http.ResponseWriter, r *http.Request) {
	scene := r.Context().Value(sceneKey).(*models.Scene)
	sceneHash := scene.GetHash(config.GetInstance().GetVideoFileNamingAlgorithm())
	sceneMarkerID, _ := strconv.Atoi(chi.URLParam(r, "sceneMarkerId"))
	var sceneMarker *models.SceneMarker
	readTxnErr := rs.withReadTxn(r, func(ctx context.Context) error {
		var err error
		sceneMarker, err = rs.sceneMarkerFinder.Find(ctx, sceneMarkerID)
		return err
	})
	if errors.Is(readTxnErr, context.Canceled) {
		return
	}
	if readTxnErr != nil {
		logger.Warnf("read transaction error on fetch scene marker screenshot: %v", readTxnErr)
		http.Error(w, readTxnErr.Error(), http.StatusInternalServerError)
		return
	}

	if sceneMarker == nil {
		http.Error(w, http.StatusText(404), 404)
		return
	}

	filepath := manager.GetInstance().Paths.SceneMarkers.GetScreenshotPath(sceneHash, int(sceneMarker.Seconds))

	// If the image doesn't exist, send the placeholder
	exists, _ := fsutil.FileExists(filepath)
	if !exists {
		w.Header().Set("Content-Type", "image/png")
		utils.ServeStaticContent(w, r, utils.PendingGenerateResource)
	} else {
		utils.ServeStaticFile(w, r, filepath)
	}
}

func (rs sceneRoutes) SceneCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sceneID, err := strconv.Atoi(chi.URLParam(r, "sceneId"))
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		var scene *models.Scene
		_ = rs.withReadTxn(r, func(ctx context.Context) error {
			qb := rs.sceneFinder
			scene, _ = qb.Find(ctx, sceneID)

			if scene != nil {
				if err := scene.LoadPrimaryFile(ctx, rs.fileGetter); err != nil {
					if !errors.Is(err, context.Canceled) {
						logger.Errorf("error loading primary file for scene %d: %v", sceneID, err)
					}
					// set scene to nil so that it doesn't try to use the primary file
					scene = nil
				}
			}

			return nil
		})
		if scene == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		ctx := context.WithValue(r.Context(), sceneKey, scene)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
