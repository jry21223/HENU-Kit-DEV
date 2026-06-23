package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

const (
	CodeOK             = 0
	CodeBadRequest     = 40000
	CodeUnauthorized   = 40001
	CodeForbidden      = 40003
	CodeNotFound       = 40004
	CodeConflict       = 40009
	CodeInternalServer = 50000
)

func OK(ctx *gin.Context, data interface{}) {
	ctx.JSON(http.StatusOK, Envelope{
		Code:    CodeOK,
		Message: "ok",
		Data:    data,
	})
}

func Error(ctx *gin.Context, status int, code int, message string, details interface{}) {
	ctx.JSON(status, Envelope{
		Code:    code,
		Message: message,
		Details: details,
	})
}
