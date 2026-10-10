package mcp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const actorMetaKey = "henukit.dev/food-actor"

var errActorContext = errors.New("无法验证当前投稿身份，请通过河大助手重试")

type actorClaims struct {
	Tool            string `json:"tool"`
	ArgumentsSHA256 string `json:"arguments_sha256"`
	Scope           string `json:"scope"`
	Kind            string `json:"kind"`
	UserID          string `json:"user_id"`
	DisplayName     string `json:"display_name"`
	IssuedAt        int64  `json:"issued_at"`
	Nonce           string `json:"nonce"`
}

// signedActor verifies host-supplied per-call metadata. It never reads actor
// fields from model arguments or derives shared-user identity from an MCP session.
func (h *Handler) signedActor(request *mcpsdk.CallToolRequest) (postActor, bool, error) {
	if request == nil || request.Params == nil {
		return postActor{}, false, nil
	}
	value, present := request.Params.GetMeta()[actorMetaKey]
	if !present {
		return postActor{}, false, nil
	}
	claims, err := decodeActorProof(value, h.actorSecret)
	if err != nil {
		return postActor{}, true, err
	}
	now := time.Now().Unix()
	scope, err := hex.DecodeString(claims.Scope)
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(claims.Nonce)
	if claims.Tool != request.Params.Name || claims.IssuedAt < now-60 || claims.IssuedAt > now+10 || err != nil || len(scope) != 32 || nonceErr != nil || len(nonce) < 18 || len(nonce) > 48 {
		return postActor{}, true, errActorContext
	}
	var args map[string]any
	if json.Unmarshal(request.Params.Arguments, &args) != nil || args == nil {
		return postActor{}, true, errActorContext
	}
	canonical, err := json.Marshal(args)
	digest := sha256.Sum256(canonical)
	if err != nil || claims.ArgumentsSHA256 != hex.EncodeToString(digest[:]) {
		return postActor{}, true, errActorContext
	}
	actor, err := actorFromClaims(claims, h.actorSecret)
	return actor, true, err
}

func decodeActorProof(value any, secret string) (actorClaims, error) {
	if secret == "" {
		return actorClaims{}, errActorContext
	}
	wire, ok := value.(map[string]any)
	if !ok || len(wire) != 2 {
		return actorClaims{}, errActorContext
	}
	encoded, ok := wire["payload"].(string)
	if !ok || len(encoded) > 5500 {
		return actorClaims{}, errActorContext
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(payload) > 4096 {
		return actorClaims{}, errActorContext
	}
	signature, ok := wire["signature"].(string)
	if !ok {
		return actorClaims{}, errActorContext
	}
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(append([]byte("henukit-food-actor-v1\n"), payload...))
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return actorClaims{}, errActorContext
	}
	var claims actorClaims
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&claims) != nil || decoder.Decode(new(any)) != io.EOF {
		return actorClaims{}, errActorContext
	}
	return claims, nil
}

func actorFromClaims(claims actorClaims, secret string) (postActor, error) {
	actor := postActor{idempotencyKey: "foodmcp:" + claims.Nonce}
	switch claims.Kind {
	case "bound":
		uid, err := uuid.Parse(claims.UserID)
		if err != nil || uid == uuid.Nil || claims.DisplayName == "" || len([]rune(claims.DisplayName)) > 120 || strings.ContainsAny(claims.DisplayName, "\r\n") {
			return postActor{}, errActorContext
		}
		actor.userID, actor.displayName = uid.String(), claims.DisplayName
	case "guest":
		if claims.UserID != "" || claims.DisplayName != "" {
			return postActor{}, errActorContext
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte("henukit-food-guest-v1\n" + claims.Scope))
		actor.userID = uuid.NewSHA1(uuid.NameSpaceURL, mac.Sum(nil)).String()
		actor.displayName = "游客"
		actor.notice = "\n当前使用游客身份，私聊发送“绑定 HENU KIT”可将后续投稿关联到账号；无需提供 UUID。"
	default:
		return postActor{}, errActorContext
	}
	return actor, nil
}
