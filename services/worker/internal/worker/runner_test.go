package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/worker/internal/mailprovider"
)

type fakeEventPublisher struct {
	events []outboxEvent
	err    error
}

func (publisher *fakeEventPublisher) Publish(_ context.Context, _ string, event outboxEvent) error {
	publisher.events = append(publisher.events, event)
	return publisher.err
}

func TestOutboxProcessorPublishesAndMarksEvent(t *testing.T) {
	db := newWorkerTestDB(t)
	event := outboxEvent{AggregateType: "food_entry", AggregateID: "entry-1", EventType: "food.tier_adjusted.v1", Payload: []byte(`{"entry_id":"entry-1"}`), Status: "pending", AvailableAt: time.Now().UTC()}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	publisher := &fakeEventPublisher{}
	if err := NewOutboxProcessor(db, publisher, "platform_events").ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&event, "id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if event.Status != "published" || event.PublishedAt == nil || len(publisher.events) != 1 || publisher.events[0].ID != event.ID {
		t.Fatalf("unexpected outbox state: event=%#v published=%#v", event, publisher.events)
	}
}

func TestOutboxProcessorMovesRepeatedFailureToFailed(t *testing.T) {
	db := newWorkerTestDB(t)
	event := outboxEvent{AggregateType: "notice", AggregateID: "notice-1", EventType: "notice.distributed.v1", Payload: []byte(`{}`), Status: "pending", AvailableAt: time.Now().UTC(), AttemptCount: 4}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	err := NewOutboxProcessor(db, &fakeEventPublisher{err: errors.New("redis unavailable")}, "platform_events").ProcessNext(context.Background())
	if err == nil {
		t.Fatal("expected publish failure")
	}
	if err := db.First(&event, "id = ?", event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if event.Status != "failed" || event.LastErrorCode != "event_publish_failed" || event.AttemptCount != 5 {
		t.Fatalf("unexpected failed outbox state: %#v", event)
	}
}

func TestMailProcessorPrioritizesCriticalAndOnlyMarksAccepted(t *testing.T) {
	db := newWorkerTestDB(t)
	now := time.Now().UTC()
	digest := mailDelivery{Category: "digest", Status: "queued", RecipientHash: "digest-hash", Recipient: "digest@example.edu", TemplateCode: "digest", Subject: "Digest", Body: "Body", RequestID: "req_digest", QueuedAt: now.Add(-time.Hour), Version: 1}
	critical := mailDelivery{Category: "critical", Status: "queued", RecipientHash: "critical-hash", Recipient: "critical@example.edu", TemplateCode: "critical", Subject: "Critical", Body: "Body", RequestID: "req_critical", QueuedAt: now, Version: 1}
	if err := db.Create(&digest).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&critical).Error; err != nil {
		t.Fatal(err)
	}
	provider := &mailprovider.Fake{}
	processor := NewMailProcessor(db, provider)
	if err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&critical, "id = ?", critical.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&digest, "id = ?", digest.ID).Error; err != nil {
		t.Fatal(err)
	}
	if critical.Status != "accepted" || critical.AcceptedAt == nil || critical.DeliveredAt != nil {
		t.Fatalf("SMTP acceptance must not become delivered: %#v", critical)
	}
	if digest.Status != "queued" || len(provider.Messages) != 1 || provider.Messages[0].To != critical.Recipient {
		t.Fatalf("critical queue was not isolated from digest: digest=%s messages=%#v", digest.Status, provider.Messages)
	}
}

func TestMailProcessorCreatesDeadLetterAfterFinalFailure(t *testing.T) {
	db := newWorkerTestDB(t)
	delivery := mailDelivery{Category: "transactional", Status: "queued", RecipientHash: "failure-hash", Recipient: "student@example.edu", TemplateCode: "receipt", Subject: "Receipt", Body: "Body", RequestID: "req_failure", QueuedAt: time.Now().UTC(), AttemptCount: 2, Version: 1}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	processor := NewMailProcessor(db, &mailprovider.Fake{Err: errors.New("raw SMTP credential-bearing error")})
	if err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&delivery, "id = ?", delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "failed" || delivery.LastErrorCode != "provider_error" || delivery.AttemptCount != 3 {
		t.Fatalf("unexpected final failure state: %#v", delivery)
	}
	var deadLetter mailDeadLetter
	if err := db.First(&deadLetter, "delivery_id = ?", delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if deadLetter.ReasonCode != "provider_error" {
		t.Fatalf("provider error must be sanitized: %#v", deadLetter)
	}
}

func TestMailProcessorHonorsSuppressionBeforeProvider(t *testing.T) {
	db := newWorkerTestDB(t)
	delivery := mailDelivery{Category: "critical", Status: "queued", RecipientHash: "suppressed-hash", Recipient: "suppressed@example.edu", TemplateCode: "security", Subject: "Security", Body: "Body", RequestID: "req_suppressed", QueuedAt: time.Now().UTC(), Version: 1}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&mailSuppression{RecipientHash: delivery.RecipientHash, ReasonCode: "user_opt_out"}).Error; err != nil {
		t.Fatal(err)
	}
	provider := &mailprovider.Fake{}
	if err := NewMailProcessor(db, provider).ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&delivery, "id = ?", delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "suppressed" || len(provider.Messages) != 0 {
		t.Fatalf("suppressed recipient reached provider: %#v", delivery)
	}
}

func TestMailProcessorBlocksQueuedNoticeImmediatelyAfterUnsubscribe(t *testing.T) {
	db := newWorkerTestDB(t)
	userID := "5d0ab937-6ca7-4721-82fe-8d259f0de723"
	delivery := mailDelivery{Category: "digest", Status: "queued", RecipientHash: "notice-hash", RecipientUserID: &userID, Recipient: "student@example.edu", TemplateCode: "campus_notice", Subject: "Notice", Body: "Body", RequestID: "req_notice", QueuedAt: time.Now().UTC(), Version: 1}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&noticeEmailSubscription{UserID: userID, Enabled: false, Version: 2}).Error; err != nil {
		t.Fatal(err)
	}
	provider := &mailprovider.Fake{}
	if err := NewMailProcessor(db, provider).ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&delivery, "id = ?", delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "suppressed" || delivery.LastErrorCode != "recipient_unsubscribed" || len(provider.Messages) != 0 {
		t.Fatalf("unsubscribed notice reached provider: %#v", delivery)
	}
}

func TestProcessorCompletesPendingTaskAndCreatesDraft(t *testing.T) {
	db := newWorkerTestDB(t)
	task := aiTask{
		Type:   "wrong_question_analysis",
		Status: taskPending,
		Input:  datatypes.JSON([]byte(`{"wrongQuestionCount":2}`)),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	processor := Processor{db: db, log: slog.Default(), llmMode: "mock"}
	if err := processor.ProcessTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}

	var completed aiTask
	if err := db.First(&completed, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if completed.Status != taskCompleted {
		t.Fatalf("expected completed task, got %s", completed.Status)
	}
	if len(completed.Result) == 0 {
		t.Fatal("expected task result JSON")
	}

	var drafts int64
	if err := db.Model(&aiDraft{}).Where("task_id = ? AND status = ?", task.ID, statusPending).Count(&drafts).Error; err != nil {
		t.Fatal(err)
	}
	if drafts != 1 {
		t.Fatalf("expected one draft, got %d", drafts)
	}

	var usage int64
	if err := db.Model(&aiUsageLog{}).Where("task_id = ?", task.ID).Count(&usage).Error; err != nil {
		t.Fatal(err)
	}
	if usage != 1 {
		t.Fatalf("expected one usage log, got %d", usage)
	}

	if err := processor.ProcessTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&aiDraft{}).Where("task_id = ?", task.ID).Count(&drafts).Error; err != nil {
		t.Fatal(err)
	}
	if drafts != 1 {
		t.Fatalf("expected idempotent processing, got %d drafts", drafts)
	}
}

func TestProcessorProcessesNextPendingTask(t *testing.T) {
	db := newWorkerTestDB(t)
	task := aiTask{Type: "paper_generation", Status: taskPending, Input: datatypes.JSON([]byte(`{}`))}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	processor := Processor{db: db, log: slog.Default(), llmMode: "mock"}
	if err := processor.ProcessNextPending(context.Background()); err != nil {
		t.Fatal(err)
	}

	var completed aiTask
	if err := db.First(&completed, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if completed.Status != taskCompleted {
		t.Fatalf("expected completed task, got %s", completed.Status)
	}
}

func newWorkerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(aiModels()...); err != nil {
		t.Fatal(err)
	}
	return db
}
