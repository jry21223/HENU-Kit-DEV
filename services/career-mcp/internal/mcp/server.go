// Package mcp implements the Career resume MCP server: two tools over
// Streamable HTTP that translate the Career resume-extraction contract into
// MCP tool calls. Career remains the sole data and policy owner; this service
// adds no business rules (and deliberately no membership check: the MCP access
// token is the trust root, matching food-mcp).
package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"henukit.dev/career-mcp/internal/careerclient"
)

const maxResumeBytes = 10 << 20

// Handler is the MCP server's HTTP handler.
type Handler struct {
	client      *careerclient.Client
	mcpServer   *mcpsdk.Server
	accessToken string
	httpHandler http.Handler
}

// Options configures the MCP handler.
type Options struct {
	Client      *careerclient.Client
	AccessToken string
}

// NewHandler builds the MCP server, its tools, and the shared Streamable HTTP
// handler. The SDK's StreamableHTTPHandler owns the session registry, so it is
// created exactly once here and reused for every request.
func NewHandler(options Options) (*Handler, error) {
	if options.Client == nil {
		return nil, errors.New("career client is required")
	}
	handler := &Handler{client: options.Client, accessToken: options.AccessToken}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "henukit-career-resume", Version: "1.0.0"}, nil)
	handler.mcpServer = server
	handler.registerTools(server)
	handler.httpHandler = mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	return handler, nil
}

// Handler returns the Streamable HTTP handler for /mcp.
func (h *Handler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.accessToken == "" {
			http.Error(w, "MCP server is not configured", http.StatusServiceUnavailable)
			return
		}
		if !validBearer(r.Header.Get("Authorization"), h.accessToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.httpHandler.ServeHTTP(w, r)
	})
}

func validBearer(header, expected string) bool {
	const prefix = "Bearer "
	return strings.HasPrefix(header, prefix) && strings.TrimSpace(strings.TrimPrefix(header, prefix)) == expected
}

func (h *Handler) registerTools(server *mcpsdk.Server) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "upload_resume", Description: "上传一份简历（PDF/DOCX/TXT，≤10 MiB），后台 AI 自动识别并提取求职画像字段，创建异步识别任务并立即返回任务 id。识别完成后调用 get_resume_extraction 获取提取结果（target_roles/tech_stack/locations/job_type/graduation_year/resume_text 六个字段的草稿）。注意：识别是异步的，通常几十秒；结果只用于预填画像草稿，不会自动保存。同一账号每自然小时至多 5 次。未配置 AI 时返回 AI_UNCONFIGURED。必填：file_name 文件名、content 文件内容（base64 无前缀）、actor_user_id 账号 UUID。"}, h.uploadResume)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "get_resume_extraction", Description: "查询一次简历识别任务的状态与结果。上传后轮询此工具直到 status 为 completed（此时 extracted 字段含画像草稿）或 failed（error_code 说明原因）。必填：extraction_id 任务 UUID（来自 upload_resume）、actor_user_id 账号 UUID。"}, h.extractionStatus)
}

type uploadResumeInput struct {
	FileName    string `json:"file_name" jsonschema:"简历文件名，如 resume.pdf，≤255 字符"`
	Content     string `json:"content" jsonschema:"文件内容，base64 无前缀，解码后 ≤10 MiB"`
	ActorUserID string `json:"actor_user_id" jsonschema:"账号 UUID"`
}

type extractionInput struct {
	ExtractionID string `json:"extraction_id" jsonschema:"识别任务 UUID"`
	ActorUserID  string `json:"actor_user_id" jsonschema:"账号 UUID"`
}

func (h *Handler) uploadResume(ctx context.Context, _ *mcpsdk.CallToolRequest, input *uploadResumeInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil {
		return nil, nil, errors.New("缺少参数")
	}
	if !validUUID(input.ActorUserID) {
		return nil, nil, errors.New("actor_user_id 必须是合法 UUID")
	}
	fileName := strings.TrimSpace(input.FileName)
	if fileName == "" || len(fileName) > 255 || strings.ContainsAny(fileName, `/\`) || fileName != filepath.Base(fileName) {
		return nil, nil, errors.New("file_name 必须是安全的文件名（不含路径分隔符，≤255 字符）")
	}
	content, err := base64.StdEncoding.DecodeString(strings.TrimSpace(input.Content))
	if err != nil || len(content) == 0 {
		return nil, nil, errors.New("content 必须是有效的 base64 且非空")
	}
	if len(content) > maxResumeBytes {
		return nil, nil, errors.New("content 解码后超过 10 MiB 上限")
	}
	data, err := h.client.CreateExtraction(ctx, input.ActorUserID, fileName, content)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("简历提取任务已创建（异步识别中，完成后调用 get_resume_extraction 查询）: " + jsonText(data)), nil, nil
}

func (h *Handler) extractionStatus(ctx context.Context, _ *mcpsdk.CallToolRequest, input *extractionInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil {
		return nil, nil, errors.New("缺少参数")
	}
	if !validUUID(input.ExtractionID) {
		return nil, nil, errors.New("extraction_id 必须是合法 UUID")
	}
	if !validUUID(input.ActorUserID) {
		return nil, nil, errors.New("actor_user_id 必须是合法 UUID")
	}
	data, err := h.client.Extraction(ctx, input.ActorUserID, input.ExtractionID)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("简历提取任务状态: " + jsonText(data)), nil, nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func upstreamMessage(err error) error {
	var upstream *careerclient.UpstreamError
	if errors.As(err, &upstream) {
		return fmt.Errorf("识别服务返回 %d（%s）：%s", upstream.StatusCode, upstream.Code, upstream.Message)
	}
	if errors.Is(err, careerclient.ErrInvalidResponse) {
		return fmt.Errorf("识别服务返回了无效响应：%v", err)
	}
	return fmt.Errorf("识别服务暂时不可用：%v", err)
}

func textResult(text string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}}}
}

func jsonText(data map[string]any) string {
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(encoded)
}
