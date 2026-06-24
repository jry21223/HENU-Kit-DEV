package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func SecurityHeaders(environment string) gin.HandlerFunc {
	production := strings.EqualFold(strings.TrimSpace(environment), "production")
	return func(ctx *gin.Context) {
		headers := ctx.Writer.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		headers.Set("Cross-Origin-Opener-Policy", "same-origin")
		headers.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		if production {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		ctx.Next()
	}
}
