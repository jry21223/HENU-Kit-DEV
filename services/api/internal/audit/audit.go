package audit

import (
	"encoding/json"
	"errors"
	"net"
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
		log.IP = ipPrefix(ctx.ClientIP())
		log.UserAgent = ctx.Request.UserAgent()
	}
	return db.Create(&log).Error
}

func ipPrefix(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return (&net.IPNet{IP: ipv4.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}).String()
	}
	ipv6 := ip.To16()
	return (&net.IPNet{IP: ipv6.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}).String()
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
