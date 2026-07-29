package accountportfolio

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedMigrationsUseDistinctSequenceNumbers(t *testing.T) {
	paths, err := fs.Glob(migrationFiles, "db/migrations/*.up.sql")
	if err != nil {
		t.Fatalf("list embedded migrations: %v", err)
	}

	bySequence := make(map[string]string, len(paths))
	for _, path := range paths {
		fileName := strings.TrimSuffix(strings.TrimPrefix(path, "db/migrations/"), ".up.sql")
		sequence, _, ok := strings.Cut(fileName, "_")
		if !ok || len(sequence) != 6 {
			t.Fatalf("migration %q must start with a six-digit sequence", path)
		}
		if previous, exists := bySequence[sequence]; exists {
			t.Fatalf("migration sequence %s is shared by %q and %q", sequence, previous, path)
		}
		bySequence[sequence] = path
	}
}
