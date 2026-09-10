//go:build media_engine

package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/graycode-cli/internal/stt"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
	graycoderouterengine "github.com/GrayCodeAI/graycode-router/engine"
)

// Env gates for opt-in backend wiring (Gap-05). Off by default so the default
// tool behavior is unchanged: unwired seams still fail safe with a clear error.
// Setting a gate to "1" wires the matching tool seam to the router engine
// facade. Credentials and endpoint come from the companion env vars; no new
// secret store path is introduced.
const (
	envMediaGate = "GRAYCODE_MEDIA"
	envSTTGate   = "GRAYCODE_STT"
)

// routerMediaEngine implements tool.MediaEngine against the router engine
// facade's image-generation backend. Video generation has no router facade
// yet, so it preserves the unwired fail-safe error.
type routerMediaEngine struct {
	eng     *graycoderouterengine.Engine
	apiKey  string
	baseURL string
	model   string
}

func (e *routerMediaEngine) Name() string { return "graycode-router" }

func (e *routerMediaEngine) GenerateImage(ctx context.Context, prompt, source string, opts tool.MediaOptions) ([]tool.MediaResult, error) {
	if source != "" {
		return nil, fmt.Errorf("image editing via the graycode-router backend is not supported; generate a new image instead")
	}
	n := opts.Count
	if n <= 0 {
		n = 1
	}
	res, err := e.eng.GenerateImage(ctx, graycoderouterengine.GenerateImageRequest{
		MediaOptions: graycoderouterengine.MediaOptions{APIKey: e.apiKey, BaseURL: e.baseURL},
		Prompt:       prompt,
		Model:        e.model,
		Size:         mediaSize(opts),
		N:            n,
	})
	if err != nil {
		return nil, err
	}
	out := make([]tool.MediaResult, 0, len(res))
	for _, r := range res {
		out = append(out, tool.MediaResult{Data: r.Image, URL: r.ProviderURL, Kind: "image", MIME: "image/png"})
	}
	return out, nil
}

func (e *routerMediaEngine) GenerateVideo(ctx context.Context, prompt, source string, opts tool.MediaOptions) ([]tool.MediaResult, error) {
	return nil, fmt.Errorf("video generation via the graycode-router backend is not wired; no video backend installed")
}

// routerTranscriber implements stt.Transcriber against the router engine
// facade's audio-transcription backend.
type routerTranscriber struct {
	eng     *graycoderouterengine.Engine
	apiKey  string
	baseURL string
	model   string
}

func (t *routerTranscriber) Name() string { return "graycode-router" }

func (t *routerTranscriber) Transcribe(ctx context.Context, localPath, language string) (string, error) {
	audio, err := os.ReadFile(localPath)
	if err != nil {
		return "", fmt.Errorf("read audio for transcription: %w", err)
	}
	return t.eng.Transcribe(ctx, graycoderouterengine.TranscribeRequest{
		MediaOptions: graycoderouterengine.MediaOptions{APIKey: t.apiKey, BaseURL: t.baseURL},
		Audio:        audio,
		FileName:     filepath.Base(localPath),
		Model:        t.model,
		Language:     language,
	})
}

// wireOptionalBackends installs env-gated media/STT backends onto the shared
// tool/stt seams. It is a no-op unless the corresponding env gate is "1".
// Called once from the gateway composition root (New) so all construction
// paths wire identically.
func wireOptionalBackends(eng *graycoderouterengine.Engine) {
	if os.Getenv(envMediaGate) == "1" {
		tool.SetMediaEngine(&routerMediaEngine{
			eng:     eng,
			apiKey:  os.Getenv("GRAYCODE_MEDIA_API_KEY"),
			baseURL: os.Getenv("GRAYCODE_MEDIA_BASE_URL"),
			model:   os.Getenv("GRAYCODE_MEDIA_MODEL"),
		})
	}
	if os.Getenv(envSTTGate) == "1" {
		stt.SetTranscriber(&routerTranscriber{
			eng:     eng,
			apiKey:  os.Getenv("GRAYCODE_STT_API_KEY"),
			baseURL: os.Getenv("GRAYCODE_STT_BASE_URL"),
			model:   os.Getenv("GRAYCODE_STT_MODEL"),
		})
	}
}

// mediaSize maps MediaOptions to an OpenAI-compatible size token. It is a
// best-effort heuristic; callers can pin a resolution directly.
func mediaSize(opts tool.MediaOptions) string {
	switch opts.AspectRatio {
	case "16:9":
		return "1792x1024"
	case "9:16":
		return "1024x1792"
	default:
		return "1024x1024"
	}
}
