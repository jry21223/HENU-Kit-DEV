package noticeowner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	noticeclient "henukit.dev/console-gateway/internal/notice"
	noticeowner "henukit.dev/notice"
)

const (
	clientID = "console-gateway-notice"
	keyID    = "notice-summary-key"
	secret   = "notice-gateway-integration-secret-32bytes"
)

func TestGatewayClientReadsReviewsAndDistributesThroughRealNoticeOwner(t *testing.T) {
	databaseURL := os.Getenv("NOTICE_GATEWAY_INTEGRATION_DATABASE_URL")
	redisAddress := os.Getenv("NOTICE_GATEWAY_INTEGRATION_REDIS_ADDR")
	if databaseURL == "" || redisAddress == "" {
		t.Skip("Notice Gateway integration database and Redis are required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddress, DB: 15})
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })

	owner, err := noticeowner.New(noticeowner.Config{
		Database: pool,
		Redis:    redisClient,
		ClientID: clientID,
		Keys:     map[string]string{keyID: secret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(owner)
	t.Cleanup(server.Close)

	client, err := noticeclient.New(server.URL, clientID, secret, keyID, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	actorID := uuid.NewString()
	runID := uuid.NewString()
	requestContext := noticeclient.WithRequestID(ctx, "req_notice_gateway_integration")
	sourceBody, _ := json.Marshal(map[string]string{"code": "gateway-" + runID, "name": "Gateway integration", "canonical_url": "https://example.edu/notices"})
	sourcePayload, err := client.CreateSource(requestContext, actorID, "idem_notice_gateway_source_"+runID, sourceBody)
	source := call(t, sourcePayload, err)
	versionPayload, err := client.CreateVersion(requestContext, actorID, stringValue(t, source, "id"), "idem_notice_gateway_version_"+runID, []byte(`{"title":"暑期安排","body":"不可变正文","source_url":"https://example.edu/notices/1"}`))
	version := call(t, versionPayload, err)
	versionID := stringValue(t, version, "id")

	pendingPayload, err := client.Snapshot(requestContext, actorID)
	pending := call(t, pendingPayload, err)
	if !bytes.Contains(pending, []byte(`"state":"pending_review"`)) || !bytes.Contains(pending, []byte(`"title":"暑期安排"`)) {
		t.Fatalf("pending snapshot omitted owner state: %s", pending)
	}

	reviewPayload, err := client.Review(requestContext, actorID, versionID, "idem_notice_gateway_review_"+runID, []byte(`{"decision":"approved","note":"来源与正文已核验","expected_revision":1}`))
	review := call(t, reviewPayload, err)
	if stringValue(t, review, "state") != "approved" || numberValue(t, review, "revision") != 2 {
		t.Fatalf("review result = %s", review)
	}
	if _, err := client.Review(requestContext, actorID, versionID, "idem_notice_gateway_stale_review_"+runID, []byte(`{"decision":"rejected","note":"过期审核","expected_revision":1}`)); !errors.Is(err, noticeclient.ErrConflict) {
		t.Fatalf("stale review error = %v, want Notice conflict", err)
	}
	if _, err := client.Distribute(requestContext, actorID, versionID, "idem_notice_gateway_stale_distribution_"+runID, []byte(`{"channel":"in_app","audience":{"kind":"all_students"},"expected_revision":1}`)); !errors.Is(err, noticeclient.ErrConflict) {
		t.Fatalf("stale distribution error = %v, want Notice conflict", err)
	}
	distributionPayload, err := client.Distribute(requestContext, actorID, versionID, "idem_notice_gateway_distribution_"+runID, []byte(`{"channel":"in_app","audience":{"kind":"all_students"},"expected_revision":2}`))
	distribution := call(t, distributionPayload, err)
	if stringValue(t, distribution, "status") != "queued" || numberValue(t, distribution, "revision") != 3 {
		t.Fatalf("distribution result = %s", distribution)
	}

	distributedPayload, err := client.Snapshot(requestContext, actorID)
	distributed := call(t, distributedPayload, err)
	if !bytes.Contains(distributed, []byte(`"state":"distributed"`)) || !bytes.Contains(distributed, []byte(`"distribution_status":"queued"`)) {
		t.Fatalf("distributed snapshot omitted persisted owner state: %s", distributed)
	}
	rejectedVersionPayload, err := client.CreateVersion(requestContext, actorID, stringValue(t, source, "id"), "idem_notice_gateway_rejected_version_"+runID, []byte(`{"title":"失效通知","body":"不应分发","source_url":"https://example.edu/notices/2"}`))
	rejectedVersion := call(t, rejectedVersionPayload, err)
	rejectedVersionID := stringValue(t, rejectedVersion, "id")
	rejectedPayload, err := client.Review(requestContext, actorID, rejectedVersionID, "idem_notice_gateway_reject_"+runID, []byte(`{"decision":"rejected","note":"来源已失效","expected_revision":1}`))
	rejected := call(t, rejectedPayload, err)
	if stringValue(t, rejected, "state") != "rejected" || numberValue(t, rejected, "revision") != 2 {
		t.Fatalf("rejected review result = %s", rejected)
	}
	rejectedSnapshotPayload, err := client.Snapshot(requestContext, actorID)
	rejectedSnapshot := call(t, rejectedSnapshotPayload, err)
	if !bytes.Contains(rejectedSnapshot, []byte(`"title":"失效通知"`)) || !bytes.Contains(rejectedSnapshot, []byte(`"state":"rejected"`)) {
		t.Fatalf("snapshot omitted rejected persisted state: %s", rejectedSnapshot)
	}
	var reviews, distributions, actorAudits int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM notice_reviews WHERE notice_version_id=$1),
		(SELECT count(*) FROM notice_distributions WHERE notice_version_id=$1),
		(SELECT count(*) FROM notice_audit_events WHERE actor_user_id=$2 AND action IN ('review.approved','distribution.queue'))`, versionID, actorID).Scan(&reviews, &distributions, &actorAudits); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 || distributions != 1 || actorAudits != 2 {
		t.Fatalf("owner facts reviews/distributions/audits=%d/%d/%d, want 1/1/2", reviews, distributions, actorAudits)
	}
	var rejectedReviews, rejectedDistributions int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM notice_reviews WHERE notice_version_id=$1),
		(SELECT count(*) FROM notice_distributions WHERE notice_version_id=$1)`, rejectedVersionID).Scan(&rejectedReviews, &rejectedDistributions); err != nil {
		t.Fatal(err)
	}
	if rejectedReviews != 1 || rejectedDistributions != 0 {
		t.Fatalf("rejected owner facts reviews/distributions=%d/%d, want 1/0", rejectedReviews, rejectedDistributions)
	}
}

func call(t *testing.T, payload json.RawMessage, err error) json.RawMessage {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func stringValue(t *testing.T, payload json.RawMessage, key string) string {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	result, _ := value[key].(string)
	return result
}

func numberValue(t *testing.T, payload json.RawMessage, key string) int {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		t.Fatal(err)
	}
	result, _ := value[key].(float64)
	return int(result)
}
