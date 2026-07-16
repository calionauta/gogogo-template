package db

import (
	"os"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// newSeedTestApp boots a PocketBase app on a temp dir with the same
// ncruces/go-sqlite3 driver as production and creates a `users` auth
// collection, mirroring production where ensureTodosCollection links the
// owner relation to `users`.
//
// NOTE: the DSN MUST use the "file:" URI prefix. ncruces only parses
// ?_pragma= query params when the DSN is a file: URI; without the prefix
// it treats the whole string (path + "?_pragma=...") as a filename and
// SILENTLY DROPS every pragma (journal_mode stays DELETE, foreign_keys
// OFF). This matches production's DBConnect in db/pocketbase.go.
func newSeedTestApp(t *testing.T, tmpDir string) *pocketbase.PocketBase {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: tmpDir,
		DBConnect: func(dbPath string) (*dbx.DB, error) {
			pragmas := "?_pragma=busy_timeout(10000)" +
				"&_pragma=journal_mode(WAL)" +
				"&_pragma=foreign_keys(ON)"
			return dbx.Open("sqlite3", "file:"+dbPath+pragmas)
		},
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	return app
}

// TestEnsureTodosCollectionAddsOwnerRelation guards the requirement that
// todos are tenant-scoped: the production seed must create the `todos`
// collection WITH an `owner` relation to `users`. Without it,
// rec.Set("owner", user) is silently dropped and todos are not associated
// with any user — the exact bug reported against the deployed app.
func TestEnsureTodosCollectionAddsOwnerRelation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "seed-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	app := newSeedTestApp(t, tmpDir)
	if err = ensureTodosCollection(app, true); err != nil {
		t.Fatalf("ensureTodosCollection: %v", err)
	}

	col, err := app.FindCollectionByNameOrId("todos")
	if err != nil {
		t.Fatalf("find todos: %v", err)
	}
	f := col.Fields.GetByName("owner")
	if f == nil {
		t.Fatal("todos collection missing owner field")
	}
	rel, ok := f.(*core.RelationField)
	if !ok {
		t.Fatalf("owner field is %T, want *core.RelationField", f)
	}
	if rel.CollectionId == "" {
		t.Fatal("owner relation has empty CollectionId")
	}
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	if rel.CollectionId != usersCol.Id {
		t.Fatalf("owner relation points to %q, want users (%q)", rel.CollectionId, usersCol.Id)
	}

	// Idempotent re-run must not error and must keep the owner field
	// (covers backfilling collections created by older seeds).
	if err = ensureTodosCollection(app, true); err != nil {
		t.Fatalf("ensureTodosCollection (second run): %v", err)
	}
	col2, err := app.FindCollectionByNameOrId("todos")
	if err != nil {
		t.Fatalf("find todos (second run): %v", err)
	}
	if col2.Fields.GetByName("owner") == nil {
		t.Fatal("owner field lost after idempotent re-run")
	}
}
