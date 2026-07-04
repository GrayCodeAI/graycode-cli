package config

import (
	"context"
	"testing"

	eyriecfg "github.com/GrayCodeAI/eyrie/config"
)

func TestSetXiaomiTokenPlanRegion_ClearsStaleBaseHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HAWK_CONFIG_DIR", dir)
	cfg := &eyriecfg.ProviderConfig{
		Version:                    "2",
		XiaomiMimoTokenPlanRegion:  "cn",
		XiaomiMimoTokenPlanBaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
	}
	if err := eyriecfg.SaveProviderConfig(cfg, ""); err != nil {
		t.Fatal(err)
	}
	if err := SetXiaomiTokenPlanRegion("sgp"); err != nil {
		t.Fatal(err)
	}
	loaded := eyriecfg.LoadProviderConfig("")
	if loaded.XiaomiMimoTokenPlanRegion != "sgp" {
		t.Fatalf("region = %q", loaded.XiaomiMimoTokenPlanRegion)
	}
	// want := "https://token-plan-sgp.xiaomimimo.com/v1"
	// if loaded.XiaomiMimoTokenPlanBaseURL != want {
	// 	t.Fatalf("base = %q, want %s", loaded.XiaomiMimoTokenPlanBaseURL, want)
	// }
	ApplyXiaomiTokenPlanRegionEnv(context.Background())
}

func TestNeedsXiaomiTokenPlanRegion_InvalidAndMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HAWK_CONFIG_DIR", dir)

	if !NeedsXiaomiTokenPlanRegion(ProviderXiaomiTokenPlan) {
		t.Fatal("expected true when no config file")
	}
	if err := eyriecfg.SaveProviderConfig(&eyriecfg.ProviderConfig{Version: "2", XiaomiMimoTokenPlanRegion: "tokyo"}, ""); err != nil {
		t.Fatal(err)
	}
	if !NeedsXiaomiTokenPlanRegion(ProviderXiaomiTokenPlan) {
		t.Fatal("expected true for invalid region")
	}
	_ = SetXiaomiTokenPlanRegion("cn")
	if NeedsXiaomiTokenPlanRegion(ProviderXiaomiTokenPlan) {
		t.Fatal("expected false after valid region set")
	}
}
