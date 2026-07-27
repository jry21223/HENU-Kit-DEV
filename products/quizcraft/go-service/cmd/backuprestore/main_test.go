package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunRejectsLegacyTargetURLBeforeOpeningIt(t *testing.T) {
	t.Setenv("QUIZCRAFT_V2_DATABASE_URL", "postgres://quizcraft:secret@localhost/quizcraft_legacy?sslmode=disable")
	t.Setenv("QUIZCRAFT_RESTORE_ADMIN_URL", "postgres://quizcraft:secret@localhost/postgres?sslmode=disable")
	err := run(context.Background(), []string{"-backup-directory", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "quizcraft_v2") {
		t.Fatalf("legacy target URL error = %v", err)
	}
}
