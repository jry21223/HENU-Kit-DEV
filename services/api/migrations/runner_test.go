package migrations

import (
	"strings"
	"testing"
)

func TestEveryVersionedMigrationHasRollback(t *testing.T) {
	up, err := migrationNames(".up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationNames(".down.sql")
	if err != nil {
		t.Fatal(err)
	}
	available := map[string]bool{}
	for _, name := range down {
		available[strings.TrimSuffix(name, ".down.sql")] = true
	}
	if len(up) == 0 {
		t.Fatal("expected versioned migrations")
	}
	for _, name := range up {
		version := strings.TrimSuffix(name, ".up.sql")
		if !available[version] {
			t.Fatalf("migration %s has no down file", version)
		}
	}
}
