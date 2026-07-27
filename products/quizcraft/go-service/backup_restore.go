package quizcraft

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var postgresIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// BackupRestoreDrillOptions defines one isolated backup/restore rehearsal for
// quizcraft_v2. RestoreAdminURL must be an administrator connection allowed to
// create and drop a temporary database; it is never used as a production
// traffic endpoint.
type BackupRestoreDrillOptions struct {
	DatabaseURL      string
	RestoreAdminURL  string
	BackupDirectory  string
	RequiredTables   []string
	DatabaseNameHint string
}

// BackupRestoreDrillReport is a durable, operator-readable result. The backup
// is intentionally retained at BackupPath for the caller's audit record; this
// function removes only its generated temporary restore database.
type BackupRestoreDrillReport struct {
	BackupPath                   string                                    `json:"backup_path"`
	BackupSHA256                 string                                    `json:"backup_sha256"`
	SourceDatabaseIdentity       string                                    `json:"source_database_identity"`
	RestoreAdminDatabaseIdentity string                                    `json:"restore_admin_database_identity"`
	RestoredDatabase             string                                    `json:"restored_database"`
	RestoredDatabaseIdentity     string                                    `json:"restored_database_identity"`
	DumpCommand                  []string                                  `json:"dump_command"`
	RestoreCommand               []string                                  `json:"restore_command"`
	VerifiedTables               []string                                  `json:"verified_tables"`
	TableCounts                  map[string]int64                          `json:"table_counts"`
	SourceTableVerification      map[string]BackupRestoreTableVerification `json:"source_table_verification"`
	RestoredTableVerification    map[string]BackupRestoreTableVerification `json:"restored_table_verification"`
	StartedAt                    time.Time                                 `json:"started_at"`
	CompletedAt                  time.Time                                 `json:"completed_at"`
	Duration                     time.Duration                             `json:"duration"`
}

// BackupRestoreTableVerification is a credential-free, deterministic summary
// of one required table at the backup boundary. ContentSHA256 is calculated
// from sorted canonical JSONB rows and is compared with the isolated restore.
type BackupRestoreTableVerification struct {
	RowCount      int64  `json:"row_count"`
	ContentSHA256 string `json:"content_sha256"`
}

// RunBackupRestoreDrill produces a custom PostgreSQL dump, verifies its
// checksum, restores it into a newly-created isolated database, and checks the
// required QuizCraft tables. It never mutates the source database.
func RunBackupRestoreDrill(ctx context.Context, options BackupRestoreDrillOptions) (BackupRestoreDrillReport, error) {
	if strings.TrimSpace(options.DatabaseURL) == "" || strings.TrimSpace(options.RestoreAdminURL) == "" {
		return BackupRestoreDrillReport{}, errors.New("source and restore-admin database URLs are required")
	}
	if strings.TrimSpace(options.BackupDirectory) == "" {
		return BackupRestoreDrillReport{}, errors.New("backup directory is required")
	}
	requiredTables := options.RequiredTables
	if len(requiredTables) == 0 {
		requiredTables = []string{"quizcraft_banks", "quizcraft_questions", "quizcraft_migration_runs"}
	}
	for _, table := range requiredTables {
		if !postgresIdentifier.MatchString(table) {
			return BackupRestoreDrillReport{}, fmt.Errorf("unsafe required table name %q", table)
		}
	}
	startedAt := time.Now().UTC()
	sourceConfig, err := pgx.ParseConfig(options.DatabaseURL)
	if err != nil {
		return BackupRestoreDrillReport{}, errors.New("invalid quizcraft_v2 database URL")
	}
	if err := RequireQuizcraftV2DatabaseURL(options.DatabaseURL); err != nil {
		return BackupRestoreDrillReport{}, err
	}
	adminConfig, err := pgx.ParseConfig(options.RestoreAdminURL)
	if err != nil {
		return BackupRestoreDrillReport{}, errors.New("invalid restore-admin database URL")
	}
	if sourceConfig.Database == "" || adminConfig.Database == "" {
		return BackupRestoreDrillReport{}, errors.New("source and restore-admin database names are required")
	}
	source, err := pgx.ConnectConfig(ctx, sourceConfig)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	defer func() { _ = source.Close(context.Background()) }()
	if err := requireQuizcraftV2Target(ctx, source); err != nil {
		return BackupRestoreDrillReport{}, err
	}
	sourceIdentity, err := databaseIdentity(ctx, source)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	sourceVerification, err := verifyRequiredTableContent(ctx, source, requiredTables, "source")
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	if err := os.MkdirAll(options.BackupDirectory, 0o700); err != nil {
		return BackupRestoreDrillReport{}, err
	}

	token, err := randomRestoreToken()
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	hint := strings.TrimSpace(options.DatabaseNameHint)
	if hint == "" {
		hint = "quizcraft_v2"
	}
	if !postgresIdentifier.MatchString(hint) {
		return BackupRestoreDrillReport{}, fmt.Errorf("unsafe database name hint %q", hint)
	}
	restoredDatabase := hint + "_restore_" + token
	if len(restoredDatabase) > 63 {
		return BackupRestoreDrillReport{}, errors.New("temporary restore database name is too long")
	}
	backupPath := filepath.Join(options.BackupDirectory, "quizcraft-v2-"+startedAt.Format("20060102T150405Z")+"-"+token+".dump")
	temporaryBackup := backupPath + ".tmp"
	if file, err := os.OpenFile(temporaryBackup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); err != nil {
		return BackupRestoreDrillReport{}, err
	} else if err := file.Close(); err != nil {
		return BackupRestoreDrillReport{}, err
	}
	defer func() { _ = os.Remove(temporaryBackup) }()
	dumpCommand := []string{"pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file=" + temporaryBackup}
	if err := runPostgresCommand(ctx, sourceConfig, dumpCommand[0], "--format=custom", "--no-owner", "--no-privileges", "--file="+temporaryBackup); err != nil {
		return BackupRestoreDrillReport{}, err
	}
	checksum, err := fileSHA256(temporaryBackup)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	if err := os.Rename(temporaryBackup, backupPath); err != nil {
		return BackupRestoreDrillReport{}, err
	}

	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	adminIdentity, err := databaseIdentity(ctx, admin)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{restoredDatabase}.Sanitize()); err != nil {
		return BackupRestoreDrillReport{}, fmt.Errorf("create isolated restore database: %w", err)
	}
	createdRestoreDatabase := true
	cleanup := func() error {
		if !createdRestoreDatabase {
			return nil
		}
		if _, err := admin.Exec(context.Background(), `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, restoredDatabase); err != nil {
			return err
		}
		if _, err := admin.Exec(context.Background(), `DROP DATABASE IF EXISTS `+pgx.Identifier{restoredDatabase}.Sanitize()); err != nil {
			return err
		}
		createdRestoreDatabase = false
		return nil
	}
	defer func() { _ = cleanup() }()

	restoreConfig := adminConfig.Copy()
	restoreConfig.Database = restoredDatabase
	restoreCommand := []string{"pg_restore", "--exit-on-error", "--no-owner", "--no-privileges", "--dbname=" + restoredDatabase, backupPath}
	if err := runPostgresCommand(ctx, restoreConfig, restoreCommand[0], restoreCommand[1:]...); err != nil {
		return BackupRestoreDrillReport{}, err
	}
	restored, err := pgx.ConnectConfig(ctx, restoreConfig)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	restoredIdentity, err := databaseIdentity(ctx, restored)
	if err != nil {
		_ = restored.Close(context.Background())
		return BackupRestoreDrillReport{}, err
	}
	restoredVerification, err := verifyRequiredTableContent(ctx, restored, requiredTables, "restored")
	if err == nil {
		err = requireEquivalentTableContent(requiredTables, sourceVerification, restoredVerification)
	}
	closeErr := restored.Close(ctx)
	if err != nil {
		return BackupRestoreDrillReport{}, err
	}
	if closeErr != nil {
		return BackupRestoreDrillReport{}, closeErr
	}
	if err := cleanup(); err != nil {
		return BackupRestoreDrillReport{}, fmt.Errorf("remove isolated restore database: %w", err)
	}
	tableCounts := make(map[string]int64, len(restoredVerification))
	for table, verification := range restoredVerification {
		tableCounts[table] = verification.RowCount
	}
	completedAt := time.Now().UTC()
	return BackupRestoreDrillReport{
		BackupPath:                   backupPath,
		BackupSHA256:                 checksum,
		SourceDatabaseIdentity:       sourceIdentity,
		RestoreAdminDatabaseIdentity: adminIdentity,
		RestoredDatabase:             restoredDatabase,
		RestoredDatabaseIdentity:     restoredIdentity,
		DumpCommand:                  dumpCommand,
		RestoreCommand:               restoreCommand,
		VerifiedTables:               append([]string(nil), requiredTables...),
		TableCounts:                  tableCounts,
		SourceTableVerification:      sourceVerification,
		RestoredTableVerification:    restoredVerification,
		StartedAt:                    startedAt,
		CompletedAt:                  completedAt,
		Duration:                     completedAt.Sub(startedAt),
	}, nil
}

func runPostgresCommand(ctx context.Context, config *pgx.ConnConfig, name string, arguments ...string) error {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = postgresEnvironment(config)
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		// Client output can contain a host or role name. It must not be carried
		// into the returned error or issue evidence because those reports are
		// routinely pasted into tickets.
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func postgresEnvironment(config *pgx.ConnConfig) []string {
	environment := os.Environ()
	set := func(key, value string) {
		prefix := key + "="
		for index := range environment {
			if strings.HasPrefix(environment[index], prefix) {
				environment[index] = prefix + value
				return
			}
		}
		environment = append(environment, prefix+value)
	}
	set("PGHOST", config.Host)
	set("PGPORT", strconv.Itoa(int(config.Port)))
	set("PGUSER", config.User)
	set("PGPASSWORD", config.Password)
	set("PGDATABASE", config.Database)
	if config.TLSConfig == nil {
		set("PGSSLMODE", "disable")
	} else {
		set("PGSSLMODE", "require")
	}
	return environment
}

func verifyRequiredTableContent(ctx context.Context, database *pgx.Conn, tables []string, role string) (map[string]BackupRestoreTableVerification, error) {
	verifications := make(map[string]BackupRestoreTableVerification, len(tables))
	for _, table := range tables {
		var exists *string
		if err := database.QueryRow(ctx, `SELECT to_regclass($1)`, "public."+table).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == nil {
			return nil, fmt.Errorf("%s database is missing required table %s", role, table)
		}
		rows, err := database.Query(ctx, `SELECT to_jsonb(source_row)::text FROM `+pgx.Identifier{table}.Sanitize()+` AS source_row ORDER BY to_jsonb(source_row)::text`)
		if err != nil {
			return nil, err
		}
		hasher := sha256.New()
		var rowCount int64
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				rows.Close()
				return nil, err
			}
			if _, err := io.WriteString(hasher, row+"\n"); err != nil {
				rows.Close()
				return nil, err
			}
			rowCount++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		verifications[table] = BackupRestoreTableVerification{RowCount: rowCount, ContentSHA256: hex.EncodeToString(hasher.Sum(nil))}
	}
	return verifications, nil
}

func requireEquivalentTableContent(tables []string, source, restored map[string]BackupRestoreTableVerification) error {
	for _, table := range tables {
		sourceValue, sourceOK := source[table]
		restoredValue, restoredOK := restored[table]
		if !sourceOK || !restoredOK || sourceValue != restoredValue {
			return fmt.Errorf("restored table %s does not match the source row count and content checksum", table)
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	stream, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = stream.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, stream); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func randomRestoreToken() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
