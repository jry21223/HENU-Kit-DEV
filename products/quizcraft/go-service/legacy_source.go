package quizcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LegacyPostgresSource struct{ database *pgxpool.Pool }

func NewLegacyPostgresSource(database *pgxpool.Pool) (*LegacyPostgresSource, error) {
	if database == nil {
		return nil, errors.New("legacy QuizCraft database is required")
	}
	return &LegacyPostgresSource{database: database}, nil
}

func (source *LegacyPostgresSource) DatabaseIdentity(ctx context.Context) (string, error) {
	return databaseIdentity(ctx, source.database)
}

func DatabaseIdentity(ctx context.Context, database *pgxpool.Pool) (string, error) {
	return databaseIdentity(ctx, database)
}

func RequireEmptyTarget(ctx context.Context, database *pgxpool.Pool) error {
	rows, err := database.Query(ctx, `SELECT schemaname,tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename LIKE 'quizcraft\_%' ESCAPE '\' ORDER BY tablename`)
	if err != nil {
		return err
	}
	type tableName struct{ schema, table string }
	tables := make([]tableName, 0)
	for rows.Next() {
		var name tableName
		if err := rows.Scan(&name.schema, &name.table); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, name := range tables {
		// Schema migration history is operator metadata, not a V2 business fact.
		// A target must receive that history before full import can be resumed, so
		// treating it as imported content would make cmd/migrate and full
		// reconciliation mutually exclusive.
		if name.table == "quizcraft_schema_migrations" {
			continue
		}
		var hasFacts bool
		query := `SELECT EXISTS(SELECT 1 FROM ` + pgx.Identifier{name.schema, name.table}.Sanitize() + ` LIMIT 1)`
		if err := database.QueryRow(ctx, query).Scan(&hasFacts); err != nil {
			return err
		}
		if hasFacts {
			return fmt.Errorf("full migration target contains existing facts in %s", name.table)
		}
	}
	return nil
}

func databaseIdentity(ctx context.Context, database databaseQueryRower) (string, error) {
	var systemIdentifier, databaseOID string
	err := database.QueryRow(ctx, `SELECT c.system_identifier::text,d.oid::text FROM pg_control_system() c JOIN pg_database d ON d.datname=current_database()`).Scan(&systemIdentifier, &databaseOID)
	return systemIdentifier + "/" + databaseOID, err
}

func (source *LegacyPostgresSource) Snapshot(ctx context.Context, sourceName string) (LegacySnapshot, error) {
	snapshot := LegacySnapshot{SourceName: strings.TrimSpace(sourceName)}
	if snapshot.SourceName == "" {
		return snapshot, errors.New("legacy source name is required")
	}
	tx, err := source.database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return snapshot, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(event_id),0) FROM quizcraft_migration_events`).Scan(&snapshot.CutoffEventID); err != nil {
		return snapshot, fmt.Errorf("read legacy migration event cutoff: %w", err)
	}
	type bankValue struct {
		name, color, sourceFile string
		metadata                map[string]any
		questions               []any
	}
	banks := map[string]*bankValue{}
	rows, err := tx.Query(ctx, `SELECT bank_key,name,color,source_file,metadata FROM question_banks ORDER BY bank_key`)
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var key string
		var value bankValue
		var metadata []byte
		if err := rows.Scan(&key, &value.name, &value.color, &value.sourceFile, &metadata); err != nil {
			rows.Close()
			return snapshot, err
		}
		value.metadata = map[string]any{}
		_ = json.Unmarshal(metadata, &value.metadata)
		banks[key] = &value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, err
	}
	rows.Close()
	rows, err = tx.Query(ctx, `SELECT bank_key,payload FROM bank_questions ORDER BY bank_key,question_id`)
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return snapshot, err
		}
		value := banks[key]
		if value == nil {
			rows.Close()
			return snapshot, fmt.Errorf("legacy question references missing bank %q", key)
		}
		var question any
		if err := json.Unmarshal(raw, &question); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("decode legacy question for bank %q: %w", key, err)
		}
		value.questions = append(value.questions, question)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, err
	}
	rows.Close()
	keys := make([]string, 0, len(banks))
	for key := range banks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := banks[key]
		meta := value.metadata
		meta["name"] = value.name
		meta["color"] = value.color
		meta["source_file"] = value.sourceFile
		meta["total"] = len(value.questions)
		document, _ := json.Marshal(map[string]any{"meta": meta, "questions": value.questions})
		snapshot.Banks = append(snapshot.Banks, LegacyBank{BankKey: key, Document: document})
	}

	rows, err = tx.Query(ctx, `SELECT feedback_id::text,COALESCE(question_bank,''),COALESCE(question_id,''),question_index,COALESCE(question_content,''),COALESCE(user_id,''),source_page,suggestion,status,resolution_note,created_at,resolved_at FROM feedbacks ORDER BY feedback_id`)
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var feedback LegacyFeedback
		if err := rows.Scan(&feedback.LegacyID, &feedback.BankKey, &feedback.QuestionID, &feedback.QuestionIndex, &feedback.QuestionContent, &feedback.UserID, &feedback.SourcePage, &feedback.Suggestion, &feedback.Status, &feedback.ResolutionNote, &feedback.CreatedAt, &feedback.ResolvedAt); err != nil {
			rows.Close()
			return snapshot, err
		}
		snapshot.Feedback = append(snapshot.Feedback, feedback)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, err
	}
	rows.Close()
	snapshot.Rankings, err = rankingSnapshot(ctx, tx)
	if err != nil {
		return snapshot, err
	}
	if err := tx.Commit(ctx); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (source *LegacyPostgresSource) EventsAfter(ctx context.Context, cursor int64, limit int) ([]LegacyEvent, int64, error) {
	if cursor < 0 || limit < 1 || limit > 10000 {
		return nil, 0, errors.New("valid legacy event cursor and limit are required")
	}
	tx, err := source.database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var head int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(event_id),0) FROM quizcraft_migration_events`).Scan(&head); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT event_id,event_type,aggregate_key,payload FROM quizcraft_migration_events WHERE event_id>$1 AND event_id<=$2 ORDER BY event_id LIMIT $3`, cursor, head, limit)
	if err != nil {
		return nil, 0, err
	}
	events := make([]LegacyEvent, 0)
	needsRankingSnapshot := false
	for rows.Next() {
		var event LegacyEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.AggregateKey, &event.Payload); err != nil {
			return nil, 0, err
		}
		event.sourcePayloadSHA256 = sha256Hex(event.Payload)
		needsRankingSnapshot = needsRankingSnapshot || event.Type == "ranking.changed"
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	if needsRankingSnapshot {
		standings, err := rankingSnapshot(ctx, tx)
		if err != nil {
			return nil, 0, err
		}
		for index := range events {
			if events[index].Type == "ranking.changed" {
				events[index].Payload = standings
			}
		}
	}
	bankPayloads := map[string]json.RawMessage{}
	feedbackPayloads := map[string]json.RawMessage{}
	for index := range events {
		switch events[index].Type {
		case "bank.upserted":
			payload, ok := bankPayloads[events[index].AggregateKey]
			if !ok {
				bank, err := bankSnapshot(ctx, tx, events[index].AggregateKey)
				if err != nil {
					return nil, 0, err
				}
				payload, _ = json.Marshal(bank)
				bankPayloads[events[index].AggregateKey] = payload
			}
			events[index].Payload = payload
		case "feedback.upserted":
			payload, ok := feedbackPayloads[events[index].AggregateKey]
			if !ok {
				feedback, err := feedbackSnapshot(ctx, tx, events[index].AggregateKey)
				if err != nil {
					return nil, 0, err
				}
				payload, _ = json.Marshal(feedback)
				feedbackPayloads[events[index].AggregateKey] = payload
			}
			events[index].Payload = payload
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return events, head, nil
}

type legacyReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func bankSnapshot(ctx context.Context, reader legacyReader, bankKey string) (LegacyBank, error) {
	var name, color, sourceFile string
	var metadataRaw []byte
	if err := reader.QueryRow(ctx, `SELECT name,color,source_file,metadata FROM question_banks WHERE bank_key=$1`, bankKey).Scan(&name, &color, &sourceFile, &metadataRaw); err != nil {
		return LegacyBank{}, fmt.Errorf("hydrate legacy bank event %q: %w", bankKey, err)
	}
	metadata := map[string]any{}
	_ = json.Unmarshal(metadataRaw, &metadata)
	metadata["name"], metadata["color"], metadata["source_file"] = name, color, sourceFile
	rows, err := reader.Query(ctx, `SELECT payload FROM bank_questions WHERE bank_key=$1 ORDER BY question_id`, bankKey)
	if err != nil {
		return LegacyBank{}, err
	}
	defer rows.Close()
	questions := make([]any, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return LegacyBank{}, err
		}
		var question any
		if err := json.Unmarshal(raw, &question); err != nil {
			return LegacyBank{}, err
		}
		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return LegacyBank{}, err
	}
	metadata["total"] = len(questions)
	document, _ := json.Marshal(map[string]any{"meta": metadata, "questions": questions})
	return LegacyBank{BankKey: bankKey, Document: document}, nil
}

func feedbackSnapshot(ctx context.Context, reader legacyReader, legacyID string) (LegacyFeedback, error) {
	var feedback LegacyFeedback
	err := reader.QueryRow(ctx, `SELECT feedback_id::text,COALESCE(question_bank,''),COALESCE(question_id,''),question_index,COALESCE(question_content,''),COALESCE(user_id,''),source_page,suggestion,status,resolution_note,created_at,resolved_at FROM feedbacks WHERE feedback_id::text=$1`, legacyID).Scan(&feedback.LegacyID, &feedback.BankKey, &feedback.QuestionID, &feedback.QuestionIndex, &feedback.QuestionContent, &feedback.UserID, &feedback.SourcePage, &feedback.Suggestion, &feedback.Status, &feedback.ResolutionNote, &feedback.CreatedAt, &feedback.ResolvedAt)
	if err != nil {
		return feedback, fmt.Errorf("hydrate legacy feedback event %q: %w", legacyID, err)
	}
	return feedback, nil
}

func rankingSnapshot(ctx context.Context, reader legacyReader) (json.RawMessage, error) {
	rows, err := reader.Query(ctx, `SELECT u.user_id,u.display_name,s.correct,s.total FROM user_stats s JOIN users u ON u.user_id=s.user_id WHERE s.total>0 ORDER BY s.correct DESC,CASE WHEN s.total>0 THEN s.correct::float/s.total ELSE 0 END DESC,u.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	standings := make([]map[string]any, 0)
	for rows.Next() {
		var legacyID, name string
		var correct, total int64
		if err := rows.Scan(&legacyID, &name, &correct, &total); err != nil {
			return nil, err
		}
		standings = append(standings, map[string]any{"legacy_subject_id": legacyID, "name": name, "correct": correct, "total": total})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(standings)
	return encoded, nil
}
