package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"final-review-platform/services/api/pkg/response"
)

func Recover(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(ctx *gin.Context, recovered interface{}) {
		log.Error("panic recovered", slog.Any("panic", recovered))
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "服务暂时不可用，请稍后再来", nil)
	})
}
