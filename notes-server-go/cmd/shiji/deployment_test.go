package main

import (
	"path/filepath"
	"shiji/internal/api"
	"shiji/internal/store"
	"testing"
	"time"
)

func TestDeploymentCheckRejectsUnrelatedDatabase(t *testing.T) {
	db, err := store.Open("sqlite:///" + filepath.Join(t.TempDir(), "check.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close(db)
	if err = deploymentCheck(db); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE TABLE unrelated_business (id integer)").Error; err != nil {
		t.Fatal(err)
	}
	if err = deploymentCheck(db); err == nil {
		t.Fatal("unrelated database accepted")
	}
	if err = db.Exec("DROP TABLE unrelated_business").Error; err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err = deploymentCheck(db); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapState(t *testing.T) {
	db, err := store.Open("sqlite:///" + filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close(db)
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		got, err := bootstrapState(db)
		if err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	}
	check("empty")
	if _, err = api.IssueInvitation(db, "admin@example.test", "", true); err != nil {
		t.Fatal(err)
	}
	check("pending")
	if err = db.Model(&store.Invitation{}).Where("role = ?", "admin").Update("expires_at", store.Time{Time: time.Now().Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	check("empty")
	if err = db.Create(&store.User{ID: store.ID(), Email: "member@example.test", Role: "member"}).Error; err != nil {
		t.Fatal(err)
	}
	check("registered")
}
