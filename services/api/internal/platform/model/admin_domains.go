package model

import "time"

const (
	IntegrationNotIntegrated = "not_integrated"
	IntegrationOK            = "ok"
	IntegrationPartial       = "partial"
	IntegrationStale         = "stale"
	IntegrationUnavailable   = "unavailable"

	UrgencyUrgent = "urgent"
	UrgencyNormal = "normal"
)

// CampusNotice is the mutable notice head. Published source content is kept in
// CampusNoticeVersion and is never overwritten.
type CampusNotice struct {
	BaseModel
	SourceID            string     `json:"source_id" gorm:"type:uuid;uniqueIndex:idx_notice_source_external;not null"`
	ExternalID          string     `json:"external_id" gorm:"size:240;uniqueIndex:idx_notice_source_external;not null"`
	OrganizationID      *string    `json:"organization_id,omitempty" gorm:"type:uuid;index"`
	Title               string     `json:"title" gorm:"size:300;not null"`
	OriginalURL         string     `json:"original_url" gorm:"size:1000"`
	OriginalPublishedAt *time.Time `json:"original_published_at,omitempty" gorm:"index"`
	CurrentVersion      int        `json:"current_version" gorm:"not null;default:1"`
	ContentHash         string     `json:"content_hash" gorm:"size:64;not null"`
	Status              string     `json:"status" gorm:"size:40;index;not null;default:review_pending"`
	DistributionStatus  string     `json:"distribution_status" gorm:"size:40;index;not null;default:not_scheduled"`
	Importance          string     `json:"importance" gorm:"size:32;index;not null;default:normal"`
	Audience            []byte     `json:"audience" gorm:"type:jsonb"`
	ReviewReason        string     `json:"review_reason,omitempty" gorm:"size:1000"`
	ReviewedBy          *string    `json:"reviewed_by,omitempty" gorm:"type:uuid;index"`
	ReviewedAt          *time.Time `json:"reviewed_at,omitempty"`
	Version             int        `json:"version" gorm:"not null;default:1"`
}

type CampusNoticeVersion struct {
	BaseModel
	NoticeID    string `json:"notice_id" gorm:"type:uuid;uniqueIndex:idx_notice_version;not null"`
	Version     int    `json:"version" gorm:"uniqueIndex:idx_notice_version;not null"`
	Title       string `json:"title" gorm:"size:300;not null"`
	Body        string `json:"body" gorm:"type:text;not null"`
	ContentHash string `json:"content_hash" gorm:"size:64;not null"`
	ObjectKeys  []byte `json:"object_keys,omitempty" gorm:"type:jsonb"`
}

type NoticeImportJob struct {
	BaseModel
	Status        string `json:"status" gorm:"size:40;index;not null"`
	TotalRows     int    `json:"total_rows"`
	CreatedRows   int    `json:"created_rows"`
	UpdatedRows   int    `json:"updated_rows"`
	DuplicateRows int    `json:"duplicate_rows"`
	FailedRows    int    `json:"failed_rows"`
	RequestedBy   string `json:"requested_by" gorm:"type:uuid;index;not null"`
	ErrorSummary  string `json:"error_summary,omitempty" gorm:"size:1000"`
}

type NoticeEmailSubscription struct {
	BaseModel
	UserID  string `json:"user_id" gorm:"type:uuid;uniqueIndex;not null"`
	Enabled bool   `json:"enabled" gorm:"index;not null;default:false"`
	Version int    `json:"version" gorm:"not null;default:1"`
}

type NoticeDistributionReceipt struct {
	BaseModel
	NoticeID string `json:"notice_id" gorm:"type:uuid;uniqueIndex:idx_notice_distribution;not null"`
	UserID   string `json:"user_id" gorm:"type:uuid;uniqueIndex:idx_notice_distribution;not null"`
	Channel  string `json:"channel" gorm:"size:24;uniqueIndex:idx_notice_distribution;not null"`
}

type MailDelivery struct {
	BaseModel
	EnqueueKey      string     `json:"-" gorm:"size:64;uniqueIndex"`
	RequestHash     string     `json:"-" gorm:"size:64"`
	Category        string     `json:"category" gorm:"size:32;index;not null"`
	Status          string     `json:"status" gorm:"size:40;index;not null"`
	RecipientHash   string     `json:"recipient_hash" gorm:"size:64;index;not null"`
	RecipientUserID *string    `json:"-" gorm:"type:uuid;index"`
	Recipient       string     `json:"-" gorm:"size:320;not null;default:''"`
	TemplateCode    string     `json:"template_code" gorm:"size:120;index;not null"`
	Subject         string     `json:"-" gorm:"size:500;not null;default:''"`
	Body            string     `json:"-" gorm:"type:text"`
	RequestID       string     `json:"request_id" gorm:"size:128;index;not null"`
	AttemptCount    int        `json:"attempt_count" gorm:"not null;default:0"`
	QueuedAt        time.Time  `json:"queued_at" gorm:"index;not null"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
	NextRetryAt     *time.Time `json:"next_retry_at,omitempty" gorm:"index"`
	LockedAt        *time.Time `json:"-" gorm:"index"`
	LockedBy        string     `json:"-" gorm:"size:120"`
	LastErrorCode   string     `json:"last_error_code,omitempty" gorm:"size:120"`
	Version         int        `json:"version" gorm:"not null;default:1"`
}

type MailDeadLetter struct {
	BaseModel
	DeliveryID string     `json:"delivery_id" gorm:"type:uuid;uniqueIndex;not null"`
	Status     string     `json:"status" gorm:"size:40;index;not null;default:open"`
	ReasonCode string     `json:"reason_code" gorm:"size:120;index;not null"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

type MailAttempt struct {
	BaseModel
	DeliveryID string     `json:"delivery_id" gorm:"type:uuid;index;not null"`
	Attempt    int        `json:"attempt" gorm:"not null"`
	Status     string     `json:"status" gorm:"size:40;index;not null"`
	ErrorCode  string     `json:"error_code,omitempty" gorm:"size:120"`
	StartedAt  time.Time  `json:"started_at" gorm:"not null"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
}

type MailSuppression struct {
	BaseModel
	RecipientHash string     `json:"recipient_hash" gorm:"size:64;uniqueIndex;not null"`
	ReasonCode    string     `json:"reason_code" gorm:"size:120;not null"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty" gorm:"index"`
	Version       int        `json:"version" gorm:"not null;default:1"`
}

type PlatformFeedback struct {
	BaseModel
	UserID     *string    `json:"user_id,omitempty" gorm:"type:uuid;index"`
	Category   string     `json:"category" gorm:"size:80;index;not null"`
	Summary    string     `json:"summary" gorm:"size:500;not null"`
	Content    string     `json:"content" gorm:"type:text;not null"`
	Urgency    string     `json:"urgency" gorm:"size:20;index;not null"`
	Status     string     `json:"status" gorm:"size:40;index;not null;default:new"`
	AssigneeID *string    `json:"assignee_id,omitempty" gorm:"type:uuid;index"`
	DueAt      time.Time  `json:"due_at" gorm:"index;not null"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty" gorm:"index"`
	RequestID  string     `json:"request_id" gorm:"size:128;index"`
	Version    int        `json:"version" gorm:"not null;default:1"`
}

type OperationCase struct {
	BaseModel
	SourceService string     `json:"source_service" gorm:"size:80;uniqueIndex:idx_operation_case_source;not null"`
	SourceType    string     `json:"source_type" gorm:"size:80;uniqueIndex:idx_operation_case_source;not null"`
	SourceID      string     `json:"source_id" gorm:"size:160;uniqueIndex:idx_operation_case_source;not null"`
	Summary       string     `json:"summary" gorm:"size:500;not null"`
	Urgency       string     `json:"urgency" gorm:"size:20;index;not null"`
	Status        string     `json:"status" gorm:"size:40;index;not null;default:open"`
	AssigneeID    *string    `json:"assignee_id,omitempty" gorm:"type:uuid;index"`
	DueAt         time.Time  `json:"due_at" gorm:"index;not null"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ActionPath    string     `json:"action_path" gorm:"size:500;not null"`
	Version       int        `json:"version" gorm:"not null;default:1"`
}

type FoodTierDefinition struct {
	BaseModel
	Code      string `json:"code" gorm:"size:60;uniqueIndex;not null"`
	Name      string `json:"name" gorm:"size:80;not null"`
	SortOrder int    `json:"sort_order" gorm:"uniqueIndex;not null"`
	Enabled   bool   `json:"enabled" gorm:"index;not null;default:true"`
}

type FoodSubmission struct {
	BaseModel
	SubmitterID     string `json:"submitter_id" gorm:"type:uuid;index;not null"`
	Name            string `json:"name" gorm:"size:200;not null"`
	Location        string `json:"location" gorm:"size:500;not null"`
	SuggestedTierID string `json:"suggested_tier_id" gorm:"type:uuid;index;not null"`
	Reason          string `json:"reason" gorm:"type:text;not null"`
	ImageObjectKey  string `json:"image_object_key" gorm:"size:500"`
	Status          string `json:"status" gorm:"size:40;index;not null;default:pending"`
	Version         int    `json:"version" gorm:"not null;default:1"`
}

type FoodEntry struct {
	BaseModel
	SubmissionID   string     `json:"submission_id" gorm:"type:uuid;uniqueIndex;not null"`
	Name           string     `json:"name" gorm:"size:200;not null"`
	Location       string     `json:"location" gorm:"size:500;not null"`
	InitialTierID  string     `json:"initial_tier_id" gorm:"type:uuid;index;not null"`
	CurrentTierID  string     `json:"current_tier_id" gorm:"type:uuid;index;not null"`
	LastAdjustedAt *time.Time `json:"last_adjusted_at,omitempty" gorm:"index"`
	Version        int        `json:"version" gorm:"not null;default:1"`
}

type FoodCalibrationRound struct {
	BaseModel
	EntryID       string     `json:"entry_id" gorm:"type:uuid;uniqueIndex:idx_food_entry_round;not null"`
	RoundNumber   int        `json:"round_number" gorm:"uniqueIndex:idx_food_entry_round;not null"`
	Status        string     `json:"status" gorm:"size:40;index;not null;default:open"`
	PolicyVersion string     `json:"policy_version" gorm:"size:80;not null;default:food_calibration_v1"`
	OpenedAt      time.Time  `json:"opened_at" gorm:"index;not null"`
	ClosedAt      *time.Time `json:"closed_at,omitempty"`
}

type FoodCalibrationVote struct {
	BaseModel
	RoundID  string `json:"round_id" gorm:"type:uuid;uniqueIndex:idx_food_round_user;not null"`
	UserID   string `json:"user_id" gorm:"type:uuid;uniqueIndex:idx_food_round_user;not null"`
	Position string `json:"position" gorm:"size:32;index;not null"`
	Status   string `json:"status" gorm:"size:32;index;not null;default:valid"`
}

type FoodVoteAnomaly struct {
	BaseModel
	RoundID  string `json:"round_id" gorm:"type:uuid;index;not null"`
	RuleCode string `json:"rule_code" gorm:"size:120;index;not null"`
	Severity string `json:"severity" gorm:"size:20;index;not null"`
	Status   string `json:"status" gorm:"size:32;index;not null;default:open"`
	Blocking bool   `json:"blocking" gorm:"index;not null;default:false"`
	Evidence []byte `json:"evidence,omitempty" gorm:"type:jsonb"`
	Version  int    `json:"version" gorm:"not null;default:1"`
}

type FoodTierAdjustment struct {
	BaseModel
	EntryID    string    `json:"entry_id" gorm:"type:uuid;index;not null"`
	RoundID    string    `json:"round_id" gorm:"type:uuid;index;not null"`
	FromTierID string    `json:"from_tier_id" gorm:"type:uuid;not null"`
	ToTierID   string    `json:"to_tier_id" gorm:"type:uuid;not null"`
	Direction  string    `json:"direction" gorm:"size:20;not null"`
	ActorID    string    `json:"actor_id" gorm:"type:uuid;index;not null"`
	AdjustedAt time.Time `json:"adjusted_at" gorm:"index;not null"`
}

type ServiceHeartbeat struct {
	BaseModel
	ServiceID       string     `json:"service_id" gorm:"size:100;uniqueIndex;not null"`
	Status          string     `json:"status" gorm:"size:32;index;not null"`
	Version         string     `json:"service_version" gorm:"column:service_version;size:80"`
	CommitSHA       string     `json:"commit_sha" gorm:"size:80"`
	DeploymentTime  *time.Time `json:"deployment_time,omitempty"`
	LastReadyAt     time.Time  `json:"last_ready_at" gorm:"index;not null"`
	OutboxPending   int64      `json:"outbox_pending"`
	WorkerAnomalies int64      `json:"worker_anomalies"`
}

type OutboxEvent struct {
	BaseModel
	AggregateType string     `json:"aggregate_type" gorm:"size:80;index;not null"`
	AggregateID   string     `json:"aggregate_id" gorm:"size:160;index;not null"`
	EventType     string     `json:"event_type" gorm:"size:160;index;not null"`
	Payload       []byte     `json:"payload" gorm:"type:jsonb;not null"`
	Status        string     `json:"status" gorm:"size:32;index;not null;default:pending"`
	AvailableAt   time.Time  `json:"available_at" gorm:"index;not null"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	LockedAt      *time.Time `json:"-" gorm:"index"`
	AttemptCount  int        `json:"attempt_count" gorm:"not null;default:0"`
	LastErrorCode string     `json:"last_error_code,omitempty" gorm:"size:120"`
}

// IdempotencyRecord stores the exact response for a mutating request. The
// actor, route and key form the replay boundary.
type IdempotencyRecord struct {
	BaseModel
	ActorID      string `json:"actor_id" gorm:"type:uuid;uniqueIndex:idx_idempotency_scope;not null"`
	Method       string `json:"method" gorm:"size:16;uniqueIndex:idx_idempotency_scope;not null"`
	Route        string `json:"route" gorm:"size:300;uniqueIndex:idx_idempotency_scope;not null"`
	Key          string `json:"key" gorm:"size:200;uniqueIndex:idx_idempotency_scope;not null"`
	RequestHash  string `json:"request_hash" gorm:"size:64;not null"`
	State        string `json:"state" gorm:"size:24;index;not null"`
	StatusCode   int    `json:"status_code"`
	ResponseBody string `json:"-" gorm:"type:text"`
}
