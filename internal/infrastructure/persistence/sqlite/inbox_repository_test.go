package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

func TestInboxRepositoryCreateListUpdateDelete(t *testing.T) {
	t.Parallel()

	db, err := New(filepath.Join(t.TempDir(), "glog.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()
	if err := db.MigrateFS(EmbeddedMigrations, "migrations"); err != nil {
		t.Fatalf("MigrateFS() error = %v", err)
	}
	repo := NewInboxRepository(db)

	item := entities.NewInboxItem("Investigate failing build")
	item.Body = "Check the latest error logs."
	item.Kind = entities.InboxKindBug
	item.Priority = entities.InboxPriorityHigh
	item.Repo = "dotcommander/glog"
	item.SessionID = "session-1"
	item.Tags = entities.HostTags{"ci", "agent"}

	if err := repo.Create(item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if item.ID == 0 {
		t.Fatal("Create() did not assign ID")
	}

	items, total, err := repo.List(ports.InboxFilters{Status: "open", Repo: "dotcommander/glog"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("List() total,len = %d,%d; want 1,1", total, len(items))
	}
	if items[0].Title != item.Title || items[0].Tags[0] != "ci" {
		t.Fatalf("List() returned unexpected item: %#v", items[0])
	}

	item.Status = entities.InboxStatusDone
	item.ClaimedBy = "codex"
	if err := repo.Update(item); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, err := repo.FindByID(item.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if updated.Status != entities.InboxStatusDone || updated.ClaimedBy != "codex" {
		t.Fatalf("FindByID() returned status=%q claimed_by=%q", updated.Status, updated.ClaimedBy)
	}

	if err := repo.Delete(item.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repo.FindByID(item.ID); err != ports.ErrNotFound {
		t.Fatalf("FindByID() after delete error = %v; want ErrNotFound", err)
	}
}
