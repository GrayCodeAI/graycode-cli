package cmd

import "testing"

func TestCloudGraphSyncCommandIsVisible(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"cloud", "graph", "sync"})
	if err != nil {
		t.Fatalf("find cloud graph sync: %v", err)
	}
	if command.Hidden {
		t.Fatal("cloud graph sync command should be visible")
	}
}

func TestCloudGraphSyncSupportsMissionGraphs(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"cloud", "graph", "sync"})
	if err != nil {
		t.Fatalf("find cloud graph sync: %v", err)
	}
	if command.Flags().Lookup("mission-dir") == nil {
		t.Fatal("cloud graph sync should expose --mission-dir")
	}
}
