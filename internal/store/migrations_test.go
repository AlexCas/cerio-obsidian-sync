package store

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrations_CreatesAllTables(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	expectedTables := []string{"files", "revisions", "client_revisions", "api_keys", "_migrations"}
	for _, table := range expectedTables {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("checking table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s not found (count=%d)", table, count)
		}
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db := openTestDB(t)

	// Run migrations twice.
	if err := RunMigrations(db); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	// Verify _migrations recorded exactly once per version.
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE version = 1`).Scan(&count)
	if err != nil {
		t.Fatalf("checking migration records: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record for migration v1, got %d", count)
	}
}

func TestRunMigrations_CreatesIndexes(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	expectedIndexes := []string{"idx_files_vault_hash", "idx_files_vault_deleted", "idx_revisions_vault_id"}
	for _, idx := range expectedIndexes {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&count)
		if err != nil {
			t.Fatalf("checking index %s: %v", idx, err)
		}
		if count != 1 {
			t.Errorf("index %s not found (count=%d)", idx, count)
		}
	}
}
