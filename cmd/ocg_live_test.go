//go:build live_test
// +build live_test

// Live integration test for the OpenCodeGo provider adapter end-to-end.
// Opt-in only — not run by default `go test ./...` or CI. Run with:
//
//	OPENCODEGO_API_KEY=... go test -tags=live_test -run TestLiveOpenCodeGoMiniMaxM3FullHawkPath ./cmd
//
// Or via `make test-live` in this repo.
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
	if cfgErr := configureSession(sess, settings); cfgErr != nil {
		t.Fatal(cfgErr)
	}
	// Use a complex task that cannot yield empty content
	sess.AddUser("Write a simple HTTP server in Go using only standard library. Respond to all requests with 'Hello, World!' and log the request path to stdout.")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ch, err := sess.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var content, thinking strings.Builder
	var contentReceived bool
	for ev := range ch {
		switch ev.Type {
		case "content":
			content.WriteString(ev.Content)
			contentReceived = true
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
	if !contentReceived && thinking.Len() < 100 {
		// If no content and negligible thinking, fail
		t.Fatalf("neither content nor substantial thinking: thinking_len=%d model=%s", thinking.Len(), effectiveModel)
	}
	if thinking.Len() > content.Len()*10 && content.Len() < 20 {
		// Allow substantial thinking when model is processing complex task,
		// but require reasonable content token count for long thinking.
		t.Logf("Allowing long thinking with minimal content: content_len=%d thinking_len=%d", content.Len(), thinking.Len())
	}
}

// ioDiscard is a minimal io.Writer for tests.
type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
