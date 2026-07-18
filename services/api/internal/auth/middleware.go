package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const CurrentUserKey = "currentUser"

type Middleware struct {
	db     *gorm.DB
	tokens *TokenManager
}

func NewMiddleware(db *gorm.DB, tokens *TokenManager) Middleware {
	return Middleware{db: db, tokens: tokens}
}

func (m Middleware) OptionalAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tokenText := bearerToken(ctx.GetHeader("Authorization"))
		if tokenText == "" {
			if cookie, err := ctx.Cookie("access_token"); err == nil {
				tokenText = cookie
			}
		}
		if tokenText == "" {
			ctx.Next()
			return
		}

		claims, err := m.tokens.Parse(tokenText, TokenTypeAccess)
		if err != nil {
			ctx.Next()
			return
		}

		var user model.User
		if err := m.db.First(&user, "id = ?", claims.UserID).Error; err == nil && claims.TokenVersion == user.TokenVersion {
			ctx.Set(CurrentUserKey, &user)
		}
		ctx.Next()
	}
}

func (m Middleware) RequireAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tokenText := bearerToken(ctx.GetHeader("Authorization"))
		if tokenText == "" {
			if cookie, err := ctx.Cookie("access_token"); err == nil {
				tokenText = cookie
			}
		}
		if tokenText == "" {
			response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
			ctx.Abort()
			return
		}

		claims, err := m.tokens.Parse(tokenText, TokenTypeAccess)
		if err != nil {
			response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
			ctx.Abort()
			return
		}

		var user model.User
		if err := m.db.First(&user, "id = ?", claims.UserID).Error; err != nil {
			response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
			ctx.Abort()
			return
		}
		if claims.TokenVersion != user.TokenVersion {
			response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "session_revoked", nil)
			ctx.Abort()
			return
		}
		ctx.Set(CurrentUserKey, &user)
		ctx.Next()
	}
}

func (m Middleware) RequireNotFrozen() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		user, ok := CurrentUser(ctx)
		if !ok {
			response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
			ctx.Abort()
			return
		}
		if user.Status == "frozen" || (user.FrozenUntil != nil && user.FrozenUntil.After(time.Now())) {
			response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "user_frozen", nil)
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func (m Middleware) RequireRole(roles ...string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(ctx *gin.Context) {
		user, ok := CurrentUser(ctx)
		if !ok {
			response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
			ctx.Abort()
			return
		}
		if !allowed[user.Role] && user.Role != model.RoleSuperAdmin {
			response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func (m Middleware) RequireAdmin() gin.HandlerFunc {
	return m.RequireRole(model.RoleAdmin, model.RoleSuperAdmin)
}

func (m Middleware) RequireReviewer() gin.HandlerFunc {
	return m.RequireRole(model.RoleReviewer, model.RoleAdmin, model.RoleSuperAdmin)
}

func (m Middleware) RequireCreator() gin.HandlerFunc {
	return m.RequireRole(model.RoleCreator, model.RoleAdmin, model.RoleSuperAdmin)
}

func CurrentUser(ctx *gin.Context) (*model.User, bool) {
	value, ok := ctx.Get(CurrentUserKey)
	if !ok {
		return nil, false
	}
	user, ok := value.(*model.User)
	return user, ok
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
