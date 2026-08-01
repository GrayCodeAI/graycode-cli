package plugin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/flags"
	"github.com/GrayCodeAI/hawk/internal/testutil"
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
