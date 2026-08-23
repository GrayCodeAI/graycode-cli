package skillcurator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeSkill(t *testing.T, dir, name, frontmatter string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o750); err != nil {
		t.Fatal(err)
	}
	body := ""
	if frontmatter != "" {
		body = "---\n" + frontmatter + "---\n\n# " + name + "\n"
	}
	if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestCurator(t *testing.T, idleDays int) (*Curator, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		SkillsDir: dir, IdleDaysBeforeArchive: idleDays, IntervalHours: 1,
		StateFile: filepath.Join(dir, ".curator_state.json"),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c, dir
}

func TestRecordUseAndListStatus(t *testing.T) {
	c, dir := newTestCurator(t, 30)
	makeSkill(t, dir, "go-helper", "category: devtools\n")
	c.RecordUse("go-helper")
	skills, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "go-helper" {
		t.Fatalf("skills = %+v", skills)
	}
	if skills[0].Category != "devtools" {
		t.Fatalf("category = %q", skills[0].Category)
	}
	if skills[0].UseCount != 1 {
		t.Fatalf("use count = %d", skills[0].UseCount)
	}
	if skills[0].Status != StatusActive {
		t.Fatalf("status = %q", skills[0].Status)
	}
}

func TestPinBypassesArchive(t *testing.T) {
	c, dir := newTestCurator(t, 0) // idle threshold 0 -> anything old archives
	makeSkill(t, dir, "keep", "")
	c.RecordUse("keep")
	c.state.Usage["keep"].LastUsed = time.Now().AddDate(0, 0, -10) // cold

	if err := c.Pin("keep"); err != nil {
		t.Fatal(err)
	}
	archived, err := c.MaybeRun(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("pinned skill was archived: %v", archived)
	}
	// It must still be present and pinned.
	skills, _ := c.List()
	if len(skills) != 1 || skills[0].Status != StatusPinned {
		t.Fatalf("skills = %+v", skills)
	}
}

func TestArchiveMovesToRecoverableDotArchive(t *testing.T) {
	c, dir := newTestCurator(t, 30)
	makeSkill(t, dir, "old-skill", "")
	if err := c.Archive("old-skill", "obsolete"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old-skill")); !os.IsNotExist(err) {
		t.Fatal("original dir should be gone")
	}
	if _, err := os.Stat(filepath.Join(dir, ".archive", "old-skill")); err != nil {
		t.Fatalf("archived copy missing: %v", err)
	}
	// List reports it as archived (recoverable), never deleted.
	skills, _ := c.List()
	found := false
	for _, s := range skills {
		if s.Name == "old-skill" && s.Status == StatusArchived {
			found = true
		}
	}
	if !found {
		t.Fatalf("archived skill not reported: %+v", skills)
	}
}

func TestArchiveRefusesPinned(t *testing.T) {
	c, dir := newTestCurator(t, 30)
	makeSkill(t, dir, "keep", "")
	_ = c.Pin("keep")
	if err := c.Archive("keep", ""); err == nil {
		t.Fatal("archive must refuse a pinned skill")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep")); err != nil {
		t.Fatal("pinned skill must remain in place")
	}
}

func TestMaybeRunArchivesColdUsedSkill(t *testing.T) {
	c, dir := newTestCurator(t, 30)
	makeSkill(t, dir, "stale", "")
	c.RecordUse("stale")
	// Force its last-used to long ago.
	c.mu.Lock()
	c.state.Usage["stale"].LastUsed = time.Now().AddDate(0, 0, -60)
	c.mu.Unlock()

	archived, err := c.MaybeRun(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0] != "stale" {
		t.Fatalf("archived = %v", archived)
	}
	// Never-used skills are left alone (conservative).
	makeSkill(t, dir, "brand-new", "")
	archived2, err := c.MaybeRun(time.Now().Add(2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(archived2) != 0 {
		t.Fatalf("never-used skill auto-archived: %v", archived2)
	}
}

func TestMaybeRunRespectsInterval(t *testing.T) {
	c, dir := newTestCurator(t, 30)
	makeSkill(t, dir, "s", "")
	c.RecordUse("s")
	c.mu.Lock()
	c.state.Usage["s"].LastUsed = time.Now().AddDate(0, 0, -60)
	c.mu.Unlock()

	now := time.Now()
	if _, err := c.MaybeRun(now); err != nil {
		t.Fatal(err)
	}
	// Running again immediately (within interval) does nothing.
	c.mu.Lock()
	c.state.Usage["s"].LastUsed = time.Now().AddDate(0, 0, -60)
	c.mu.Unlock()
	archived, err := c.MaybeRun(now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("review ran within interval: %v", archived)
	}
}

func TestStateRoundTrip(t *testing.T) {
	c, dir := newTestCurator(t, 30)
	makeSkill(t, dir, "x", "")
	c.RecordUse("x")
	_ = c.Pin("x")

	c2, err := New(Config{SkillsDir: dir, StateFile: filepath.Join(dir, ".curator_state.json")})
	if err != nil {
		t.Fatal(err)
	}
	skills, _ := c2.List()
	if len(skills) != 1 || skills[0].Status != StatusPinned {
		t.Fatalf("pin not restored: %+v", skills)
	}
	if skills[0].UseCount != 1 {
		t.Fatalf("usage not restored: %+v", skills[0])
	}
}
