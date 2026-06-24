package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	RoleUser       = "user"
	RoleCreator    = "creator"
	RoleReviewer   = "reviewer"
	RoleOperator   = "operator"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"

	StatusDraft        = "draft"
	StatusPending      = "pending"
	StatusApproved     = "approved"
	StatusRejected     = "rejected"
	StatusNeedsChanges = "needs_changes"
	StatusPublished    = "published"
	StatusArchived     = "archived"

	OrderPending   = "pending"
	OrderPaying    = "paying"
	OrderPaid      = "paid"
	OrderClosed    = "closed"
	OrderExpired   = "expired"
	OrderFailed    = "failed"
	OrderCancelled = "cancelled"
	OrderRefunded  = "refunded"

	PaymentIncidentOpen     = "open"
	PaymentIncidentResolved = "resolved"
	PaymentIncidentIgnored  = "ignored"

	MaterialAccessFree          = "free"
	MaterialAccessLoginRequired = "login_required"
	MaterialAccessPaid          = "paid"
	MaterialAccessMemberOnly    = "member_only"

	AITaskPending    = "pending"
	AITaskProcessing = "processing"
	AITaskCompleted  = "completed"
	AITaskFailed     = "failed"
	AITaskReviewed   = "reviewed"
	AITaskPublished  = "published"
	AITaskRejected   = "rejected"
)

type BaseModel struct {
	ID        string         `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (m *BaseModel) BeforeCreate(_ *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return nil
}

type ReviewFields struct {
	Status       string     `json:"status" gorm:"size:32;index"`
	ReviewerID   *string    `json:"reviewerId,omitempty" gorm:"type:uuid;index"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	ReviewReason string     `json:"reviewReason,omitempty" gorm:"size:1000"`
}

type ContentStats struct {
	Visibility   string `json:"visibility" gorm:"size:32;default:public;index"`
	LikeCount    int64  `json:"likeCount" gorm:"default:0"`
	CommentCount int64  `json:"commentCount" gorm:"default:0"`
	CollectCount int64  `json:"collectCount" gorm:"default:0"`
}

type User struct {
	BaseModel
	Email         string     `json:"email" gorm:"size:255;uniqueIndex;not null"`
	Name          string     `json:"name" gorm:"size:80;not null"`
	Role          string     `json:"role" gorm:"size:32;default:user;index"`
	Status        string     `json:"status" gorm:"size:32;default:active;index"`
	SchoolID      *string    `json:"schoolId,omitempty" gorm:"type:uuid;index"`
	MajorID       *string    `json:"majorId,omitempty" gorm:"type:uuid;index"`
	Grade         string     `json:"grade" gorm:"size:32"`
	EmailVerified bool       `json:"emailVerified" gorm:"default:false"`
	FrozenUntil   *time.Time `json:"frozenUntil,omitempty"`
	PointsBalance int64      `json:"pointsBalance" gorm:"default:0"`
}

type EmailVerificationCode struct {
	BaseModel
	Email     string     `json:"email" gorm:"size:255;index;not null"`
	CodeHash  string     `json:"-" gorm:"size:128;not null"`
	Purpose   string     `json:"purpose" gorm:"size:32;default:login;index"`
	ExpiresAt time.Time  `json:"expiresAt" gorm:"index;not null"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
}

type School struct {
	BaseModel
	Name         string `json:"name" gorm:"size:120;not null"`
	Slug         string `json:"slug" gorm:"size:120;uniqueIndex;not null"`
	EmailDomains string `json:"emailDomains" gorm:"size:500"`
	Status       string `json:"status" gorm:"size:32;default:published;index"`
}

type College struct {
	BaseModel
	SchoolID string `json:"schoolId" gorm:"type:uuid;index;not null"`
	Name     string `json:"name" gorm:"size:120;not null"`
	Status   string `json:"status" gorm:"size:32;default:published;index"`
}

type Major struct {
	BaseModel
	SchoolID  string `json:"schoolId" gorm:"type:uuid;index;not null"`
	CollegeID string `json:"collegeId" gorm:"type:uuid;index;not null"`
	Name      string `json:"name" gorm:"size:120;not null"`
	Slug      string `json:"slug" gorm:"size:120;index;not null"`
	Status    string `json:"status" gorm:"size:32;default:published;index"`
}

type Course struct {
	BaseModel
	SchoolID    string `json:"schoolId" gorm:"type:uuid;index;not null"`
	CollegeID   string `json:"collegeId" gorm:"type:uuid;index;not null"`
	MajorID     string `json:"majorId" gorm:"type:uuid;index;not null"`
	Grade       string `json:"grade" gorm:"size:32;index;not null"`
	Name        string `json:"name" gorm:"size:160;not null"`
	Slug        string `json:"slug" gorm:"size:160;index;not null"`
	Description string `json:"description" gorm:"size:1000"`
	ExamScope   string `json:"examScope" gorm:"type:text"`
	Status      string `json:"status" gorm:"size:32;default:published;index"`
}

type Material struct {
	BaseModel
	CourseID       string     `json:"courseId" gorm:"type:uuid;index;not null"`
	Title          string     `json:"title" gorm:"size:200;not null"`
	Type           string     `json:"type" gorm:"size:40;index;not null"`
	Description    string     `json:"description" gorm:"size:1000"`
	StorageKey     string     `json:"-" gorm:"size:500;not null"`
	FileName       string     `json:"fileName" gorm:"size:255"`
	FileSize       int64      `json:"fileSize"`
	PreviewContent string     `json:"previewContent" gorm:"type:text"`
	AccessLevel    string     `json:"accessLevel" gorm:"size:32;default:login_required;index"`
	Status         string     `json:"status" gorm:"size:32;default:draft;index"`
	CreatedBy      *string    `json:"createdBy,omitempty" gorm:"type:uuid;index"`
	ReviewerID     *string    `json:"reviewerId,omitempty" gorm:"type:uuid;index"`
	ReviewedAt     *time.Time `json:"reviewedAt,omitempty"`
	ReviewReason   string     `json:"reviewReason,omitempty" gorm:"size:1000"`
}

type CoursePackage struct {
	BaseModel
	SchoolID    string  `json:"schoolId" gorm:"type:uuid;index;not null"`
	CollegeID   string  `json:"collegeId" gorm:"type:uuid;index;not null"`
	MajorID     string  `json:"majorId" gorm:"type:uuid;index;not null"`
	CourseID    *string `json:"courseId,omitempty" gorm:"type:uuid;index"`
	Grade       string  `json:"grade" gorm:"size:32;index;not null"`
	Title       string  `json:"title" gorm:"size:200;not null"`
	Slug        string  `json:"slug" gorm:"size:220;uniqueIndex;not null"`
	Description string  `json:"description" gorm:"size:1000"`
	PriceFen    int64   `json:"priceFen" gorm:"not null;default:0"`
	Currency    string  `json:"currency" gorm:"size:8;default:CNY"`
	Status      string  `json:"status" gorm:"size:32;default:draft;index"`
}

type CoursePackageItem struct {
	BaseModel
	PackageID    string `json:"packageId" gorm:"type:uuid;uniqueIndex:idx_package_resource;not null"`
	ResourceType string `json:"resourceType" gorm:"size:40;uniqueIndex:idx_package_resource;not null"`
	ResourceID   string `json:"resourceId" gorm:"type:uuid;uniqueIndex:idx_package_resource;not null"`
	SortOrder    int    `json:"sortOrder" gorm:"default:0"`
}

type MaterialAccessGrant struct {
	BaseModel
	UserID     string     `json:"userId" gorm:"type:uuid;index;not null"`
	MaterialID *string    `json:"materialId,omitempty" gorm:"type:uuid;index"`
	PackageID  *string    `json:"packageId,omitempty" gorm:"type:uuid;index"`
	Source     string     `json:"source" gorm:"size:40;index;not null"`
	OrderID    *string    `json:"orderId,omitempty" gorm:"type:uuid;index"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty" gorm:"index"`
}

type MaterialDownloadLog struct {
	BaseModel
	UserID       *string   `json:"userId,omitempty" gorm:"type:uuid;index"`
	MaterialID   string    `json:"materialId" gorm:"type:uuid;index;not null"`
	AccessLevel  string    `json:"accessLevel" gorm:"size:32;index;not null"`
	IP           string    `json:"ip" gorm:"size:80"`
	UserAgent    string    `json:"userAgent" gorm:"size:500"`
	DownloadedAt time.Time `json:"downloadedAt" gorm:"index;not null"`
}

type Order struct {
	BaseModel
	UserID          string         `json:"userId" gorm:"type:uuid;index;not null"`
	ProductType     string         `json:"productType" gorm:"size:40;index;not null"`
	ProductID       string         `json:"productId" gorm:"type:uuid;index;not null"`
	OutTradeNo      string         `json:"outTradeNo" gorm:"size:80;uniqueIndex;not null"`
	PaymentProvider string         `json:"paymentProvider" gorm:"size:40;default:wechat_native;index"`
	Status          string         `json:"status" gorm:"size:32;default:pending;index"`
	AmountTotal     int64          `json:"amountTotal" gorm:"not null"`
	Currency        string         `json:"currency" gorm:"size:8;default:CNY"`
	PaidAt          *time.Time     `json:"paidAt,omitempty"`
	ExpiresAt       *time.Time     `json:"expiresAt,omitempty" gorm:"index"`
	RiskFlag        string         `json:"riskFlag" gorm:"size:120"`
	Metadata        datatypes.JSON `json:"metadata,omitempty"`
}

type PaymentRecord struct {
	BaseModel
	OrderID        string         `json:"orderId" gorm:"type:uuid;index;not null"`
	Provider       string         `json:"provider" gorm:"size:40;index;not null"`
	TransactionID  string         `json:"transactionId" gorm:"size:120;index"`
	TradeState     string         `json:"tradeState" gorm:"size:40;index"`
	AmountTotal    int64          `json:"amountTotal"`
	RawNotify      datatypes.JSON `json:"rawNotify,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey" gorm:"size:160;uniqueIndex"`
	ProcessedAt    *time.Time     `json:"processedAt,omitempty"`
}

type PaymentIncident struct {
	BaseModel
	OrderID        *string        `json:"orderId,omitempty" gorm:"type:uuid;index"`
	Provider       string         `json:"provider" gorm:"size:40;index;not null"`
	IncidentType   string         `json:"incidentType" gorm:"size:80;index;not null"`
	Severity       string         `json:"severity" gorm:"size:32;default:high;index"`
	Status         string         `json:"status" gorm:"size:32;default:open;index"`
	OutTradeNo     string         `json:"outTradeNo" gorm:"size:80;index"`
	TransactionID  string         `json:"transactionId" gorm:"size:120;index"`
	TradeState     string         `json:"tradeState" gorm:"size:40;index"`
	ExpectedAmount int64          `json:"expectedAmount"`
	ActualAmount   int64          `json:"actualAmount"`
	Message        string         `json:"message" gorm:"size:500"`
	RawNotify      datatypes.JSON `json:"rawNotify,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey" gorm:"size:200;uniqueIndex"`
	HandledBy      *string        `json:"handledBy,omitempty" gorm:"type:uuid;index"`
	HandledAt      *time.Time     `json:"handledAt,omitempty"`
	HandleNote     string         `json:"handleNote,omitempty" gorm:"size:1000"`
}

type QuizQuestion struct {
	BaseModel
	CourseID         string  `json:"courseId" gorm:"type:uuid;index;not null"`
	KnowledgePointID *string `json:"knowledgePointId,omitempty" gorm:"type:uuid;index"`
	Type             string  `json:"type" gorm:"size:40;index;not null"`
	Stem             string  `json:"stem" gorm:"type:text;not null"`
	Answer           string  `json:"-" gorm:"type:text;not null"`
	Explanation      string  `json:"explanation" gorm:"type:text"`
	Difficulty       int     `json:"difficulty" gorm:"default:1;index"`
	Status           string  `json:"status" gorm:"size:32;default:draft;index"`
	AuthorID         *string `json:"authorId,omitempty" gorm:"type:uuid;index"`
}

type QuizOption struct {
	BaseModel
	QuestionID string `json:"questionId" gorm:"type:uuid;index;not null"`
	Label      string `json:"label" gorm:"size:16;not null"`
	Content    string `json:"content" gorm:"type:text;not null"`
	SortOrder  int    `json:"sortOrder" gorm:"default:0"`
}

type QuizAttempt struct {
	BaseModel
	UserID     string     `json:"userId" gorm:"type:uuid;index;not null"`
	CourseID   string     `json:"courseId" gorm:"type:uuid;index;not null"`
	Mode       string     `json:"mode" gorm:"size:40;index;default:practice"`
	Score      float64    `json:"score"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type QuizAnswer struct {
	BaseModel
	AttemptID  string  `json:"attemptId" gorm:"type:uuid;index;not null"`
	QuestionID string  `json:"questionId" gorm:"type:uuid;index;not null"`
	UserID     string  `json:"userId" gorm:"type:uuid;index;not null"`
	Answer     string  `json:"answer" gorm:"type:text"`
	IsCorrect  bool    `json:"isCorrect"`
	Score      float64 `json:"score"`
}

type WrongQuestion struct {
	BaseModel
	UserID     string `json:"userId" gorm:"type:uuid;uniqueIndex:idx_wrong_user_question;not null"`
	QuestionID string `json:"questionId" gorm:"type:uuid;uniqueIndex:idx_wrong_user_question;not null"`
	CourseID   string `json:"courseId" gorm:"type:uuid;index;not null"`
	WrongCount int    `json:"wrongCount" gorm:"default:1"`
	LastAnswer string `json:"lastAnswer" gorm:"type:text"`
}

type WeaknessReport struct {
	BaseModel
	UserID   string         `json:"userId" gorm:"type:uuid;index;not null"`
	CourseID *string        `json:"courseId,omitempty" gorm:"type:uuid;index"`
	Summary  string         `json:"summary" gorm:"type:text"`
	Details  datatypes.JSON `json:"details,omitempty"`
	Source   string         `json:"source" gorm:"size:40;default:system"`
}

type WikiEntry struct {
	BaseModel
	ReviewFields
	ContentStats
	AuthorID string  `json:"authorId" gorm:"type:uuid;index;not null"`
	CourseID *string `json:"courseId,omitempty" gorm:"type:uuid;index"`
	Title    string  `json:"title" gorm:"size:200;not null"`
	Slug     string  `json:"slug" gorm:"size:220;uniqueIndex;not null"`
	Content  string  `json:"content" gorm:"type:text"`
	Version  int     `json:"version" gorm:"default:1"`
}

type WikiEditHistory struct {
	BaseModel
	EntryID  string `json:"entryId" gorm:"type:uuid;index;not null"`
	EditorID string `json:"editorId" gorm:"type:uuid;index;not null"`
	Version  int    `json:"version" gorm:"not null"`
	Content  string `json:"content" gorm:"type:text"`
	Summary  string `json:"summary" gorm:"size:500"`
}

type WikiEditProposal struct {
	BaseModel
	ReviewFields
	EntryID         string `json:"entryId" gorm:"type:uuid;index;not null"`
	EditorID        string `json:"editorId" gorm:"type:uuid;index;not null"`
	BaseVersion     int    `json:"baseVersion" gorm:"not null;index"`
	ProposedTitle   string `json:"proposedTitle" gorm:"size:200;not null"`
	ProposedContent string `json:"proposedContent" gorm:"type:text;not null"`
	Summary         string `json:"summary" gorm:"size:500"`
}

type WikiCreatorApplication struct {
	BaseModel
	ReviewFields
	UserID      string `json:"userId" gorm:"type:uuid;index;not null"`
	Reason      string `json:"reason" gorm:"type:text"`
	SampleTitle string `json:"sampleTitle" gorm:"size:200"`
	SampleBody  string `json:"sampleBody" gorm:"type:text"`
}

type BlogPost struct {
	BaseModel
	ReviewFields
	ContentStats
	AuthorID string `json:"authorId" gorm:"type:uuid;index;not null"`
	Title    string `json:"title" gorm:"size:200;not null"`
	Slug     string `json:"slug" gorm:"size:220;uniqueIndex;not null"`
	Content  string `json:"content" gorm:"type:text"`
}

type BlogComment struct {
	BaseModel
	AuthorID string `json:"authorId" gorm:"type:uuid;index;not null"`
	PostID   string `json:"postId" gorm:"type:uuid;index;not null"`
	Content  string `json:"content" gorm:"type:text;not null"`
	Status   string `json:"status" gorm:"size:32;default:published;index"`
}

type ForumBoard struct {
	BaseModel
	Name        string `json:"name" gorm:"size:120;not null"`
	Slug        string `json:"slug" gorm:"size:120;uniqueIndex;not null"`
	Description string `json:"description" gorm:"size:500"`
	Status      string `json:"status" gorm:"size:32;default:published;index"`
}

type ForumPost struct {
	BaseModel
	ContentStats
	AuthorID     string     `json:"authorId" gorm:"type:uuid;index;not null"`
	BoardID      string     `json:"boardId" gorm:"type:uuid;index;not null"`
	Title        string     `json:"title" gorm:"size:200;not null"`
	Content      string     `json:"content" gorm:"type:text"`
	Type         string     `json:"type" gorm:"size:40;default:normal;index"`
	RewardPoints int64      `json:"rewardPoints" gorm:"default:0"`
	RewardStatus string     `json:"rewardStatus" gorm:"size:32;index"`
	Status       string     `json:"status" gorm:"size:32;default:published;index"`
	ReviewerID   *string    `json:"reviewerId,omitempty" gorm:"type:uuid;index"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	ReviewReason string     `json:"reviewReason,omitempty" gorm:"size:1000"`
}

type ForumReply struct {
	BaseModel
	AuthorID     string     `json:"authorId" gorm:"type:uuid;index;not null"`
	PostID       string     `json:"postId" gorm:"type:uuid;index;not null"`
	Content      string     `json:"content" gorm:"type:text;not null"`
	IsBest       bool       `json:"isBest" gorm:"default:false"`
	Status       string     `json:"status" gorm:"size:32;default:published;index"`
	ReviewerID   *string    `json:"reviewerId,omitempty" gorm:"type:uuid;index"`
	ReviewedAt   *time.Time `json:"reviewedAt,omitempty"`
	ReviewReason string     `json:"reviewReason,omitempty" gorm:"size:1000"`
}

type Moment struct {
	BaseModel
	ContentStats
	AuthorID string         `json:"authorId" gorm:"type:uuid;index;not null"`
	Content  string         `json:"content" gorm:"size:500"`
	Images   datatypes.JSON `json:"images,omitempty"`
	Status   string         `json:"status" gorm:"size:32;default:published;index"`
}

type MomentComment struct {
	BaseModel
	AuthorID string `json:"authorId" gorm:"type:uuid;index;not null"`
	MomentID string `json:"momentId" gorm:"type:uuid;index;not null"`
	Content  string `json:"content" gorm:"type:text;not null"`
	Status   string `json:"status" gorm:"size:32;default:published;index"`
}

type UserRelation struct {
	BaseModel
	UserID   string `json:"userId" gorm:"type:uuid;uniqueIndex:idx_user_relation;not null"`
	TargetID string `json:"targetId" gorm:"type:uuid;uniqueIndex:idx_user_relation;not null"`
	Type     string `json:"type" gorm:"size:32;uniqueIndex:idx_user_relation;not null"`
}

type PointsLog struct {
	BaseModel
	UserID         string `json:"userId" gorm:"type:uuid;index;not null"`
	Delta          int64  `json:"delta" gorm:"not null"`
	BalanceAfter   int64  `json:"balanceAfter" gorm:"not null"`
	Reason         string `json:"reason" gorm:"size:120;index"`
	ReferenceType  string `json:"referenceType" gorm:"size:60;index"`
	ReferenceID    string `json:"referenceId" gorm:"size:120;index"`
	IdempotencyKey string `json:"idempotencyKey" gorm:"size:160;uniqueIndex"`
}

type PointsRule struct {
	BaseModel
	Code        string `json:"code" gorm:"size:100;uniqueIndex;not null"`
	Description string `json:"description" gorm:"size:500"`
	Delta       int64  `json:"delta" gorm:"not null"`
	Enabled     bool   `json:"enabled" gorm:"default:true"`
}

type Membership struct {
	BaseModel
	UserID    string     `json:"userId" gorm:"type:uuid;index;not null"`
	PlanCode  string     `json:"planCode" gorm:"size:60;index;not null"`
	Status    string     `json:"status" gorm:"size:32;default:active;index"`
	Source    string     `json:"source" gorm:"size:40;index"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty" gorm:"index"`
}

type MembershipPlan struct {
	BaseModel
	Code     string         `json:"code" gorm:"size:60;uniqueIndex;not null"`
	Name     string         `json:"name" gorm:"size:120;not null"`
	PriceFen int64          `json:"priceFen"`
	Benefits datatypes.JSON `json:"benefits,omitempty"`
	Status   string         `json:"status" gorm:"size:32;default:published;index"`
}

type AITask struct {
	BaseModel
	UserID    *string        `json:"userId,omitempty" gorm:"type:uuid;index"`
	CourseID  *string        `json:"courseId,omitempty" gorm:"type:uuid;index"`
	Type      string         `json:"type" gorm:"size:60;index;not null"`
	Status    string         `json:"status" gorm:"size:32;default:pending;index"`
	Input     datatypes.JSON `json:"input,omitempty"`
	Result    datatypes.JSON `json:"result,omitempty"`
	Error     string         `json:"error" gorm:"type:text"`
	StartedAt *time.Time     `json:"startedAt,omitempty"`
	EndedAt   *time.Time     `json:"endedAt,omitempty"`
}

type AIDraft struct {
	BaseModel
	ReviewFields
	TaskID       string         `json:"taskId" gorm:"type:uuid;index;not null"`
	CourseID     *string        `json:"courseId,omitempty" gorm:"type:uuid;index"`
	OutputType   string         `json:"outputType" gorm:"size:60;index;not null"`
	DraftContent datatypes.JSON `json:"draftContent,omitempty"`
	PublishedID  *string        `json:"publishedId,omitempty" gorm:"type:uuid;index"`
}

type AIUsageLog struct {
	BaseModel
	UserID    *string `json:"userId,omitempty" gorm:"type:uuid;index"`
	TaskID    *string `json:"taskId,omitempty" gorm:"type:uuid;index"`
	Model     string  `json:"model" gorm:"size:120"`
	TokensIn  int64   `json:"tokensIn"`
	TokensOut int64   `json:"tokensOut"`
	CostFen   int64   `json:"costFen"`
}

type Notification struct {
	BaseModel
	UserID string         `json:"userId" gorm:"type:uuid;index;not null"`
	Type   string         `json:"type" gorm:"size:60;index;not null"`
	Title  string         `json:"title" gorm:"size:200;not null"`
	Body   string         `json:"body" gorm:"type:text"`
	Data   datatypes.JSON `json:"data,omitempty"`
	ReadAt *time.Time     `json:"readAt,omitempty"`
}

type Report struct {
	BaseModel
	ReviewFields
	ReporterID  string `json:"reporterId" gorm:"type:uuid;index;not null"`
	TargetType  string `json:"targetType" gorm:"size:60;index;not null"`
	TargetID    string `json:"targetId" gorm:"size:120;index;not null"`
	Reason      string `json:"reason" gorm:"size:500"`
	Description string `json:"description" gorm:"type:text"`
}

type OperationLog struct {
	BaseModel
	OperatorID string         `json:"operatorId" gorm:"type:uuid;index;not null"`
	Action     string         `json:"action" gorm:"size:120;index;not null"`
	TargetType string         `json:"targetType" gorm:"size:60;index"`
	TargetID   string         `json:"targetId" gorm:"size:120;index"`
	IP         string         `json:"ip" gorm:"size:80"`
	UserAgent  string         `json:"userAgent" gorm:"size:500"`
	Metadata   datatypes.JSON `json:"metadata,omitempty"`
}

type LeaderboardSnapshot struct {
	BaseModel
	Type       string         `json:"type" gorm:"size:60;index;not null"`
	Period     string         `json:"period" gorm:"size:40;index;not null"`
	SnapshotAt time.Time      `json:"snapshotAt" gorm:"index;not null"`
	Data       datatypes.JSON `json:"data,omitempty"`
}

type SystemConfig struct {
	BaseModel
	Key         string         `json:"key" gorm:"size:120;uniqueIndex;not null"`
	Value       datatypes.JSON `json:"value,omitempty"`
	Description string         `json:"description" gorm:"size:500"`
	UpdatedBy   *string        `json:"updatedBy,omitempty" gorm:"type:uuid;index"`
}

func AllModels() []interface{} {
	return []interface{}{
		&User{}, &EmailVerificationCode{},
		&School{}, &College{}, &Major{}, &Course{},
		&Material{}, &CoursePackage{}, &CoursePackageItem{}, &MaterialAccessGrant{}, &MaterialDownloadLog{},
		&Order{}, &PaymentRecord{}, &PaymentIncident{},
		&QuizQuestion{}, &QuizOption{}, &QuizAttempt{}, &QuizAnswer{}, &WrongQuestion{}, &WeaknessReport{},
		&WikiEntry{}, &WikiEditHistory{}, &WikiEditProposal{}, &WikiCreatorApplication{},
		&BlogPost{}, &BlogComment{},
		&ForumBoard{}, &ForumPost{}, &ForumReply{},
		&Moment{}, &MomentComment{},
		&UserRelation{},
		&PointsLog{}, &PointsRule{},
		&Membership{}, &MembershipPlan{},
		&AITask{}, &AIDraft{}, &AIUsageLog{},
		&Notification{}, &Report{}, &OperationLog{}, &LeaderboardSnapshot{}, &SystemConfig{},
	}
}
