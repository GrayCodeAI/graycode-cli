package flags

import (
	"os"
	"testing"
)

func TestSpawnV2DefaultOff(t *testing.T) {
	ResetForTest()
	t.Setenv(EnvSpawnV2, "")
	if SpawnV2() {
		t.Fatal("SpawnV2 default should be false until PACK-02 ships")
	}
}

func TestEnvParsing(t *testing.T) {
	ResetForTest()
	cases := []struct {
		val  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"YES", true},
		{"0", false},
		{"false", false},
		{"off", false},
	}
	for _, tc := range cases {
		t.Setenv(EnvFolderTrust, tc.val)
		if got := FolderTrust(); got != tc.want {
			t.Errorf("FolderTrust(%q)=%v want %v", tc.val, got, tc.want)
		}
	}
}

func TestFolderTrustDefaultOn(t *testing.T) {
	ResetForTest()
	t.Setenv(EnvFolderTrust, "")
	// Unset so LookupEnv fails
	_ = os.Unsetenv(EnvFolderTrust)
	if !FolderTrust() {
		t.Fatal("FolderTrust default should be true after PACK-03")
	}
}

func TestSetForTestOverridesEnv(t *testing.T) {
	ResetForTest()
	t.Setenv(EnvMarketplace, "1")
	SetForTest(EnvMarketplace, false)
	if Marketplace() {
		t.Fatal("SetForTest should override env")
	}
}
