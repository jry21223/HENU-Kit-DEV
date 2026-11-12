package quizcraft

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Release catalog fingerprints are derived from the accepted exact migration
// stream on PostgreSQL16. Ownership and OIDs are deliberately not schema shape.
//
//go:embed db/release/catalog.sql
var releaseCatalogSQL string

var releaseCatalogFingerprints = map[string]string{
	"000011": "d85af2a604a10149193f9bdd1abd2bfc6d06ceddaeb2b014a30922e8ab1e1d2d",
	"000012": "50e86d48063156e0847c0116bfb8a398aa4c6f438eb9e669c09973ea2392415d",
}
var releaseSnapshotPattern = regexp.MustCompile(`^[0-9A-F]+-[0-9A-F]+-[0-9]+$`)

type ReleaseDatabaseIdentity struct {
	Database string    `json:"database"`
	OID      uint32    `json:"oid"`
	Started  time.Time `json:"started"`
	Port     int       `json:"port"`
	Version  int       `json:"version"`
}
type ReleaseSchemaState struct {
	Version       string            `json:"version"`
	History       []SchemaMigration `json:"history"`
	HistorySHA256 string            `json:"history_sha256"`
	CatalogSHA256 string            `json:"catalog_sha256"`
}
type ReleaseState struct {
	Format                        string                  `json:"format"`
	DatabaseIdentity              ReleaseDatabaseIdentity `json:"database_identity"`
	Schema                        ReleaseSchemaState      `json:"schema"`
	RequiresEnglishTrainingDataV1 bool                    `json:"requires_english_training_data_v1"`
}

// ReadReleaseState recognizes only the two reviewed tracked release schemas.
// It never adopts history, creates tables, accepts SQL, or changes a fact.
// The command boundary additionally requires the independently owned V2 target.
func ReadReleaseState(ctx context.Context, pool *pgxpool.Pool, directory, snapshot string) (ReleaseState, error) {
	var state ReleaseState
	if pool == nil {
		return state, errors.New("release target is required")
	}
	if snapshot != "" && !releaseSnapshotPattern.MatchString(snapshot) {
		return state, errors.New("invalid release snapshot")
	}
	sources, err := readMigrationSources(directory)
	if err != nil {
		return state, err
	}
	if len(sources) != 12 {
		return state, errors.New("unknown release migration source inventory")
	}
	for i, s := range sources {
		if s.Version != fmt.Sprintf("%06d", i+1) {
			return state, errors.New("noncontiguous release source inventory")
		}
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return state, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if snapshot != "" {
		if _, err = tx.Exec(ctx, "SET TRANSACTION SNAPSHOT '"+snapshot+"'"); err != nil {
			return state, err
		}
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path=public,pg_catalog`); err != nil {
		return state, err
	}
	if err = tx.QueryRow(ctx, `SELECT current_database(),(SELECT oid FROM pg_database WHERE datname=current_database()),pg_postmaster_start_time(),current_setting('port')::int,current_setting('server_version_num')::int`).Scan(&state.DatabaseIdentity.Database, &state.DatabaseIdentity.OID, &state.DatabaseIdentity.Started, &state.DatabaseIdentity.Port, &state.DatabaseIdentity.Version); err != nil {
		return state, err
	}
	if state.DatabaseIdentity.Version < 160000 || state.DatabaseIdentity.Version >= 170000 {
		return state, errors.New("release state requires PostgreSQL16")
	}
	rows, err := tx.Query(ctx, `SELECT version,filename,source_sha256 FROM public.quizcraft_schema_migrations ORDER BY version`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var s SchemaMigration
		if err = rows.Scan(&s.Version, &s.File, &s.SHA256); err != nil {
			rows.Close()
			return state, err
		}
		state.Schema.History = append(state.Schema.History, s)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()
	n := len(state.Schema.History)
	if n != 11 && n != 12 {
		return state, errors.New("unknown release history")
	}
	for i, s := range state.Schema.History {
		if s != sources[i].SchemaMigration {
			return state, errors.New("release history differs from exact source")
		}
	}
	state.Schema.Version = state.Schema.History[n-1].Version
	body, err := json.Marshal(state.Schema.History)
	if err != nil {
		return state, err
	}
	sum := sha256.Sum256(body)
	state.Schema.HistorySHA256 = hex.EncodeToString(sum[:])
	rows, err = tx.Query(ctx, releaseCatalogSQL)
	if err != nil {
		return state, err
	}
	h := sha256.New()
	for rows.Next() {
		var record string
		if err = rows.Scan(&record); err != nil {
			rows.Close()
			return state, err
		}
		_, _ = h.Write([]byte(record + "\n"))
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return state, err
	}
	rows.Close()
	state.Schema.CatalogSHA256 = hex.EncodeToString(h.Sum(nil))
	if state.Schema.CatalogSHA256 != releaseCatalogFingerprints[state.Schema.Version] {
		return state, errors.New("release catalog is not a known healthy schema")
	}
	if n == 12 {
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quizcraft_training_units) OR EXISTS(SELECT 1 FROM quizcraft_training_unit_versions) OR EXISTS(SELECT 1 FROM quizcraft_training_members) OR EXISTS(SELECT 1 FROM quizcraft_training_sessions) OR EXISTS(SELECT 1 FROM quizcraft_training_outcomes) OR EXISTS(SELECT 1 FROM quizcraft_training_disclosures) OR EXISTS(SELECT 1 FROM quizcraft_practice_sessions WHERE purpose='training')`).Scan(&state.RequiresEnglishTrainingDataV1)
		if err != nil {
			return state, err
		}
	}
	state.Format = "quizcraft-release-state-v1"
	if err = tx.Commit(ctx); err != nil {
		return ReleaseState{}, err
	}
	return state, nil
}
