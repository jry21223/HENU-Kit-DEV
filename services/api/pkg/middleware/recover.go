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
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "internal_server_error", nil)
	})
}
