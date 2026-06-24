package user

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const (
	relationFollow     = "follow"
	relationBlock      = "block"
	relationMomentLike = "moment_like"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type publicProfile struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Role              string `json:"role"`
	Status            string `json:"status"`
	SchoolID          string `json:"schoolId,omitempty"`
	MajorID           string `json:"majorId,omitempty"`
	Grade             string `json:"grade,omitempty"`
	CreatedAt         string `json:"createdAt"`
	FollowingByMe     bool   `json:"followingByMe"`
	FollowsMe         bool   `json:"followsMe"`
	MutualFriend      bool   `json:"mutualFriend"`
	BlockedByMe       bool   `json:"blockedByMe"`
	BlockedMe         bool   `json:"blockedMe"`
	FollowingCount    int64  `json:"followingCount"`
	FollowersCount    int64  `json:"followersCount"`
	MomentsCount      int64  `json:"momentsCount"`
	BlogPostsCount    int64  `json:"blogPostsCount"`
	ForumPostsCount   int64  `json:"forumPostsCount"`
	ForumRepliesCount int64  `json:"forumRepliesCount"`
}

type userSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type momentDTO struct {
	ID             string             `json:"id"`
	AuthorID       string             `json:"authorId"`
	Content        string             `json:"content"`
	Images         []string           `json:"images"`
	Status         string             `json:"status"`
	Visibility     string             `json:"visibility"`
	LikeCount      int64              `json:"likeCount"`
	CommentCount   int64              `json:"commentCount"`
	CollectCount   int64              `json:"collectCount"`
	CreatedAt      string             `json:"createdAt"`
	UpdatedAt      string             `json:"updatedAt"`
	Author         userSummary        `json:"author"`
	LikedByMe      bool               `json:"likedByMe"`
	RecentComments []momentCommentDTO `json:"recentComments,omitempty"`
}

type momentCommentDTO struct {
	ID        string      `json:"id"`
	AuthorID  string      `json:"authorId"`
	MomentID  string      `json:"momentId"`
	Content   string      `json:"content"`
	Status    string      `json:"status"`
	CreatedAt string      `json:"createdAt"`
	UpdatedAt string      `json:"updatedAt"`
	Author    userSummary `json:"author"`
}

type blogPostDTO struct {
	ID           string `json:"id"`
	AuthorID     string `json:"authorId"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	Content      string `json:"content"`
	Visibility   string `json:"visibility"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	CollectCount int64  `json:"collectCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type forumPostDTO struct {
	ID           string `json:"id"`
	AuthorID     string `json:"authorId"`
	BoardID      string `json:"boardId"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Type         string `json:"type"`
	RewardPoints int64  `json:"rewardPoints"`
	RewardStatus string `json:"rewardStatus,omitempty"`
	Visibility   string `json:"visibility"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	CollectCount int64  `json:"collectCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type forumReplyDTO struct {
	ID        string `json:"id"`
	AuthorID  string `json:"authorId"`
	PostID    string `json:"postId"`
	PostTitle string `json:"postTitle"`
	Content   string `json:"content"`
	IsBest    bool   `json:"isBest"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (h Handler) Profile(ctx *gin.Context) {
	currentUser, _ := auth.CurrentUser(ctx)
	limit, ok := parseLimit(ctx.Query("limit"), 10, 30)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var target model.User
	if err := h.db.First(&target, "id = ? AND status = ?", ctx.Param("id"), "active").Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "user_not_found", nil)
		return
	}
	if currentUser != nil && h.hasBlockEitherDirection(currentUser.ID, target.ID) && currentUser.ID != target.ID {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "user_not_found", nil)
		return
	}

	moments := h.visibleMoments(currentUser, target.ID, limit)
	posts := h.blogPosts(target.ID, limit)
	forumPosts := h.forumPosts(target.ID, limit)
	replies := h.forumReplies(target.ID, limit)
	response.OK(ctx, gin.H{
		"profile":      h.profileDTO(currentUser, target),
		"moments":      h.momentDTOs(currentUser, moments),
		"blogPosts":    blogPostDTOs(posts),
		"forumPosts":   forumPostDTOs(forumPosts),
		"forumReplies": h.forumReplyDTOs(replies),
	})
}

func (h Handler) profileDTO(currentUser *model.User, target model.User) publicProfile {
	followingByMe := currentUser != nil && h.hasRelation(currentUser.ID, target.ID, relationFollow)
	followsMe := currentUser != nil && h.hasRelation(target.ID, currentUser.ID, relationFollow)
	blockedByMe := currentUser != nil && h.hasRelation(currentUser.ID, target.ID, relationBlock)
	blockedMe := currentUser != nil && h.hasRelation(target.ID, currentUser.ID, relationBlock)
	profile := publicProfile{
		ID:                target.ID,
		Name:              target.Name,
		Role:              target.Role,
		Status:            target.Status,
		Grade:             target.Grade,
		CreatedAt:         formatTime(target.CreatedAt),
		FollowingByMe:     followingByMe,
		FollowsMe:         followsMe,
		MutualFriend:      followingByMe && followsMe,
		BlockedByMe:       blockedByMe,
		BlockedMe:         blockedMe,
		FollowingCount:    h.relationCount("user_id = ? AND type = ?", target.ID, relationFollow),
		FollowersCount:    h.relationCount("target_id = ? AND type = ?", target.ID, relationFollow),
		MomentsCount:      h.countVisibleMoments(currentUser, target.ID),
		BlogPostsCount:    h.countPublishedBlogPosts(target.ID),
		ForumPostsCount:   h.countPublishedForumPosts(target.ID),
		ForumRepliesCount: h.countPublishedForumReplies(target.ID),
	}
	if target.SchoolID != nil {
		profile.SchoolID = *target.SchoolID
	}
	if target.MajorID != nil {
		profile.MajorID = *target.MajorID
	}
	return profile
}

func (h Handler) visibleMoments(currentUser *model.User, authorID string, limit int) []model.Moment {
	var moments []model.Moment
	if err := h.db.Where("author_id = ? AND status = ?", authorID, model.StatusPublished).
		Order("created_at desc").
		Limit(limit).
		Find(&moments).Error; err != nil {
		return []model.Moment{}
	}
	visible := make([]model.Moment, 0, len(moments))
	for _, item := range moments {
		if h.canViewMoment(currentUser, item) {
			visible = append(visible, item)
		}
	}
	return visible
}

func (h Handler) blogPosts(authorID string, limit int) []model.BlogPost {
	var posts []model.BlogPost
	if err := h.db.Where("author_id = ? AND status = ? AND visibility = ?", authorID, model.StatusPublished, "public").
		Order("created_at desc").
		Limit(limit).
		Find(&posts).Error; err != nil {
		return []model.BlogPost{}
	}
	return posts
}

func (h Handler) forumPosts(authorID string, limit int) []model.ForumPost {
	var posts []model.ForumPost
	if err := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.author_id = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", authorID, model.StatusPublished, "public", model.StatusPublished).
		Order("forum_posts.created_at desc").
		Limit(limit).
		Find(&posts).Error; err != nil {
		return []model.ForumPost{}
	}
	return posts
}

func (h Handler) forumReplies(authorID string, limit int) []model.ForumReply {
	var replies []model.ForumReply
	if err := h.db.Model(&model.ForumReply{}).
		Joins("JOIN forum_posts ON forum_posts.id = forum_replies.post_id").
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_replies.author_id = ? AND forum_replies.status = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", authorID, model.StatusPublished, model.StatusPublished, "public", model.StatusPublished).
		Order("forum_replies.created_at desc").
		Limit(limit).
		Find(&replies).Error; err != nil {
		return []model.ForumReply{}
	}
	return replies
}

func (h Handler) momentDTOs(currentUser *model.User, moments []model.Moment) []momentDTO {
	dtos := make([]momentDTO, 0, len(moments))
	for _, item := range moments {
		dtos = append(dtos, h.momentDTO(currentUser, item))
	}
	return dtos
}

func (h Handler) momentDTO(currentUser *model.User, item model.Moment) momentDTO {
	return momentDTO{
		ID:             item.ID,
		AuthorID:       item.AuthorID,
		Content:        item.Content,
		Images:         decodeImages(item.Images),
		Status:         item.Status,
		Visibility:     item.Visibility,
		LikeCount:      item.LikeCount,
		CommentCount:   item.CommentCount,
		CollectCount:   item.CollectCount,
		CreatedAt:      formatTime(item.CreatedAt),
		UpdatedAt:      formatTime(item.UpdatedAt),
		Author:         h.userSummary(item.AuthorID),
		LikedByMe:      currentUser != nil && h.hasRelation(currentUser.ID, item.ID, relationMomentLike),
		RecentComments: h.recentComments(item.ID),
	}
}

func (h Handler) recentComments(momentID string) []momentCommentDTO {
	var comments []model.MomentComment
	if err := h.db.Where("moment_id = ? AND status = ?", momentID, model.StatusPublished).
		Order("created_at asc").
		Limit(5).
		Find(&comments).Error; err != nil {
		return []momentCommentDTO{}
	}
	dtos := make([]momentCommentDTO, 0, len(comments))
	for _, comment := range comments {
		dtos = append(dtos, momentCommentDTO{
			ID:        comment.ID,
			AuthorID:  comment.AuthorID,
			MomentID:  comment.MomentID,
			Content:   comment.Content,
			Status:    comment.Status,
			CreatedAt: formatTime(comment.CreatedAt),
			UpdatedAt: formatTime(comment.UpdatedAt),
			Author:    h.userSummary(comment.AuthorID),
		})
	}
	return dtos
}

func (h Handler) userSummary(userID string) userSummary {
	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return userSummary{ID: userID}
	}
	return userSummary{ID: user.ID, Name: user.Name, Role: user.Role}
}

func blogPostDTOs(posts []model.BlogPost) []blogPostDTO {
	dtos := make([]blogPostDTO, 0, len(posts))
	for _, post := range posts {
		dtos = append(dtos, blogPostDTO{
			ID:           post.ID,
			AuthorID:     post.AuthorID,
			Title:        post.Title,
			Slug:         post.Slug,
			Content:      post.Content,
			Visibility:   post.Visibility,
			LikeCount:    post.LikeCount,
			CommentCount: post.CommentCount,
			CollectCount: post.CollectCount,
			CreatedAt:    formatTime(post.CreatedAt),
			UpdatedAt:    formatTime(post.UpdatedAt),
		})
	}
	return dtos
}

func forumPostDTOs(posts []model.ForumPost) []forumPostDTO {
	dtos := make([]forumPostDTO, 0, len(posts))
	for _, post := range posts {
		dtos = append(dtos, forumPostDTO{
			ID:           post.ID,
			AuthorID:     post.AuthorID,
			BoardID:      post.BoardID,
			Title:        post.Title,
			Content:      post.Content,
			Type:         post.Type,
			RewardPoints: post.RewardPoints,
			RewardStatus: post.RewardStatus,
			Visibility:   post.Visibility,
			LikeCount:    post.LikeCount,
			CommentCount: post.CommentCount,
			CollectCount: post.CollectCount,
			CreatedAt:    formatTime(post.CreatedAt),
			UpdatedAt:    formatTime(post.UpdatedAt),
		})
	}
	return dtos
}

func (h Handler) forumReplyDTOs(replies []model.ForumReply) []forumReplyDTO {
	postIDs := make([]string, 0, len(replies))
	seen := map[string]struct{}{}
	for _, reply := range replies {
		if _, ok := seen[reply.PostID]; ok {
			continue
		}
		seen[reply.PostID] = struct{}{}
		postIDs = append(postIDs, reply.PostID)
	}
	postsByID := map[string]model.ForumPost{}
	if len(postIDs) > 0 {
		var posts []model.ForumPost
		if err := h.db.Where("id IN ?", postIDs).Find(&posts).Error; err == nil {
			for _, post := range posts {
				postsByID[post.ID] = post
			}
		}
	}
	dtos := make([]forumReplyDTO, 0, len(replies))
	for _, reply := range replies {
		post := postsByID[reply.PostID]
		dtos = append(dtos, forumReplyDTO{
			ID:        reply.ID,
			AuthorID:  reply.AuthorID,
			PostID:    reply.PostID,
			PostTitle: post.Title,
			Content:   reply.Content,
			IsBest:    reply.IsBest,
			CreatedAt: formatTime(reply.CreatedAt),
			UpdatedAt: formatTime(reply.UpdatedAt),
		})
	}
	return dtos
}

func (h Handler) canViewMoment(currentUser *model.User, item model.Moment) bool {
	if item.Status != model.StatusPublished {
		return false
	}
	if item.Visibility == "public" || item.Visibility == "" {
		if currentUser == nil {
			return true
		}
		return !h.hasBlockEitherDirection(currentUser.ID, item.AuthorID)
	}
	if item.Visibility != "mutual_friends" || currentUser == nil {
		return false
	}
	if currentUser.ID == item.AuthorID {
		return true
	}
	return !h.hasBlockEitherDirection(currentUser.ID, item.AuthorID) &&
		h.hasRelation(currentUser.ID, item.AuthorID, relationFollow) &&
		h.hasRelation(item.AuthorID, currentUser.ID, relationFollow)
}

func (h Handler) countVisibleMoments(currentUser *model.User, authorID string) int64 {
	var moments []model.Moment
	if err := h.db.Select("id", "author_id", "status", "visibility").
		Where("author_id = ? AND status = ?", authorID, model.StatusPublished).
		Find(&moments).Error; err != nil {
		return 0
	}
	var count int64
	for _, item := range moments {
		if h.canViewMoment(currentUser, item) {
			count++
		}
	}
	return count
}

func (h Handler) countPublishedBlogPosts(authorID string) int64 {
	var count int64
	h.db.Model(&model.BlogPost{}).Where("author_id = ? AND status = ? AND visibility = ?", authorID, model.StatusPublished, "public").Count(&count)
	return count
}

func (h Handler) countPublishedForumPosts(authorID string) int64 {
	var count int64
	h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.author_id = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", authorID, model.StatusPublished, "public", model.StatusPublished).
		Count(&count)
	return count
}

func (h Handler) countPublishedForumReplies(authorID string) int64 {
	var count int64
	h.db.Model(&model.ForumReply{}).
		Joins("JOIN forum_posts ON forum_posts.id = forum_replies.post_id").
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_replies.author_id = ? AND forum_replies.status = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", authorID, model.StatusPublished, model.StatusPublished, "public", model.StatusPublished).
		Count(&count)
	return count
}

func (h Handler) relationCount(where string, args ...interface{}) int64 {
	var count int64
	h.db.Model(&model.UserRelation{}).Where(where, args...).Count(&count)
	return count
}

func (h Handler) hasRelation(userID string, targetID string, relationType string) bool {
	if userID == "" || targetID == "" {
		return false
	}
	var count int64
	if err := h.db.Model(&model.UserRelation{}).
		Where("user_id = ? AND target_id = ? AND type = ?", userID, targetID, relationType).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (h Handler) hasBlockEitherDirection(leftID string, rightID string) bool {
	if leftID == "" || rightID == "" {
		return false
	}
	var count int64
	if err := h.db.Model(&model.UserRelation{}).
		Where("type = ? AND ((user_id = ? AND target_id = ?) OR (user_id = ? AND target_id = ?))", relationBlock, leftID, rightID, rightID, leftID).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func decodeImages(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var images []string
	if err := json.Unmarshal(raw, &images); err != nil {
		return []string{}
	}
	return images
}

func parseLimit(value string, fallback int, max int) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, false
	}
	if limit > max {
		return max, true
	}
	return limit, true
}

func formatTime(value time.Time) string {
	return value.Format(time.RFC3339)
}
