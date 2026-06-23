package notification

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
)

type ReviewNotificationInput struct {
	NotificationType string
	UserID           string
	ResourceType     string
	ResourceID       string
	ResourceTitle    string
	Status           string
	Reason           string
}

type ReportResultNotificationInput struct {
	UserID     string
	ReportID   string
	TargetType string
	TargetID   string
	Status     string
	Reason     string
}

func CreateReviewNotification(tx *gorm.DB, input ReviewNotificationInput) error {
	input.UserID = strings.TrimSpace(input.UserID)
	if input.UserID == "" {
		return nil
	}
	notificationType := strings.TrimSpace(input.NotificationType)
	if notificationType == "" {
		notificationType = "content_review"
	}
	title, body := reviewNotificationCopy(input)
	data, err := json.Marshal(map[string]string{
		"resourceType": strings.TrimSpace(input.ResourceType),
		"resourceId":   strings.TrimSpace(input.ResourceID),
		"status":       strings.TrimSpace(input.Status),
	})
	if err != nil {
		return err
	}
	return tx.Create(&model.Notification{
		UserID: input.UserID,
		Type:   notificationType,
		Title:  title,
		Body:   body,
		Data:   datatypes.JSON(data),
	}).Error
}

func CreateReportResultNotification(tx *gorm.DB, input ReportResultNotificationInput) error {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return nil
	}
	title := "\u4e3e\u62a5\u5df2\u5904\u7406"
	body := "\u4f60\u63d0\u4ea4\u7684\u4e3e\u62a5\u5df2\u5904\u7406\u3002"
	if input.Status == model.StatusRejected {
		title = "\u4e3e\u62a5\u672a\u91c7\u7eb3"
		body = "\u4f60\u63d0\u4ea4\u7684\u4e3e\u62a5\u672a\u88ab\u91c7\u7eb3\u3002"
	}
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		body += " \u539f\u56e0\uff1a" + reason
	}
	data, err := json.Marshal(map[string]string{
		"reportId":   strings.TrimSpace(input.ReportID),
		"targetType": strings.TrimSpace(input.TargetType),
		"targetId":   strings.TrimSpace(input.TargetID),
		"status":     strings.TrimSpace(input.Status),
	})
	if err != nil {
		return err
	}
	return tx.Create(&model.Notification{
		UserID: userID,
		Type:   "report_result",
		Title:  title,
		Body:   body,
		Data:   datatypes.JSON(data),
	}).Error
}

func reviewNotificationCopy(input ReviewNotificationInput) (string, string) {
	resourceType := strings.TrimSpace(input.ResourceType)
	resourceTitle := strings.TrimSpace(input.ResourceTitle)
	reason := strings.TrimSpace(input.Reason)
	kind := reviewResourceKind(resourceType)
	approved := input.Status != model.StatusRejected

	if resourceType == "forum_reply" && resourceTitle != "" {
		title := "\u8ba8\u8bba\u56de\u590d\u5ba1\u6838\u5df2\u901a\u8fc7"
		body := fmt.Sprintf("\u4f60\u5728\u300c%s\u300d\u4e0b\u7684\u8ba8\u8bba\u56de\u590d\u5df2\u901a\u8fc7\u5ba1\u6838\u3002", resourceTitle)
		if !approved {
			title = "\u8ba8\u8bba\u56de\u590d\u5ba1\u6838\u672a\u901a\u8fc7"
			body = fmt.Sprintf("\u4f60\u5728\u300c%s\u300d\u4e0b\u7684\u8ba8\u8bba\u56de\u590d\u5ba1\u6838\u672a\u901a\u8fc7\u3002", resourceTitle)
		}
		return appendReviewReason(title, body, reason)
	}

	statusCopy := "\u5df2\u901a\u8fc7"
	if !approved {
		statusCopy = "\u672a\u901a\u8fc7"
	}
	title := fmt.Sprintf("%s\u5ba1\u6838%s", kind, statusCopy)
	body := fmt.Sprintf("\u4f60\u7684%s\u5df2\u901a\u8fc7\u5ba1\u6838\u3002", kind)
	if resourceTitle != "" {
		body = fmt.Sprintf("\u4f60\u7684%s\u300c%s\u300d\u5df2\u901a\u8fc7\u5ba1\u6838\u3002", kind, resourceTitle)
	}
	if !approved {
		body = fmt.Sprintf("\u4f60\u7684%s\u5ba1\u6838\u672a\u901a\u8fc7\u3002", kind)
		if resourceTitle != "" {
			body = fmt.Sprintf("\u4f60\u7684%s\u300c%s\u300d\u5ba1\u6838\u672a\u901a\u8fc7\u3002", kind, resourceTitle)
		}
	}
	return appendReviewReason(title, body, reason)
}

func reviewResourceKind(resourceType string) string {
	switch resourceType {
	case "material":
		return "\u8d44\u6599"
	case "wiki_entry":
		return "Wiki \u8bcd\u6761"
	case "wiki_proposal":
		return "Wiki \u7f16\u8f91\u63d0\u6848"
	case "blog_post":
		return "\u535a\u5ba2"
	case "ai_draft":
		return "AI \u8349\u7a3f"
	case "forum_post":
		return "\u8ba8\u8bba\u5e16"
	case "forum_reply":
		return "\u8ba8\u8bba\u56de\u590d"
	default:
		return "\u5185\u5bb9"
	}
}

func appendReviewReason(title string, body string, reason string) (string, string) {
	if reason != "" {
		body += " \u539f\u56e0\uff1a" + reason
	}
	return title, body
}
