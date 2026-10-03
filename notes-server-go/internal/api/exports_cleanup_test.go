package api

import (
	"os"
	"path/filepath"
	"shiji/internal/store"
	"testing"
	"time"
)

func TestCleanupPreservesLiveBuildingAndFreshOrphans(t *testing.T) {
	f := newNotesFixture(t)
	a := f.app
	a.Settings.ExportsDir = t.TempDir()
	var user store.User
	if e := a.DB.First(&user).Error; e != nil {
		t.Fatal(e)
	}
	expired := store.ID()
	building := store.ID()
	oldOrphan := store.ID()
	freshOrphan := store.ID()
	past := time.Now().Add(-2 * time.Hour)
	for _, row := range []store.Export{{ID: expired, UserID: user.ID, Ready: true, Manifest: store.Map{}, CreatedAt: store.Now(), ExpiresAt: store.Time{Time: past}}, {ID: building, UserID: user.ID, Ready: false, Manifest: store.Map{}, CreatedAt: store.Now(), ExpiresAt: store.Time{Time: time.Now().Add(time.Hour)}}} {
		if e := a.DB.Create(&row).Error; e != nil {
			t.Fatal(e)
		}
	}
	names := []string{expired + ".zip", building + ".part", oldOrphan + ".part", freshOrphan + ".zip", "keep.txt"}
	for _, name := range names {
		if e := os.WriteFile(filepath.Join(a.Settings.ExportsDir, name), []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{oldOrphan + ".part", "keep.txt"} {
		os.Chtimes(filepath.Join(a.Settings.ExportsDir, name), past, past)
	}
	if e := a.CleanupExports(); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{expired + ".zip", oldOrphan + ".part"} {
		if _, e := os.Stat(filepath.Join(a.Settings.ExportsDir, name)); !os.IsNotExist(e) {
			t.Fatal("stale file remains", name, e)
		}
	}
	for _, name := range []string{building + ".part", freshOrphan + ".zip", "keep.txt"} {
		if _, e := os.Stat(filepath.Join(a.Settings.ExportsDir, name)); e != nil {
			t.Fatal("live/unknown file removed", name, e)
		}
	}
}
