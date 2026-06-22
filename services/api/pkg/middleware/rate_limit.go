package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"final-review-platform/services/api/pkg/response"
)

func RateLimit(rps float64, burst int) gin.HandlerFunc {
	if rps <= 0 {
		rps = 20
	}
	if burst <= 0 {
		burst = 40
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)

	return func(ctx *gin.Context) {
		if !limiter.Allow() {
			response.Error(ctx, http.StatusTooManyRequests, response.CodeBadRequest, "rate_limited", nil)
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}
