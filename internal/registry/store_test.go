package registry

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreUpsertResolveRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	store := New(path)

	now := time.Now().UTC()
	entry := Entry{
		ID:          "id-1",
		DisplayName: "demo",
		Path:        "/tmp/demo",
		Status:      "installed",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.Upsert(entry); err != nil {
		t.Fatalf("Upsert error: %v", err)
	}

	byName, found, err := store.Resolve("demo")
	if err != nil {
		t.Fatalf("Resolve by name error: %v", err)
	}
	if !found || byName.ID != "id-1" {
		t.Fatalf("resolve by name mismatch: found=%v id=%q", found, byName.ID)
	}

	byID, found, err := store.Resolve("id-1")
	if err != nil {
		t.Fatalf("Resolve by id error: %v", err)
	}
	if !found || byID.DisplayName != "demo" {
		t.Fatalf("resolve by id mismatch: found=%v name=%q", found, byID.DisplayName)
	}

	if err := store.RemoveByID("id-1"); err != nil {
		t.Fatalf("RemoveByID error: %v", err)
	}
	entries, err := store.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(entries))
	}
}
