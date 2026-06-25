package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/admin"
	"final-review-platform/services/api/internal/ai"
	"final-review-platform/services/api/internal/analytics"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/blog"
	"final-review-platform/services/api/internal/course"
	"final-review-platform/services/api/internal/downloadlog"
	"final-review-platform/services/api/internal/entitlement"
	"final-review-platform/services/api/internal/forum"
	"final-review-platform/services/api/internal/health"
	"final-review-platform/services/api/internal/leaderboard"
	"final-review-platform/services/api/internal/material"
	"final-review-platform/services/api/internal/member"
	"final-review-platform/services/api/internal/moment"
	"final-review-platform/services/api/internal/notification"
	"final-review-platform/services/api/internal/order"
	"final-review-platform/services/api/internal/org"
	"final-review-platform/services/api/internal/packagecatalog"
	"final-review-platform/services/api/internal/payment"
	"final-review-platform/services/api/internal/points"
	"final-review-platform/services/api/internal/quiz"
	"final-review-platform/services/api/internal/relation"
	"final-review-platform/services/api/internal/report"
	"final-review-platform/services/api/internal/search"
	"final-review-platform/services/api/internal/user"
	"final-review-platform/services/api/internal/wiki"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/middleware"
	"final-review-platform/services/api/pkg/response"
)

func NewRouter(cfg config.Config, log *slog.Logger, db *gorm.DB, cache *redislib.Client) *gin.Engine {
	if err := config.ValidateHTTPConfig(cfg); err != nil {
		panic(err)
	}
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(middleware.SecurityHeaders(cfg.Environment))
	router.Use(middleware.RequestLogger(log))
	router.Use(middleware.Recover(log))
	router.Use(middleware.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	healthHandler := health.NewHandler(cfg, db, cache)
	leaderboardHandler := leaderboard.NewHandler(db)
	tokenManager, err := auth.NewTokenManager(cfg)
	if err != nil {
		panic(err)
	}
	authHandler := auth.NewHandler(cfg, db, tokenManager)
	authMiddleware := auth.NewMiddleware(db, tokenManager)
	blogHandler := blog.NewHandler(db)
	forumHandler := forum.NewHandler(db)
	orgHandler := org.NewHandler(db)
	courseHandler := course.NewHandler(db)
	materialHandler := material.NewHandler(db, cfg.LocalUploadDir)
	memberHandler := member.NewHandler(db)
	momentHandler := moment.NewHandler(db, cfg.LocalUploadDir)
	downloadLogHandler := downloadlog.NewHandler(db)
	entitlementHandler := entitlement.NewHandler(db)
	notificationHandler := notification.NewHandler(db)
	orderHandler := order.NewHandler(db)
	paymentHandler := payment.NewHandler(db, cfg)
	pointsHandler := points.NewHandler(db)
	packageHandler := packagecatalog.NewHandler(db)
	quizHandler := quiz.NewHandler(db)
	relationHandler := relation.NewHandler(db)
	reportHandler := report.NewHandler(db)
	searchHandler := search.NewHandler(db)
	wikiHandler := wiki.NewHandler(db)
	userHandler := user.NewHandler(db)
	adminHandler := admin.NewHandler(db, cfg.LocalUploadDir, cfg.OperationLogRetentionDays, cfg.OperationLogExportLimit)
	aiHandler := ai.NewHandler(db, cache, cfg.AITaskStream)
	analyticsHandler := analytics.NewHandler(db)
	router.GET("/healthz", healthHandler.Healthz)
	router.GET("/readyz", healthHandler.Readyz)
	v1 := router.Group("/api/v1")
	v1.GET("/healthz", healthHandler.Healthz)
	v1.GET("/readyz", healthHandler.Readyz)
	v1.GET("/version", func(ctx *gin.Context) {
		response.OK(ctx, gin.H{
			"service":     "final-review-api",
			"version":     cfg.Version,
			"environment": cfg.Environment,
		})
	})
	v1.GET("/search", searchHandler.Query)
	v1.GET("/leaderboards/wiki", leaderboardHandler.Wiki)
	v1.GET("/leaderboards/quiz", leaderboardHandler.Quiz)
	v1.GET("/leaderboards/overall", leaderboardHandler.Overall)
	v1.POST("/auth/send-code", authHandler.SendCode)
	v1.POST("/auth/login", authHandler.Login)
	v1.POST("/auth/refresh", authHandler.Refresh)
	v1.POST("/auth/logout", authHandler.Logout)
	v1.GET("/auth/me", authMiddleware.RequireAuth(), authHandler.Me)
	v1.PATCH("/auth/me", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), authHandler.UpdateMe)
	v1.GET("/schools", orgHandler.Schools)
	v1.GET("/colleges", orgHandler.Colleges)
	v1.GET("/majors", orgHandler.Majors)
	v1.GET("/courses", orgHandler.Courses)
	v1.GET("/courses/:id", orgHandler.Course)
	v1.GET("/courses/:id/materials", courseHandler.CourseMaterials)
	v1.GET("/courses/:id/packages", packageHandler.CoursePackages)
	v1.GET("/courses/:id/questions", quizHandler.CourseQuestions)
	v1.GET("/materials", materialHandler.List)
	v1.GET("/materials/:id", materialHandler.Detail)
	v1.GET("/materials/:id/download", authMiddleware.OptionalAuth(), materialHandler.Download)
	v1.GET("/packages", packageHandler.List)
	v1.GET("/packages/:id", packageHandler.Detail)
	v1.GET("/membership/plans", memberHandler.Plans)
	v1.POST("/membership/redeem", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), memberHandler.Redeem)
	v1.POST("/orders", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), orderHandler.Create)
	v1.GET("/orders/:id", authMiddleware.RequireAuth(), orderHandler.Detail)
	v1.GET("/orders/:id/status", authMiddleware.RequireAuth(), orderHandler.Status)
	v1.POST("/payments/wechat/native", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), paymentHandler.WeChatNative)
	v1.POST("/payments/wechat/close", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), paymentHandler.WeChatClose)
	v1.POST("/payments/wechat/notify", paymentHandler.WeChatNotify)
	v1.GET("/questions/:id", quizHandler.Question)
	v1.POST("/questions/:id/submit", authMiddleware.OptionalAuth(), quizHandler.Submit)
	v1.GET("/blog/posts", blogHandler.ListPublished)
	v1.GET("/blog/posts/:id", blogHandler.Detail)
	v1.POST("/blog/posts", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), blogHandler.Create)
	v1.GET("/forum/boards", forumHandler.Boards)
	v1.GET("/forum/posts", forumHandler.ListPublished)
	v1.GET("/forum/posts/:id", forumHandler.Detail)
	v1.POST("/forum/posts", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), forumHandler.Create)
	v1.POST("/forum/posts/:id/replies", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), forumHandler.CreateReply)
	v1.POST("/forum/replies/:id/mark-best", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), forumHandler.MarkBestReply)
	v1.GET("/moments", authMiddleware.OptionalAuth(), momentHandler.List)
	v1.POST("/moments", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), momentHandler.Create)
	v1.POST("/moments/images", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), momentHandler.UploadImage)
	v1.GET("/moments/images/:id", authMiddleware.OptionalAuth(), momentHandler.ServeImage)
	v1.DELETE("/moments/:id", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), momentHandler.Delete)
	v1.POST("/moments/:id/like", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), momentHandler.Like)
	v1.POST("/moments/:id/comments", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), momentHandler.CreateComment)
	v1.DELETE("/moments/comments/:id", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), momentHandler.DeleteComment)
	v1.GET("/wiki/entries", wikiHandler.ListPublished)
	v1.GET("/wiki/entries/:id", wikiHandler.Detail)
	v1.GET("/wiki/creator-applications/me", authMiddleware.RequireAuth(), wikiHandler.MyCreatorApplications)
	v1.POST("/wiki/creator-applications", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), wikiHandler.CreateCreatorApplication)
	v1.POST("/wiki/entries", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), authMiddleware.RequireCreator(), wikiHandler.Create)
	v1.POST("/wiki/entries/:id/proposals", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), authMiddleware.RequireCreator(), wikiHandler.CreateProposal)
	v1.POST("/reports", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), reportHandler.Create)
	v1.POST("/quiz/attempts", authMiddleware.RequireAuth(), quizHandler.CreateAttempt)
	v1.GET("/me/quiz-attempts", authMiddleware.RequireAuth(), quizHandler.MyAttempts)
	v1.GET("/me/wrong-questions", authMiddleware.RequireAuth(), quizHandler.WrongQuestions)
	v1.DELETE("/me/wrong-questions/:id", authMiddleware.RequireAuth(), quizHandler.DeleteWrongQuestion)
	v1.GET("/me/weakness-report", authMiddleware.RequireAuth(), quizHandler.WeaknessReport)
	v1.GET("/me/points", authMiddleware.RequireAuth(), pointsHandler.Me)
	v1.GET("/me/points/logs", authMiddleware.RequireAuth(), pointsHandler.MyLogs)
	v1.GET("/me/membership", authMiddleware.RequireAuth(), memberHandler.Me)
	v1.GET("/me/downloads", authMiddleware.RequireAuth(), downloadLogHandler.MyDownloads)
	v1.GET("/me/entitlements", authMiddleware.RequireAuth(), entitlementHandler.Me)
	v1.GET("/me/wiki-entries", authMiddleware.RequireAuth(), wikiHandler.MyEntries)
	v1.PATCH("/me/wiki-entries/:id", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), wikiHandler.ResubmitEntry)
	v1.GET("/me/wiki-proposals", authMiddleware.RequireAuth(), wikiHandler.MyProposals)
	v1.PATCH("/me/wiki-proposals/:id", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), wikiHandler.ResubmitProposal)
	v1.GET("/me/forum-posts", authMiddleware.RequireAuth(), forumHandler.MyPosts)
	v1.PATCH("/me/forum-posts/:id", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), forumHandler.ResubmitPost)
	v1.GET("/me/forum-replies", authMiddleware.RequireAuth(), forumHandler.MyReplies)
	v1.PATCH("/me/forum-replies/:id", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), forumHandler.ResubmitReply)
	v1.GET("/me/notifications", authMiddleware.RequireAuth(), notificationHandler.MyNotifications)
	v1.POST("/me/notifications/:id/read", authMiddleware.RequireAuth(), notificationHandler.MarkRead)
	v1.POST("/me/notifications/read-all", authMiddleware.RequireAuth(), notificationHandler.MarkAllRead)
	v1.GET("/me/following", authMiddleware.RequireAuth(), relationHandler.Following)
	v1.GET("/me/followers", authMiddleware.RequireAuth(), relationHandler.Followers)
	v1.GET("/me/friends", authMiddleware.RequireAuth(), relationHandler.Friends)
	v1.POST("/users/:id/follow", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), relationHandler.Follow)
	v1.POST("/users/:id/unfollow", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), relationHandler.Unfollow)
	v1.POST("/users/:id/block", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), relationHandler.Block)
	v1.POST("/users/:id/unblock", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), relationHandler.Unblock)
	v1.GET("/users/:id", authMiddleware.OptionalAuth(), userHandler.Profile)
	v1.POST("/ai/tasks", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), aiHandler.CreateTask)
	v1.GET("/ai/tasks/:id", authMiddleware.RequireAuth(), aiHandler.Task)

	admin := v1.Group("/admin")
	admin.Use(authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), authMiddleware.RequireAdmin())
	admin.GET("/healthz", func(ctx *gin.Context) {
		response.OK(ctx, gin.H{"admin": true})
	})
	admin.GET("/users", adminHandler.ListUsers)
	admin.PATCH("/users/:id", adminHandler.UpdateUser)
	admin.GET("/media-assets", adminHandler.ListMediaAssets)
	admin.POST("/media-assets/cleanup", adminHandler.CleanupMediaAssets)
	admin.GET("/points/logs", pointsHandler.AdminLogs)
	admin.GET("/points/rules", pointsHandler.AdminRules)
	admin.POST("/points/rules", pointsHandler.CreateRule)
	admin.PATCH("/points/rules/:id", pointsHandler.UpdateRule)
	admin.GET("/memberships", memberHandler.AdminMemberships)
	admin.POST("/memberships/grant", memberHandler.Grant)
	admin.POST("/memberships/:id/revoke", memberHandler.Revoke)
	admin.GET("/access-grants", adminHandler.ListAccessGrants)
	admin.POST("/access-grants", adminHandler.CreateAccessGrant)
	admin.DELETE("/access-grants/:id", adminHandler.RevokeAccessGrant)
	admin.GET("/orders", adminHandler.ListOrders)
	admin.GET("/payment-reconciliation", adminHandler.ListPaymentReconciliation)
	admin.GET("/payment-incidents/summary", adminHandler.PaymentIncidentSummary)
	admin.GET("/payment-incidents", adminHandler.ListPaymentIncidents)
	admin.POST("/payment-incidents/:id/resolve", adminHandler.ResolvePaymentIncident)
	admin.POST("/schools", adminHandler.CreateSchool)
	admin.PATCH("/schools/:id", adminHandler.UpdateSchool)
	admin.DELETE("/schools/:id", adminHandler.ArchiveSchool)
	admin.POST("/colleges", adminHandler.CreateCollege)
	admin.PATCH("/colleges/:id", adminHandler.UpdateCollege)
	admin.DELETE("/colleges/:id", adminHandler.ArchiveCollege)
	admin.POST("/majors", adminHandler.CreateMajor)
	admin.PATCH("/majors/:id", adminHandler.UpdateMajor)
	admin.DELETE("/majors/:id", adminHandler.ArchiveMajor)
	admin.GET("/courses", adminHandler.ListCourses)
	admin.POST("/courses", adminHandler.CreateCourse)
	admin.PATCH("/courses/:id", adminHandler.UpdateCourse)
	admin.DELETE("/courses/:id", adminHandler.ArchiveCourse)
	admin.GET("/packages", adminHandler.ListCoursePackages)
	admin.POST("/packages", adminHandler.CreateCoursePackage)
	admin.PATCH("/packages/:id", adminHandler.UpdateCoursePackage)
	admin.DELETE("/packages/:id", adminHandler.ArchiveCoursePackage)
	admin.GET("/packages/:id/items", adminHandler.ListCoursePackageItems)
	admin.POST("/packages/:id/items", adminHandler.CreateCoursePackageItem)
	admin.DELETE("/packages/:id/items/:itemId", adminHandler.DeleteCoursePackageItem)
	admin.GET("/materials", adminHandler.ListMaterials)
	admin.POST("/materials", adminHandler.CreateMaterial)
	admin.PATCH("/materials/:id/status", adminHandler.UpdateMaterialStatus)
	admin.PATCH("/materials/:id", adminHandler.UpdateMaterial)
	admin.DELETE("/materials/:id", adminHandler.ArchiveMaterial)
	admin.POST("/materials/upload", adminHandler.UploadMaterial)
	admin.GET("/downloads", downloadLogHandler.AdminDownloads)
	admin.GET("/operation-logs", adminHandler.OperationLogs)
	admin.GET("/operation-logs/export", adminHandler.ExportOperationLogs)
	admin.GET("/operation-logs/retention", adminHandler.OperationLogRetention)
	admin.GET("/analytics/overview", analyticsHandler.Overview)

	review := v1.Group("/admin")
	review.Use(authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), authMiddleware.RequireReviewer())
	review.GET("/ai/tasks", aiHandler.AdminTasks)
	review.GET("/ai/drafts", aiHandler.AdminDrafts)
	review.POST("/ai/drafts/:id/approve", aiHandler.ApproveDraft)
	review.POST("/ai/drafts/:id/reject", aiHandler.RejectDraft)
	review.POST("/ai/drafts/:id/publish", aiHandler.PublishDraft)
	review.GET("/material-reviews", adminHandler.ListMaterialReviews)
	review.POST("/materials/:id/approve", adminHandler.ApproveMaterial)
	review.POST("/materials/:id/reject", adminHandler.RejectMaterial)
	review.GET("/blog/posts", blogHandler.AdminPosts)
	review.POST("/blog/posts/:id/approve", blogHandler.ApprovePost)
	review.POST("/blog/posts/:id/reject", blogHandler.RejectPost)
	review.GET("/forum/posts", forumHandler.AdminPosts)
	review.POST("/forum/posts/:id/approve", forumHandler.ApprovePost)
	review.POST("/forum/posts/:id/reject", forumHandler.RejectPost)
	review.GET("/forum/replies", forumHandler.AdminReplies)
	review.POST("/forum/replies/:id/approve", forumHandler.ApproveReply)
	review.POST("/forum/replies/:id/reject", forumHandler.RejectReply)
	review.GET("/wiki/entries", wikiHandler.AdminEntries)
	review.POST("/wiki/entries/:id/approve", wikiHandler.ApproveEntry)
	review.POST("/wiki/entries/:id/reject", wikiHandler.RejectEntry)
	review.GET("/wiki/creator-applications", wikiHandler.AdminCreatorApplications)
	review.POST("/wiki/creator-applications/:id/approve", wikiHandler.ApproveCreatorApplication)
	review.POST("/wiki/creator-applications/:id/reject", wikiHandler.RejectCreatorApplication)
	review.GET("/wiki/proposals", wikiHandler.AdminProposals)
	review.POST("/wiki/proposals/:id/approve", wikiHandler.ApproveProposal)
	review.POST("/wiki/proposals/:id/reject", wikiHandler.RejectProposal)
	review.GET("/reports", reportHandler.AdminReports)
	review.POST("/reports/:id/resolve", reportHandler.Resolve)
	review.POST("/reports/:id/reject", reportHandler.Reject)

	v1.GET("/protected-example", authMiddleware.RequireAuth(), authMiddleware.RequireNotFrozen(), func(ctx *gin.Context) {
		response.OK(ctx, gin.H{"ok": true})
	})

	router.NoRoute(func(ctx *gin.Context) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "not_found", nil)
	})

	return router
}
