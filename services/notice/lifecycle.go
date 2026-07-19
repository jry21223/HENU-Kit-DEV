package notice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lifecycleStore struct {
	database *pgxpool.Pool
}

func (s *lifecycleStore) createSource(ctx context.Context, tx pgx.Tx, value actor, requestID string, input sourceInput) (map[string]any, error) {
	id := uuid.New()
	_, err := tx.Exec(ctx, `INSERT INTO notice_sources (id,code,name,canonical_url,created_by) VALUES ($1,$2,$3,$4,$5)`, id, input.Code, input.Name, input.CanonicalURL, value.userID)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, value, "source.create", "notice_source", id, requestID); err != nil {
		return nil, err
	}
	return map[string]any{"id": id.String(), "code": input.Code, "name": input.Name, "canonical_url": input.CanonicalURL}, nil
}

func (s *lifecycleStore) createVersion(ctx context.Context, tx pgx.Tx, value actor, requestID string, sourceID uuid.UUID, input versionInput) (map[string]any, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM notice_sources WHERE id=$1 FOR UPDATE`, sourceID).Scan(&exists); err != nil {
		return nil, err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM notice_versions WHERE source_id=$1`, sourceID).Scan(&version); err != nil {
		return nil, err
	}
	id, hash := uuid.New(), sha256.Sum256([]byte(input.Body))
	if _, err := tx.Exec(ctx, `INSERT INTO notice_versions (id,source_id,version,title,body,source_url,content_hash,source_published_at,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, sourceID, version, input.Title, input.Body, input.SourceURL, hex.EncodeToString(hash[:]), input.SourcePublishedAt, value.userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO notice_lifecycles (notice_version_id) VALUES ($1)`, id); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, value, "version.create", "notice_version", id, requestID); err != nil {
		return nil, err
	}
	return map[string]any{"id": id.String(), "source_id": sourceID.String(), "version": version, "state": "pending_review", "revision": 1}, nil
}

func (s *lifecycleStore) review(ctx context.Context, tx pgx.Tx, value actor, requestID string, versionID uuid.UUID, input reviewInput) (map[string]any, error) {
	var state string
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT state,revision FROM notice_lifecycles WHERE notice_version_id=$1 FOR UPDATE`, versionID).Scan(&state, &revision); err != nil {
		return nil, err
	}
	if state != "pending_review" || revision != input.ExpectedRevision {
		return nil, errConflict
	}
	if _, err := tx.Exec(ctx, `INSERT INTO notice_reviews (notice_version_id,decision,note,actor_user_id,request_id) VALUES ($1,$2,$3,$4,$5)`, versionID, input.Decision, input.Note, value.userID, requestID); err != nil {
		return nil, err
	}
	revision++
	if _, err := tx.Exec(ctx, `UPDATE notice_lifecycles SET state=$2,revision=$3,updated_at=now() WHERE notice_version_id=$1`, versionID, input.Decision, revision); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, value, "review."+input.Decision, "notice_version", versionID, requestID); err != nil {
		return nil, err
	}
	return map[string]any{"version_id": versionID.String(), "state": input.Decision, "revision": revision}, nil
}

func (s *lifecycleStore) distribute(ctx context.Context, tx pgx.Tx, value actor, requestID string, versionID uuid.UUID, input distributionInput) (map[string]any, error) {
	var state string
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT state,revision FROM notice_lifecycles WHERE notice_version_id=$1 FOR UPDATE`, versionID).Scan(&state, &revision); err != nil {
		return nil, err
	}
	if state != "approved" || revision != input.ExpectedRevision {
		return nil, errConflict
	}
	id := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO notice_distributions (id,notice_version_id,channel,audience_kind,audience_value,actor_user_id,request_id) VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, versionID, input.Channel, input.Audience.Kind, input.Audience.Value, value.userID, requestID); err != nil {
		return nil, err
	}
	revision++
	if _, err := tx.Exec(ctx, `UPDATE notice_lifecycles SET state='distributed',revision=$2,updated_at=now() WHERE notice_version_id=$1`, versionID, revision); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, value, "distribution.queue", "notice_distribution", id, requestID); err != nil {
		return nil, err
	}
	return map[string]any{"id": id.String(), "version_id": versionID.String(), "status": "queued", "revision": revision}, nil
}

func (s *lifecycleStore) summary(ctx context.Context) (map[string]int64, error) {
	counts := map[string]int64{"pending_review": 0, "approved": 0, "distributed": 0}
	rows, err := s.database.Query(ctx, `SELECT state,count(*) FROM notice_lifecycles GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		counts[state] = count
	}
	return counts, rows.Err()
}

func (s *lifecycleStore) snapshot(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.database.Query(ctx, `SELECT v.id,s.id,s.code,s.name,v.version,v.title,v.body,v.source_url,v.content_hash,l.state,l.revision,v.source_published_at,v.created_at,(SELECT count(*) FROM notice_distributions d WHERE d.notice_version_id=v.id),COALESCE((SELECT status FROM notice_distributions d WHERE d.notice_version_id=v.id ORDER BY created_at DESC LIMIT 1),'') FROM notice_versions v JOIN notice_sources s ON s.id=v.source_id JOIN notice_lifecycles l ON l.notice_version_id=v.id ORDER BY v.created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var versionID, sourceID uuid.UUID
		var code, name, title, body, sourceURL, contentHash, state, distributionStatus string
		var version int
		var revision, distributionCount int64
		var publishedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(&versionID, &sourceID, &code, &name, &version, &title, &body, &sourceURL, &contentHash, &state, &revision, &publishedAt, &createdAt, &distributionCount, &distributionStatus); err != nil {
			return nil, err
		}
		item := map[string]any{"id": versionID.String(), "source": map[string]any{"id": sourceID.String(), "code": code, "name": name}, "version": version, "title": title, "body": body, "source_url": sourceURL, "content_hash": contentHash, "state": state, "revision": revision, "created_at": createdAt, "distribution_count": distributionCount}
		if publishedAt != nil {
			item["source_published_at"] = publishedAt
		}
		if distributionStatus != "" {
			item["distribution_status"] = distributionStatus
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func audit(ctx context.Context, tx pgx.Tx, value actor, action, resourceType string, resourceID uuid.UUID, requestID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO notice_audit_events (actor_user_id,permission_code,action,resource_type,resource_id,request_id) VALUES ($1,$2,$3,$4,$5,$6)`, value.userID, value.permission, action, resourceType, resourceID, requestID)
	return err
}
