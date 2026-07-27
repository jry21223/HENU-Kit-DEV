package quizcraft

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var versionedMigrationFile = regexp.MustCompile(`^([0-9]{6})_[a-z0-9_]+\.up\.sql$`)

// 000009 is the first artifact that introduced the migration-history table.
// Databases created by the preceding released schema must be adopted once,
// rather than having 000001–000008 replayed onto existing objects.
const firstTrackedSchemaMigrationVersion = "000009"

// preHistoryBaselineSchemaFingerprint is the PostgreSQL 16 catalog digest of
// the released 000001-000008 QuizCraft schema. Relation names alone are not
// enough to prove that a manual historical baseline is safe to adopt: the
// digest also binds every column, constraint, index, trigger, and trigger
// function used by that baseline.
const preHistoryBaselineSchemaFingerprint = "6b9a5891613f6010d96d448c850a25f87db7d5f3e6d01403d29ba297425b0010"

var preHistoryBaselineTables = []string{
	"quizcraft_bank_version_questions",
	"quizcraft_bank_versions",
	"quizcraft_banks",
	"quizcraft_favorites",
	"quizcraft_feedback_inbox_deliveries",
	"quizcraft_feedback_inbox_outbox",
	"quizcraft_feedbacks",
	"quizcraft_idempotency_results",
	"quizcraft_learning_state",
	"quizcraft_practice_sessions",
	"quizcraft_practice_session_questions",
	"quizcraft_practice_session_claims",
	"quizcraft_practice_attempts",
	"quizcraft_question_stats",
	"quizcraft_question_versions",
	"quizcraft_questions",
	"quizcraft_ranking_profiles",
	"quizcraft_ranking_settlement_events",
	"quizcraft_service_nonces",
	"quizcraft_shadow_comparisons",
	"quizcraft_shadow_gate_reports",
	"quizcraft_workshop_audit_events",
	"quizcraft_workshop_version_states",
	"quizcraft_migration_runs",
	"quizcraft_migration_exceptions",
	"quizcraft_migration_exception_resolutions",
	"quizcraft_migration_event_receipts",
	"quizcraft_legacy_feedback_archives",
	"quizcraft_legacy_feedback_state_events",
	"quizcraft_legacy_ranking_snapshots",
}

var preHistoryBaselineFunctionNames = []string{
	"quizcraft_guard_bank_version_update",
	"quizcraft_guard_membership_insert",
	"quizcraft_reject_immutable_mutation",
}

// SchemaMigration records one immutable SQL source accepted by the target
// database. The source checksum makes a changed migration fail closed instead
// of being silently reinterpreted on a later recovery run.
type SchemaMigration struct {
	Version string `json:"version"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
}

// SchemaMigrationReport is the public result of applying the versioned
// quizcraft_v2 schema. A repeated run must only populate Skipped.
type SchemaMigrationReport struct {
	Applied []SchemaMigration `json:"applied"`
	Adopted []SchemaMigration `json:"adopted"`
	Skipped []SchemaMigration `json:"skipped"`
}

type migrationSource struct {
	SchemaMigration
	SQL string
}

// ApplyVersionedMigrations applies the ordered .up.sql files in directory to
// one target database. It records the exact file checksum in the target, so it
// is safe to repeat after a process interruption and refuses changed sources.
// The target is intentionally supplied by the caller; it never reads a legacy
// connection string and therefore cannot move Portal traffic or V2 practice
// facts.
func ApplyVersionedMigrations(ctx context.Context, database *pgxpool.Pool, directory string) (SchemaMigrationReport, error) {
	if database == nil {
		return SchemaMigrationReport{}, errors.New("quizcraft_v2 target database is required")
	}
	sources, err := readMigrationSources(directory)
	if err != nil {
		return SchemaMigrationReport{}, err
	}
	connection, err := database.Acquire(ctx)
	if err != nil {
		return SchemaMigrationReport{}, err
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended('quizcraft-v2-schema-migrations', 0))`); err != nil {
		return SchemaMigrationReport{}, err
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended('quizcraft-v2-schema-migrations', 0))`)
	}()
	var targetSchema string
	if err := connection.QueryRow(ctx, `SELECT current_schema()`).Scan(&targetSchema); err != nil {
		return SchemaMigrationReport{}, err
	}
	if targetSchema != "public" {
		return SchemaMigrationReport{}, errors.New("quizcraft_v2 schema migrations require the public PostgreSQL schema")
	}
	var migrationHistoryExists bool
	if err := connection.QueryRow(ctx, `SELECT to_regclass('public.quizcraft_schema_migrations') IS NOT NULL`).Scan(&migrationHistoryExists); err != nil {
		return SchemaMigrationReport{}, err
	}
	completeBaseline := false
	if !migrationHistoryExists {
		anyBaseline, complete, err := inspectPreHistoryBaseline(ctx, connection)
		if err != nil {
			return SchemaMigrationReport{}, err
		}
		if anyBaseline && !complete {
			return SchemaMigrationReport{}, errors.New("refusing to adopt a partial pre-history QuizCraft schema; restore a known baseline or reconcile it before running migrations")
		}
		completeBaseline = complete
	}
	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS quizcraft_schema_migrations (
        version text PRIMARY KEY CHECK (version ~ '^[0-9]{6}$'),
        filename text NOT NULL CHECK (filename <> ''),
        source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
        applied_at timestamptz NOT NULL DEFAULT now()
    )`); err != nil {
		return SchemaMigrationReport{}, err
	}

	rows, err := connection.Query(ctx, `SELECT version,filename,source_sha256 FROM quizcraft_schema_migrations ORDER BY version`)
	if err != nil {
		return SchemaMigrationReport{}, err
	}
	stored := map[string]SchemaMigration{}
	for rows.Next() {
		var item SchemaMigration
		if err := rows.Scan(&item.Version, &item.File, &item.SHA256); err != nil {
			rows.Close()
			return SchemaMigrationReport{}, err
		}
		stored[item.Version] = item
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return SchemaMigrationReport{}, err
	}
	rows.Close()

	report := SchemaMigrationReport{Applied: []SchemaMigration{}, Adopted: []SchemaMigration{}, Skipped: []SchemaMigration{}}
	adopted, err := adoptPreHistorySchema(ctx, connection, sources, stored, completeBaseline)
	if err != nil {
		return report, err
	}
	for _, source := range sources {
		if item, wasAdopted := adopted[source.Version]; wasAdopted {
			stored[item.Version] = item
			report.Adopted = append(report.Adopted, item)
		}
	}
	for _, source := range sources {
		if _, wasAdopted := adopted[source.Version]; wasAdopted {
			continue
		}
		if prior, exists := stored[source.Version]; exists {
			if prior.File != source.File || prior.SHA256 != source.SHA256 {
				return report, fmt.Errorf("migration %s checksum or filename changed after it was applied", source.Version)
			}
			report.Skipped = append(report.Skipped, source.SchemaMigration)
			continue
		}
		transaction, err := connection.Begin(ctx)
		if err != nil {
			return report, err
		}
		if _, err := transaction.Exec(ctx, source.SQL); err != nil {
			_ = transaction.Rollback(ctx)
			return report, fmt.Errorf("apply migration %s: %w", source.File, err)
		}
		if _, err := transaction.Exec(ctx, `INSERT INTO quizcraft_schema_migrations(version,filename,source_sha256) VALUES($1,$2,$3)`, source.Version, source.File, source.SHA256); err != nil {
			_ = transaction.Rollback(ctx)
			return report, err
		}
		if err := transaction.Commit(ctx); err != nil {
			return report, err
		}
		report.Applied = append(report.Applied, source.SchemaMigration)
	}
	return report, nil
}

// adoptPreHistorySchema records the released 000001–000008 baseline in one
// transaction. It is deliberately conservative: an empty target starts from
// 000001, a complete old target is adopted, and any partial untracked schema
// fails closed instead of guessing which DDL has already run.
func adoptPreHistorySchema(ctx context.Context, connection *pgxpool.Conn, sources []migrationSource, stored map[string]SchemaMigration, shouldAdopt bool) (map[string]SchemaMigration, error) {
	if len(stored) != 0 || !shouldAdopt {
		return map[string]SchemaMigration{}, nil
	}
	containsTracker := false
	for _, source := range sources {
		if source.Version == firstTrackedSchemaMigrationVersion {
			containsTracker = true
			break
		}
	}
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	if !containsTracker {
		return nil, errors.New("cannot adopt a pre-history QuizCraft schema without migration 000009 in the supplied directory")
	}
	adopted := map[string]SchemaMigration{}
	for _, source := range sources {
		if source.Version >= firstTrackedSchemaMigrationVersion {
			break
		}
		if _, err := transaction.Exec(ctx, `INSERT INTO quizcraft_schema_migrations(version,filename,source_sha256) VALUES($1,$2,$3)`, source.Version, source.File, source.SHA256); err != nil {
			return nil, err
		}
		adopted[source.Version] = source.SchemaMigration
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, err
	}
	return adopted, nil
}

type relationReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func inspectPreHistoryBaseline(ctx context.Context, reader relationReader) (anyBaseline, completeBaseline bool, err error) {
	rows, err := reader.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename LIKE 'quizcraft\_%' ESCAPE '\' AND tablename <> 'quizcraft_schema_migrations' ORDER BY tablename`)
	if err != nil {
		return false, false, err
	}
	defer rows.Close()
	relations := map[string]bool{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return false, false, err
		}
		relations[table] = true
	}
	if err := rows.Err(); err != nil {
		return false, false, err
	}
	if len(relations) == 0 {
		return false, false, nil
	}
	for _, table := range preHistoryBaselineTables {
		if !relations[table] {
			return true, false, nil
		}
	}
	serverVersion, err := postgresServerVersion(ctx, reader)
	if err != nil {
		return false, false, err
	}
	if serverVersion < 160000 || serverVersion >= 170000 {
		return true, false, fmt.Errorf("refusing to adopt a pre-history QuizCraft schema outside the PostgreSQL 16 catalog baseline (server_version_num=%d)", serverVersion)
	}
	fingerprint, err := preHistorySchemaFingerprint(ctx, reader)
	if err != nil {
		return false, false, err
	}
	if fingerprint != preHistoryBaselineSchemaFingerprint {
		return true, false, fmt.Errorf("refusing to adopt a partial pre-history QuizCraft schema: catalog fingerprint %s does not match the released baseline", fingerprint)
	}
	return true, true, nil
}

func postgresServerVersion(ctx context.Context, reader relationReader) (int, error) {
	rows, err := reader.Query(ctx, `SHOW server_version_num`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errors.New("PostgreSQL did not return server_version_num")
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	version, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse PostgreSQL server_version_num: %w", err)
	}
	return version, nil
}

func preHistorySchemaFingerprint(ctx context.Context, reader relationReader) (string, error) {
	var definition strings.Builder
	columns, err := reader.Query(ctx, `SELECT c.relname,a.attnum,a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,a.attidentity,a.attgenerated,COALESCE(pg_get_expr(d.adbin,d.adrelid,false),'') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum WHERE n.nspname='public' AND c.relname=ANY($1::text[]) AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped ORDER BY c.relname,a.attnum`, preHistoryBaselineTables)
	if err != nil {
		return "", err
	}
	for columns.Next() {
		var relation, name, dataType, defaultValue string
		var number int
		var notNull bool
		var identity, generated byte
		if err := columns.Scan(&relation, &number, &name, &dataType, &notNull, &identity, &generated, &defaultValue); err != nil {
			columns.Close()
			return "", err
		}
		fmt.Fprintf(&definition, "column|%q|%d|%q|%q|%t|%q|%q|%q\n", relation, number, name, dataType, notNull, string(identity), string(generated), defaultValue)
	}
	if err := columns.Err(); err != nil {
		columns.Close()
		return "", err
	}
	columns.Close()

	constraints, err := reader.Query(ctx, `SELECT c.relname,k.conname,k.contype,k.condeferrable,k.condeferred,k.convalidated,pg_get_constraintdef(k.oid,false) FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=ANY($1::text[]) ORDER BY c.relname,k.conname`, preHistoryBaselineTables)
	if err != nil {
		return "", err
	}
	for constraints.Next() {
		var relation, name, definitionValue string
		var kind byte
		var deferrable, deferred, validated bool
		if err := constraints.Scan(&relation, &name, &kind, &deferrable, &deferred, &validated, &definitionValue); err != nil {
			constraints.Close()
			return "", err
		}
		fmt.Fprintf(&definition, "constraint|%q|%q|%q|%t|%t|%t|%q\n", relation, name, string(kind), deferrable, deferred, validated, definitionValue)
	}
	if err := constraints.Err(); err != nil {
		constraints.Close()
		return "", err
	}
	constraints.Close()

	indexes, err := reader.Query(ctx, `SELECT c.relname,i.relname,x.indisvalid,x.indisready,x.indisunique,x.indisprimary,pg_get_indexdef(x.indexrelid,0,false) FROM pg_index x JOIN pg_class c ON c.oid=x.indrelid JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=ANY($1::text[]) ORDER BY c.relname,i.relname`, preHistoryBaselineTables)
	if err != nil {
		return "", err
	}
	for indexes.Next() {
		var relation, name, definitionValue string
		var valid, ready, unique, primary bool
		if err := indexes.Scan(&relation, &name, &valid, &ready, &unique, &primary, &definitionValue); err != nil {
			indexes.Close()
			return "", err
		}
		fmt.Fprintf(&definition, "index|%q|%q|%t|%t|%t|%t|%q\n", relation, name, valid, ready, unique, primary, definitionValue)
	}
	if err := indexes.Err(); err != nil {
		indexes.Close()
		return "", err
	}
	indexes.Close()

	triggers, err := reader.Query(ctx, `SELECT c.relname,t.tgname,t.tgenabled,t.tgtype,pg_get_triggerdef(t.oid,false) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=ANY($1::text[]) AND NOT t.tgisinternal ORDER BY c.relname,t.tgname`, preHistoryBaselineTables)
	if err != nil {
		return "", err
	}
	for triggers.Next() {
		var relation, name, triggerDefinition string
		var enabled byte
		var triggerType int16
		if err := triggers.Scan(&relation, &name, &enabled, &triggerType, &triggerDefinition); err != nil {
			triggers.Close()
			return "", err
		}
		fmt.Fprintf(&definition, "trigger|%q|%q|%q|%d|%q\n", relation, name, string(enabled), triggerType, triggerDefinition)
	}
	if err := triggers.Err(); err != nil {
		triggers.Close()
		return "", err
	}
	triggers.Close()

	functions, err := reader.Query(ctx, `SELECT p.proname,p.oid::regprocedure::text,pg_get_function_identity_arguments(p.oid),pg_get_functiondef(p.oid) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname=ANY($1::text[]) ORDER BY p.proname,p.oid::regprocedure::text`, preHistoryBaselineFunctionNames)
	if err != nil {
		return "", err
	}
	for functions.Next() {
		var name, identity, arguments, definitionValue string
		if err := functions.Scan(&name, &identity, &arguments, &definitionValue); err != nil {
			functions.Close()
			return "", err
		}
		fmt.Fprintf(&definition, "function|%q|%q|%q|%q\n", name, identity, arguments, definitionValue)
	}
	if err := functions.Err(); err != nil {
		functions.Close()
		return "", err
	}
	functions.Close()

	digest := sha256.Sum256([]byte(definition.String()))
	return hex.EncodeToString(digest[:]), nil
}

func readMigrationSources(directory string) ([]migrationSource, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	sources := make([]migrationSource, 0, len(entries))
	seenVersions := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := versionedMigrationFile.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		if seenVersions[matches[1]] {
			return nil, fmt.Errorf("duplicate migration version %s", matches[1])
		}
		seenVersions[matches[1]] = true
		path := filepath.Join(directory, entry.Name())
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(source)) == "" {
			return nil, fmt.Errorf("migration %s is empty", entry.Name())
		}
		digest := sha256.Sum256(source)
		sources = append(sources, migrationSource{
			SchemaMigration: SchemaMigration{Version: matches[1], File: entry.Name(), SHA256: hex.EncodeToString(digest[:])},
			SQL:             string(source),
		})
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("no versioned .up.sql migrations found in %s", directory)
	}
	sort.Slice(sources, func(left, right int) bool { return sources[left].Version < sources[right].Version })
	return sources, nil
}

const legacySnapshotArtifactFormat = "quizcraft-v2-legacy-snapshot/v1"

// LegacySnapshotArtifact freezes the exact import input for a recoverable
// full migration. The source database identity prevents an operator from
// accidentally importing a snapshot back into its source database.
type LegacySnapshotArtifact struct {
	Format                 string         `json:"format"`
	CreatedAt              time.Time      `json:"created_at"`
	SourceDatabaseIdentity string         `json:"source_database_identity"`
	SnapshotSHA256         string         `json:"snapshot_sha256"`
	Snapshot               LegacySnapshot `json:"snapshot"`
}

// WriteLegacySnapshotArtifact writes a root/operator-owned import input using
// create-only semantics. Resume must use this exact immutable artifact rather
// than resampling a moving legacy database.
func WriteLegacySnapshotArtifact(path, sourceDatabaseIdentity string, snapshot LegacySnapshot) (LegacySnapshotArtifact, error) {
	snapshot, err := validatedSnapshot(snapshot)
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	if strings.TrimSpace(sourceDatabaseIdentity) == "" {
		return LegacySnapshotArtifact{}, errors.New("legacy source database identity is required")
	}
	if _, err := os.Stat(path); err == nil {
		return LegacySnapshotArtifact{}, fmt.Errorf("snapshot artifact already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return LegacySnapshotArtifact{}, err
	}
	checksum, err := snapshotChecksum(snapshot)
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	artifact := LegacySnapshotArtifact{
		Format:                 legacySnapshotArtifactFormat,
		CreatedAt:              time.Now().UTC(),
		SourceDatabaseIdentity: strings.TrimSpace(sourceDatabaseIdentity),
		SnapshotSHA256:         checksum,
		Snapshot:               snapshot,
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return LegacySnapshotArtifact{}, err
	}
	temporary, err := os.CreateTemp(directory, ".quizcraft-v2-snapshot-*")
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return LegacySnapshotArtifact{}, err
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		_ = temporary.Close()
		return LegacySnapshotArtifact{}, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return LegacySnapshotArtifact{}, err
	}
	if err := temporary.Close(); err != nil {
		return LegacySnapshotArtifact{}, err
	}
	if err := os.Link(temporaryName, path); err != nil {
		return LegacySnapshotArtifact{}, fmt.Errorf("create snapshot artifact without overwrite: %w", err)
	}
	return artifact, nil
}

// ReadLegacySnapshotArtifact verifies the immutable artifact before it reaches
// the import boundary.
func ReadLegacySnapshotArtifact(path string) (LegacySnapshotArtifact, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	var artifact LegacySnapshotArtifact
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		return LegacySnapshotArtifact{}, err
	}
	if artifact.Format != legacySnapshotArtifactFormat {
		return LegacySnapshotArtifact{}, errors.New("unsupported legacy snapshot artifact format")
	}
	if strings.TrimSpace(artifact.SourceDatabaseIdentity) == "" {
		return LegacySnapshotArtifact{}, errors.New("snapshot artifact is missing source database identity")
	}
	snapshot, err := validatedSnapshot(artifact.Snapshot)
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	checksum, err := snapshotChecksum(snapshot)
	if err != nil {
		return LegacySnapshotArtifact{}, err
	}
	if artifact.SnapshotSHA256 != checksum {
		return LegacySnapshotArtifact{}, errors.New("snapshot artifact checksum does not match its content")
	}
	artifact.Snapshot = snapshot
	return artifact, nil
}

func validatedSnapshot(snapshot LegacySnapshot) (LegacySnapshot, error) {
	snapshot.SourceName = strings.TrimSpace(snapshot.SourceName)
	if snapshot.SourceName == "" || snapshot.CutoffEventID < 0 {
		return LegacySnapshot{}, errors.New("migration source and non-negative cutoff event are required")
	}
	if len(snapshot.Rankings) == 0 {
		snapshot.Rankings = json.RawMessage(`[]`)
	}
	if !json.Valid(snapshot.Rankings) {
		return LegacySnapshot{}, errors.New("legacy ranking snapshot must be valid JSON")
	}
	seenBanks := map[string]bool{}
	for index := range snapshot.Banks {
		snapshot.Banks[index].BankKey = strings.TrimSpace(snapshot.Banks[index].BankKey)
		if snapshot.Banks[index].BankKey == "" || seenBanks[snapshot.Banks[index].BankKey] {
			return LegacySnapshot{}, errors.New("legacy snapshot contains a missing or duplicate bank key")
		}
		if !json.Valid(snapshot.Banks[index].Document) {
			return LegacySnapshot{}, fmt.Errorf("legacy snapshot bank %s is not valid JSON", snapshot.Banks[index].BankKey)
		}
		seenBanks[snapshot.Banks[index].BankKey] = true
	}
	return snapshot, nil
}

func snapshotChecksum(snapshot LegacySnapshot) (string, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return sha256Hex(encoded), nil
}
