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
	return []interface{}{&aiTask{}, &aiDraft{}, &aiUsageLog{}, &mailDelivery{}, &mailAttempt{}, &mailDeadLetter{}, &mailSuppression{}, &noticeEmailSubscription{}, &outboxEvent{}}
}

type mailDelivery struct {
	BaseModel
	EnqueueKey      string
	RequestHash     string
	Category        string  `gorm:"size:32;index;not null"`
	Status          string  `gorm:"size:40;index;not null"`
	RecipientHash   string  `gorm:"size:64;index;not null"`
	RecipientUserID *string `gorm:"type:uuid;index"`
	Recipient       string  `gorm:"size:320;not null"`
	TemplateCode    string  `gorm:"size:120;index;not null"`
	Subject         string  `gorm:"size:500;not null"`
	Body            string  `gorm:"type:text"`
	RequestID       string  `gorm:"size:128;index;not null"`
	AttemptCount    int
	QueuedAt        time.Time
	AcceptedAt      *time.Time
	DeliveredAt     *time.Time
	NextRetryAt     *time.Time
	LockedAt        *time.Time
	LockedBy        string
	LastErrorCode   string
	Version         int
}

func (mailDelivery) TableName() string { return "mail_deliveries" }

type mailAttempt struct {
	BaseModel
	DeliveryID string `gorm:"type:uuid;index;not null"`
	Attempt    int
	Status     string
	ErrorCode  string
	StartedAt  time.Time
	EndedAt    *time.Time
}

func (mailAttempt) TableName() string { return "mail_attempts" }

type mailDeadLetter struct {
	BaseModel
	DeliveryID string `gorm:"type:uuid;uniqueIndex;not null"`
	Status     string
	ReasonCode string
	ResolvedAt *time.Time
}

func (mailDeadLetter) TableName() string { return "mail_dead_letters" }

type mailSuppression struct {
	BaseModel
	RecipientHash string `gorm:"size:64;uniqueIndex;not null"`
	ReasonCode    string
	ExpiresAt     *time.Time
	Version       int
}

func (mailSuppression) TableName() string { return "mail_suppressions" }

type noticeEmailSubscription struct {
	BaseModel
	UserID  string
	Enabled bool
	Version int
}

func (noticeEmailSubscription) TableName() string { return "notice_email_subscriptions" }

type outboxEvent struct {
	BaseModel
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte `gorm:"type:json"`
	Status        string
	AvailableAt   time.Time
	PublishedAt   *time.Time
	LockedAt      *time.Time
	AttemptCount  int
	LastErrorCode string
}

func (outboxEvent) TableName() string { return "outbox_events" }

type serviceHeartbeat struct {
	BaseModel
	ServiceID       string `gorm:"size:100;uniqueIndex;not null"`
	Status          string
	Version         string `gorm:"column:service_version"`
	CommitSHA       string
	DeploymentTime  *time.Time
	LastReadyAt     time.Time
	OutboxPending   int64
	WorkerAnomalies int64
}

func (serviceHeartbeat) TableName() string { return "service_heartbeats" }
