package audit

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
)

var ErrMissingOperator = errors.New("audit operator missing")

func Record(ctx *gin.Context, db *gorm.DB, action string, targetType string, targetID string, metadata map[string]interface{}) error {
	user, ok := auth.CurrentUser(ctx)
	if !ok || user == nil || strings.TrimSpace(user.ID) == "" {
		return ErrMissingOperator
	}
	return RecordForOperator(ctx, db, user.ID, action, targetType, targetID, metadata)
}

func RecordForOperator(ctx *gin.Context, db *gorm.DB, operatorID string, action string, targetType string, targetID string, metadata map[string]interface{}) error {
	operatorID = strings.TrimSpace(operatorID)
	if operatorID == "" {
		return ErrMissingOperator
	}
	log := model.OperationLog{
		OperatorID: operatorID,
		Action:     strings.TrimSpace(action),
		TargetType: strings.TrimSpace(targetType),
		TargetID:   strings.TrimSpace(targetID),
		Metadata:   marshalMetadata(metadata),
	}
	if ctx != nil && ctx.Request != nil {
		log.IP = ctx.ClientIP()
		log.UserAgent = ctx.Request.UserAgent()
	}
	return db.Create(&log).Error
}

func marshalMetadata(metadata map[string]interface{}) datatypes.JSON {
	if len(metadata) == 0 {
		return nil
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil
	}
	return datatypes.JSON(raw)
}
