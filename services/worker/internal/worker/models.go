package worker

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	taskPending    = "pending"
	taskProcessing = "processing"
	taskCompleted  = "completed"
	taskFailed     = "failed"
	statusPending  = "pending"
)

type BaseModel struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (m *BaseModel) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return nil
}

type aiTask struct {
	BaseModel
	UserID    *string        `gorm:"type:uuid;index"`
	CourseID  *string        `gorm:"type:uuid;index"`
	Type      string         `gorm:"size:60;index;not null"`
	Status    string         `gorm:"size:32;default:pending;index"`
	Input     datatypes.JSON `gorm:"column:input"`
	Result    datatypes.JSON `gorm:"column:result"`
	Error     string         `gorm:"type:text"`
	StartedAt *time.Time
	EndedAt   *time.Time
}

func (aiTask) TableName() string {
	return "ai_tasks"
}

type aiDraft struct {
	BaseModel
	Status       string  `gorm:"size:32;index"`
	ReviewerID   *string `gorm:"type:uuid;index"`
	ReviewedAt   *time.Time
	ReviewReason string         `gorm:"size:1000"`
	TaskID       string         `gorm:"type:uuid;index;not null"`
	CourseID     *string        `gorm:"type:uuid;index"`
	OutputType   string         `gorm:"size:60;index;not null"`
	DraftContent datatypes.JSON `gorm:"column:draft_content"`
	PublishedID  *string        `gorm:"type:uuid;index"`
}

func (aiDraft) TableName() string {
	return "ai_drafts"
}

type aiUsageLog struct {
	BaseModel
	UserID    *string `gorm:"type:uuid;index"`
	TaskID    *string `gorm:"type:uuid;index"`
	Model     string  `gorm:"size:120"`
	TokensIn  int64
	TokensOut int64
	CostFen   int64
}

func (aiUsageLog) TableName() string {
	return "ai_usage_logs"
}

func aiModels() []interface{} {
	return []interface{}{&aiTask{}, &aiDraft{}, &aiUsageLog{}}
}
