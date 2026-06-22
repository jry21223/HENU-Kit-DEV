package worker

import (
	"context"
	"log/slog"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

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
