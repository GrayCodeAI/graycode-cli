package plugin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/flags"
	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func TestMarketplaceFind(t *testing.T) {
	flags.ResetForTest()
	t.Cleanup(flags.ResetForTest)
	flags.SetForTest(flags.EnvMarketplace, true)

	idx := MarketplaceIndex{
		Version: 1,
		Plugins: []MarketplaceEntry{
			{Name: "cool-plugin", Repo: "org/cool-plugin", Description: "cool", Version: "1.0.0"},
		},
	}
	srv := testutil.NewLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(idx)
	}))

	mc := &MarketplaceClient{
		Sources:  []MarketplaceSource{{Name: "test", URL: srv.URL}},
		CacheDir: t.TempDir(),
		client:   srv.Client(),
	}
	e, err := mc.Find("cool-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if e.Repo != "org/cool-plugin" {
		t.Fatalf("%+v", e)
	}
	if _, err := mc.Find("missing"); err == nil {
		t.Fatal("expected not found")
	}
}

func TestMarketplaceInstallDisabled(t *testing.T) {
	flags.ResetForTest()
	t.Cleanup(flags.ResetForTest)
	flags.SetForTest(flags.EnvMarketplace, false)

	mc := NewMarketplaceClient()
	_, err := mc.Install(MarketplaceEntry{Name: "x", Repo: "a/b"})
	if err == nil {
		t.Fatal("expected disabled error")
	}
}

func TestMarketplaceInstallRejectsSCPStyleURL(t *testing.T) {
	flags.ResetForTest()
	t.Cleanup(flags.ResetForTest)
	flags.SetForTest(flags.EnvMarketplace, true)

	mc := NewMarketplaceClient()
	_, err := mc.Install(MarketplaceEntry{Name: "x", Repo: "git@github.com:user/repo.git"})
	if err == nil {
		t.Fatal("expected scp-style URL rejection")
	}
	if !strings.Contains(err.Error(), "scp-style") {
		t.Errorf("error should mention scp-style, got: %v", err)
	}
}

func TestNoPhantomDefaultMarketplaceSource(t *testing.T) {
	for _, src := range DefaultMarketplaceSources() {
		if strings.Contains(src.URL, "plugins-registry.json") {
			t.Fatalf("default source %q points at plugins-registry.json, which nothing generates", src.Name)
		}
	}
}

func TestFetchAllWithNoSourcesReturnsEmptyNotError(t *testing.T) {
	mc := &MarketplaceClient{Sources: nil, CacheDir: t.TempDir()}
	entries, err := mc.FetchAll()
	if err != nil {
		t.Fatalf("FetchAll with no sources returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(entries))
	}
}
