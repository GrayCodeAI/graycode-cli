package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
	"github.com/GrayCodeAI/eyrie/credentials"
	"github.com/GrayCodeAI/eyrie/setup"
	"github.com/GrayCodeAI/hawk/internal/observability/logger"
)

func TestLiveOpenCodeGoMiniMaxM3FullHawkPath(t *testing.T) {
	if credentials.LookupSecret(context.Background(), "OPENCODEGO_API_KEY") == "" {
		t.Skip("OPENCODEGO_API_KEY not configured") // TODO: https://github.com/GrayCodeAI/hawk/issues/29
	}
	settings, err := loadEffectiveSettings()
	if err != nil {
		t.Fatal(err)
	}
	systemPrompt, err := buildSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	effectiveModel, effectiveProvider := effectiveModelAndProvider(settings)
	registry, err := defaultRegistry(settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("provider=%s model=%s tools=%d system_len=%d", effectiveProvider, effectiveModel, len(registry.EyrieTools()), len(systemPrompt))

	adapter := setup.ConfiguredDeploymentAdapters(eyriecfg.LoadProviderConfig(""))["opencodego"]
	t.Logf("adapter_type=%T", adapter.Provider)

	sess := newHawkSession(settings, effectiveProvider, effectiveModel, systemPrompt, registry)
	sess.SetLogger(logger.New(ioDiscard{}, logger.Info))
	if err := configureSession(sess, settings); err != nil {
		t.Fatal(err)
	}
	sess.AddUser("Hi")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ch, err := sess.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var content, thinking strings.Builder
	for ev := range ch {
		switch ev.Type {
		case "content":
			content.WriteString(ev.Content)
		case "thinking":
			thinking.WriteString(ev.Content)
			t.Logf("thinking chunk len=%d", len(ev.Content))
		case "error":
			t.Logf("error: %s", ev.Content)
		case "done":
			t.Log("done")
		}
	}
	t.Logf("content_len=%d thinking_len=%d", content.Len(), thinking.Len())
	if content.Len() == 0 {
		t.Fatalf("reasoning-only or empty: thinking_len=%d model=%s", thinking.Len(), effectiveModel)
	}
}

// ioDiscard is a minimal io.Writer for tests.
type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
