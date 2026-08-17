// Package mcp implements the Food Post MCP server: five tools over Streamable
// HTTP that translate the Food signed contract into MCP tool calls. Food
// remains the sole data and policy owner (ADR-0032); this service adds no
// business rules.
package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"henukit.dev/food-mcp/internal/foodclient"
)

const (
	maxDishes     = 6
	maxImages     = 6
	maxImageBytes = 2 << 20
)

// Handler is the MCP server's HTTP handler.
type Handler struct {
	client      *foodclient.Client
	mcpServer   *mcpsdk.Server
	accessToken string
	httpHandler http.Handler
}

// Options configures the MCP handler.
type Options struct {
	Client      *foodclient.Client
	AccessToken string
}

// NewHandler builds the MCP server, its tools, and the shared Streamable HTTP
// handler. The SDK's StreamableHTTPHandler owns the session registry, so it is
// created exactly once here and reused for every request.
func NewHandler(options Options) (*Handler, error) {
	if options.Client == nil {
		return nil, errors.New("food client is required")
	}
	handler := &Handler{client: options.Client, accessToken: options.AccessToken}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "henukit-food-posts", Version: "1.0.0"}, nil)
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
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "create_food_post", Description: "发布一条校园美食投稿（创建即公开，无审核环节）。必填：venue_name 店铺名、campus（minglun/jinming/longzihu）、tier（hang 夯|top 顶级|elite 人上人|npc NPC|bad 拉完了）、review_text 锐评正文、actor_user_id、actor_display_name。可选：price_reference、hours_reference、dishes（至多 6 道）、images（至多 6 张，单张 ≤2MiB，base64 无前缀，仅 image/jpeg|image/png|image/webp）。同一 actor 每自然日至多 3 条，超限返回 DAILY_POST_CAP_REACHED。"}, h.createPost)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_food_posts", Description: "读取公开美食投稿列表，可选按 campus 过滤。无需登录。"}, h.listPosts)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "get_food_post", Description: "读取单条公开美食投稿详情。"}, h.getPost)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_food_venues", Description: "读取指定校区的场所汇总。campus 必填（minglun/jinming/longzihu）。"}, h.listVenues)
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_my_food_posts", Description: "读取 actor_user_id 自己发布过的全部美食投稿。"}, h.myPosts)
}

type createPostInput struct {
	VenueName      string           `json:"venue_name" jsonschema:"店铺名，1-160 字"`
	Campus         string           `json:"campus" jsonschema:"校区：minglun|jinming|longzihu"`
	Tier           string           `json:"tier" jsonschema:"五档定位：hang(夯)|top(顶级)|elite(人上人)|npc(NPC)|bad(拉完了)"`
	ReviewText     string           `json:"review_text" jsonschema:"锐评正文，2-2000 字"`
	PriceReference string           `json:"price_reference,omitempty" jsonschema:"价格参考（可选）"`
	HoursReference string           `json:"hours_reference,omitempty" jsonschema:"营业参考（可选）"`
	Dishes         []postDishInput  `json:"dishes,omitempty" jsonschema:"推荐菜品，至多 6 道"`
	Images         []postImageInput `json:"images,omitempty" jsonschema:"图片，至多 6 张，单张 ≤2MiB"`
	ActorUserID    string           `json:"actor_user_id" jsonschema:"投稿账号的 UUID"`
	ActorName      string           `json:"actor_display_name" jsonschema:"投稿账号的显示名，1-120 字"`
}

type postDishInput struct {
	Name   string `json:"name"`
	Price  string `json:"price,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type postImageInput struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
}

type listInput struct {
	Campus string `json:"campus,omitempty" jsonschema:"校区过滤（可选）：minglun|jinming|longzihu"`
}

type getPostInput struct {
	PostID string `json:"post_id" jsonschema:"帖子 UUID"`
}

type actorInput struct {
	ActorUserID string `json:"actor_user_id" jsonschema:"投稿账号的 UUID"`
}

func (h *Handler) createPost(ctx context.Context, _ *mcpsdk.CallToolRequest, input *createPostInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil {
		return nil, nil, errors.New("缺少参数")
	}
	if !validUUID(input.ActorUserID) || strings.TrimSpace(input.ActorName) == "" || len([]rune(input.ActorName)) > 120 {
		return nil, nil, errors.New("actor_user_id 必须是合法 UUID，actor_display_name 非空且 ≤120 字")
	}
	if err := validateCreateInput(input); err != nil {
		return nil, nil, err
	}
	images, err := decodeImages(input.Images)
	if err != nil {
		return nil, nil, err
	}
	key := "foodmcp:" + uuid.NewString()
	payload := map[string]any{
		"venue_name": input.VenueName, "campus": input.Campus, "tier": input.Tier, "review_text": input.ReviewText,
		"price_reference": input.PriceReference, "hours_reference": input.HoursReference,
		"dishes": input.Dishes, "images": images,
	}
	data, err := h.client.CreatePost(ctx, input.ActorUserID, strings.TrimSpace(input.ActorName), key, payload)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("投稿已发布（创建即公开）: " + jsonText(data)), nil, nil
}

func validateCreateInput(input *createPostInput) error {
	if strings.TrimSpace(input.VenueName) == "" || len([]rune(input.VenueName)) > 160 {
		return errors.New("venue_name 必填且 ≤160 字")
	}
	if !validCampus(input.Campus) {
		return errors.New("campus 必须是 minglun/jinming/longzihu")
	}
	switch input.Tier {
	case "hang", "top", "elite", "npc", "bad":
	default:
		return errors.New("tier 必须是 hang/top/elite/npc/bad")
	}
	if len([]rune(input.ReviewText)) < 2 || len([]rune(input.ReviewText)) > 2000 {
		return errors.New("review_text 必填且 2-2000 字")
	}
	if len([]rune(input.PriceReference)) > 200 || len([]rune(input.HoursReference)) > 200 {
		return errors.New("price_reference 与 hours_reference 至多 200 字")
	}
	if len(input.Dishes) > maxDishes {
		return fmt.Errorf("dishes 至多 %d 道", maxDishes)
	}
	for _, dish := range input.Dishes {
		if strings.TrimSpace(dish.Name) == "" || len([]rune(dish.Name)) > 80 || len([]rune(dish.Price)) > 40 || len([]rune(dish.Reason)) > 200 {
			return errors.New("菜品 name 必填且 ≤80 字，price ≤40 字，reason ≤200 字")
		}
	}
	if len(input.Images) > maxImages {
		return fmt.Errorf("images 至多 %d 张", maxImages)
	}
	return nil
}

func decodeImages(inputs []postImageInput) ([]map[string]string, error) {
	images := make([]map[string]string, 0, len(inputs))
	for _, item := range inputs {
		switch item.ContentType {
		case "image/jpeg", "image/png", "image/webp":
		default:
			return nil, errors.New("图片 content_type 仅支持 image/jpeg|image/png|image/webp")
		}
		bytes, err := base64.StdEncoding.DecodeString(item.Data)
		if err != nil || len(bytes) == 0 || len(bytes) > maxImageBytes {
			return nil, errors.New("图片 base64 无效或超过 2MiB")
		}
		images = append(images, map[string]string{"content_type": item.ContentType, "data": item.Data})
	}
	return images, nil
}

func validCampus(campus string) bool {
	switch campus {
	case "minglun", "jinming", "longzihu":
		return true
	}
	return false
}

func (h *Handler) listPosts(ctx context.Context, _ *mcpsdk.CallToolRequest, input *listInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil {
		input = &listInput{}
	}
	if input.Campus != "" && !validCampus(input.Campus) {
		return nil, nil, errors.New("campus 必须是 minglun/jinming/longzihu")
	}
	data, err := h.client.ListPosts(ctx, input.Campus)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("公开投稿列表: " + jsonText(data)), nil, nil
}

func (h *Handler) getPost(ctx context.Context, _ *mcpsdk.CallToolRequest, input *getPostInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil || !validUUID(input.PostID) {
		return nil, nil, errors.New("post_id 必须是合法 UUID")
	}
	data, err := h.client.Post(ctx, input.PostID)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("投稿详情: " + jsonText(data)), nil, nil
}

func (h *Handler) listVenues(ctx context.Context, _ *mcpsdk.CallToolRequest, input *listInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil || input.Campus == "" {
		return nil, nil, errors.New("campus 必填（minglun/jinming/longzihu）")
	}
	if !validCampus(input.Campus) {
		return nil, nil, errors.New("campus 必须是 minglun/jinming/longzihu")
	}
	data, err := h.client.Venues(ctx, input.Campus)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("场所汇总: " + jsonText(data)), nil, nil
}

func (h *Handler) myPosts(ctx context.Context, _ *mcpsdk.CallToolRequest, input *actorInput) (*mcpsdk.CallToolResult, any, error) {
	if input == nil || !validUUID(input.ActorUserID) {
		return nil, nil, errors.New("actor_user_id 必须是合法 UUID")
	}
	data, err := h.client.MyPosts(ctx, input.ActorUserID)
	if err != nil {
		return nil, nil, upstreamMessage(err)
	}
	return textResult("我的投稿: " + jsonText(data)), nil, nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func upstreamMessage(err error) error {
	var upstream *foodclient.UpstreamError
	if errors.As(err, &upstream) {
		return fmt.Errorf("Food 返回 %d（%s）：%s", upstream.StatusCode, upstream.Code, upstream.Message)
	}
	if errors.Is(err, foodclient.ErrInvalidResponse) {
		return fmt.Errorf("投稿服务返回了无效响应：%v", err)
	}
	return fmt.Errorf("投稿服务暂时不可用：%v", err)
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
