// Package practice contains the deliberately narrow Portal Gateway boundary
// to QuizCraft. This file is intentionally separate from client.go: catalog
// reads and persisted practice commands use different credentials and have
// different capability and response-safety rules.
package practice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

const anonymousCookieName = "quizcraft_anonymous"

var (
	ErrPracticeCommandBadRequest   = errors.New("QuizCraft rejected the practice command")
	ErrPracticeCommandUnauthorized = errors.New("QuizCraft rejected Portal command authentication")
	ErrPracticeCommandForbidden    = errors.New("QuizCraft denied access to the practice session")
	ErrPracticeCommandNotFound     = errors.New("QuizCraft practice session was not found")
	ErrPracticeCommandConflict     = errors.New("QuizCraft practice command conflicted")
	ErrPracticeCommandUnavailable  = errors.New("QuizCraft practice commands are unavailable")
	ErrPracticeCommandInvalid      = errors.New("QuizCraft returned an invalid practice command response")
)

// UpdateRankingProfilePath mirrors quizcraft.yaml's updateRankingProfile
// command. Unlike the favorites commands it has no /api/v1/portal/... variant
// in the generated contract, so it is declared alongside its only consumer.
const UpdateRankingProfilePath = "/api/v1/ranking-profile"

// CommandClient owns only the two Portal-initiated practice commands. Its
// service credential must never be the catalog/read credential.
type CommandClient struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// CommandResult is an already validated Core response. Raw is the original
// envelope so Gateway can preserve QuizCraft's request id and response shape
// without inventing browser-side facts. AnonymousCookie, when present, is the
// one Core-issued identity cookie that may cross the boundary.
type CommandResult struct {
	Raw             json.RawMessage
	AnonymousCookie *http.Cookie
}

// NewCommandClient creates the default-off write client. The caller is
// responsible for not creating it until PORTAL_PRACTICE_COMMANDS_ENABLED=1.
func NewCommandClient(baseURL, clientID, clientSecret, keyID string) (*CommandClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("invalid QuizCraft Portal command client configuration")
	}
	return &CommandClient{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

func (c *CommandClient) CreateSession(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (CommandResult, error) {
	return c.command(ctx, CreatePortalPracticeSessionPath, actorUserID, requestID, idempotencyKey, raw, anonymousCookie, http.StatusCreated, validatePracticeSessionEnvelope)
}

func (c *CommandClient) SubmitAnswer(ctx context.Context, sessionID, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (CommandResult, error) {
	if !validPracticeCommandUUID(sessionID) {
		return CommandResult{}, ErrPracticeCommandBadRequest
	}
	path := strings.Replace(SubmitPortalPracticeAnswerPath, "{session_id}", url.PathEscape(sessionID), 1)
	return c.command(ctx, path, actorUserID, requestID, idempotencyKey, raw, anonymousCookie, http.StatusOK, validatePracticeAnswerEnvelope)
}

// CreateFeedback submits one signed-in user's correction. Core owns the
// question reference validation, the per-user idempotency history, and the
// 202 write result; Gateway only relays the accepted envelope.
func (c *CommandClient) CreateFeedback(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (CommandResult, error) {
	return c.command(ctx, CreatePortalPracticeFeedbackPath, actorUserID, requestID, idempotencyKey, raw, anonymousCookie, http.StatusAccepted, validatePracticeFeedbackEnvelope)
}

// UpdateRankingProfile applies the signed-in user's public ranking identity.
// Core owns nickname normalization and the per-user idempotency history; the
// Gateway relays the accepted OperationEnvelope unchanged.
func (c *CommandClient) UpdateRankingProfile(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (CommandResult, error) {
	return c.command(ctx, http.MethodPatch, UpdateRankingProfilePath, actorUserID, requestID, idempotencyKey, raw, anonymousCookie, http.StatusOK, validatePracticeOperationEnvelope)
}

// UpdateRankingProfile applies the signed-in user's public ranking identity.
// Core owns nickname normalization and the per-user idempotency history; the
// Gateway relays the accepted OperationEnvelope unchanged.
func (c *CommandClient) UpdateRankingProfile(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (CommandResult, error) {
	return c.command(ctx, http.MethodPatch, UpdateRankingProfilePath, actorUserID, requestID, idempotencyKey, raw, anonymousCookie, http.StatusOK, validatePracticeOperationEnvelope)
}

type commandEnvelopeValidator func([]byte) error

func (c *CommandClient) command(ctx context.Context, path, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie, expectedStatus int, validate commandEnvelopeValidator) (CommandResult, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(requestID) == "" || !ValidIdempotencyKey(idempotencyKey) || len(raw) == 0 || len(raw) > 2<<20 {
		return CommandResult{}, ErrPracticeCommandBadRequest
	}
	actorUserID = strings.TrimSpace(actorUserID)
	if actorUserID != "" && !validPracticeCommandUUID(actorUserID) {
		return CommandResult{}, ErrPracticeCommandUnauthorized
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return CommandResult{}, fmt.Errorf("create QuizCraft Portal command: %w", ErrPracticeCommandUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-Id", requestID)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	if anonymousCookie != nil && anonymousCookie.Name == anonymousCookieName && strings.TrimSpace(anonymousCookie.Value) != "" {
		// Do not copy the browser cookie wholesale. Core is the sole issuer and
		// verifier of this one identity cookie.
		request.AddCookie(&http.Cookie{Name: anonymousCookieName, Value: anonymousCookie.Value})
	}
	if actorUserID == "" {
		if err := c.signer.Sign(request); err != nil {
			return CommandResult{}, fmt.Errorf("sign guest QuizCraft Portal command: %w", ErrPracticeCommandUnavailable)
		}
	} else if err := c.signer.SignWithActor(request, actorUserID); err != nil {
		return CommandResult{}, fmt.Errorf("sign QuizCraft Portal command: %w", ErrPracticeCommandUnavailable)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return CommandResult{}, fmt.Errorf("call QuizCraft Portal command: %w", ErrPracticeCommandUnavailable)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case expectedStatus:
	case http.StatusBadRequest:
		return CommandResult{}, ErrPracticeCommandBadRequest
	case http.StatusUnauthorized:
		return CommandResult{}, ErrPracticeCommandUnauthorized
	case http.StatusForbidden:
		return CommandResult{}, ErrPracticeCommandForbidden
	case http.StatusNotFound:
		return CommandResult{}, ErrPracticeCommandNotFound
	case http.StatusConflict:
		return CommandResult{}, ErrPracticeCommandConflict
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return CommandResult{}, fmt.Errorf("QuizCraft Portal command status %d: %w", response.StatusCode, ErrPracticeCommandUnavailable)
	}
	rawResponse, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	if err != nil || len(rawResponse) == 0 || len(rawResponse) > 2<<20 || validate(rawResponse) != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	cookie, err := permittedAnonymousCookie(response.Header.Values("Set-Cookie"))
	if err != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	return CommandResult{Raw: rawResponse, AnonymousCookie: cookie}, nil
}

// ValidIdempotencyKey is shared by Gateway's public boundary and its Core
// command client so the browser contract cannot drift before signing.
func ValidIdempotencyKey(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 16 && len(value) <= 160
}

func validPracticeCommandUUID(value string) bool {
	return ValidUUID(value)
}

// ValidUUID accepts the canonical UUID text form that QuizCraft Core parses
// for actor and resource identifiers. Keep it exported so Gateway rejects a
// malformed Portal Session as a browser 401 before attempting service auth.
func ValidUUID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func permittedAnonymousCookie(headers []string) (*http.Cookie, error) {
	var allowed *http.Cookie
	for _, header := range headers {
		cookie, err := http.ParseSetCookie(header)
		if err != nil {
			return nil, err
		}
		if cookie.Name != anonymousCookieName {
			// Core has no other browser-cookie capability on this route. Ignore
			// unrelated upstream cookies rather than reflecting them to Portal.
			continue
		}
		if allowed != nil || cookie.Value == "" || cookie.Path != "/" || cookie.Domain != "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			return nil, errors.New("invalid QuizCraft anonymous cookie")
		}
		allowed = cookie
	}
	return allowed, nil
}

func validatePracticeSessionEnvelope(raw []byte) error {
	envelope, valid := practiceRequiredObject(raw)
	if !valid || !practiceOnlyKeys(envelope, "request_id", "data") {
		return errors.New("invalid practice session envelope")
	}
	requestID, valid := practiceRequiredString(envelope, "request_id")
	if !valid || strings.TrimSpace(requestID) == "" {
		return errors.New("invalid practice session request id")
	}
	sessionRaw, valid := practiceRequiredRaw(envelope, "data")
	if !valid {
		return errors.New("invalid practice session data")
	}
	session, valid := practiceRequiredObject(sessionRaw)
	if !valid || !practiceOnlyKeys(session, "session_id", "bank_id", "bank_version_id", "mode", "excluded_unavailable_count", "questions") {
		return errors.New("invalid practice session data")
	}
	sessionID, sessionIDOK := practiceRequiredString(session, "session_id")
	bankID, bankIDOK := practiceRequiredString(session, "bank_id")
	bankVersionID, bankVersionIDOK := practiceRequiredString(session, "bank_version_id")
	mode, modeOK := practiceRequiredString(session, "mode")
	excludedUnavailableCount, excludedUnavailableCountOK := practiceRequiredInt(session, "excluded_unavailable_count")
	questions, questionsOK := practiceRequiredArray(session, "questions")
	if !sessionIDOK || !validPracticeCommandUUID(sessionID) || !bankIDOK || !validPracticeCommandUUID(bankID) || !bankVersionIDOK || !validPracticeCommandUUID(bankVersionID) || !modeOK || !validPracticeSessionMode(mode) || !excludedUnavailableCountOK || excludedUnavailableCount < 0 || !questionsOK {
		return errors.New("invalid practice session data")
	}
	for _, rawQuestion := range questions {
		question, questionOK := practiceRequiredObject(rawQuestion)
		if !questionOK || !practiceOnlyKeys(question, "question_id", "question_version_id", "type", "chapter_id", "chapter", "content", "options") {
			return errors.New("invalid practice question")
		}
		questionID, questionIDOK := practiceRequiredString(question, "question_id")
		questionVersionID, questionVersionIDOK := practiceRequiredString(question, "question_version_id")
		questionType, questionTypeOK := practiceRequiredString(question, "type")
		_, chapterIDOK := practiceRequiredString(question, "chapter_id")
		_, chapterOK := practiceRequiredString(question, "chapter")
		_, contentOK := practiceRequiredString(question, "content")
		if !questionIDOK || !validPracticeCommandUUID(questionID) || !questionVersionIDOK || !validPracticeCommandUUID(questionVersionID) || !questionTypeOK || !validPracticeQuestionType(questionType) || !chapterIDOK || !chapterOK || !contentOK {
			return errors.New("invalid practice question")
		}
		if options, present := question["options"]; present && !practiceStringArray(options) {
			return errors.New("invalid practice question options")
		}
	}
	return nil
}

func validatePracticeFeedbackEnvelope(raw []byte) error {
	envelope, valid := practiceRequiredObject(raw)
	if !valid || !practiceOnlyKeys(envelope, "request_id", "data") {
		return errors.New("invalid practice feedback envelope")
	}
	requestID, valid := practiceRequiredString(envelope, "request_id")
	if !valid || strings.TrimSpace(requestID) == "" {
		return errors.New("invalid practice feedback request id")
	}
	dataRaw, valid := practiceRequiredRaw(envelope, "data")
	if !valid {
		return errors.New("invalid practice feedback data")
	}
	data, valid := practiceRequiredObject(dataRaw)
	if !valid || !practiceOnlyKeys(data, "operation_id", "state", "idempotency_key", "request_id", "resource_id") {
		return errors.New("invalid practice feedback data")
	}
	operationID, operationOK := practiceRequiredString(data, "operation_id")
	state, stateOK := practiceRequiredString(data, "state")
	_, idempotencyOK := practiceRequiredString(data, "idempotency_key")
	_, innerRequestOK := practiceRequiredString(data, "request_id")
	resourceID, resourceOK := practiceRequiredString(data, "resource_id")
	if !operationOK || !validPracticeCommandUUID(operationID) || !stateOK || state != "succeeded" || !idempotencyOK || !innerRequestOK || !resourceOK || !validPracticeCommandUUID(resourceID) {
		return errors.New("invalid practice feedback data")
	}
	return nil
}

func validatePracticeAnswerEnvelope(raw []byte) error {
	envelope, valid := practiceRequiredObject(raw)
	if !valid || !practiceOnlyKeys(envelope, "request_id", "data") {
		return errors.New("invalid practice answer envelope")
	}
	requestID, valid := practiceRequiredString(envelope, "request_id")
	if !valid || strings.TrimSpace(requestID) == "" {
		return errors.New("invalid practice answer request id")
	}
	resultRaw, valid := practiceRequiredRaw(envelope, "data")
	if !valid {
		return errors.New("invalid practice answer data")
	}
	result, valid := practiceRequiredObject(resultRaw)
	if !valid || !practiceOnlyKeys(result, "question_id", "question_version_id", "correct", "replayed", "expected_answer", "analysis") {
		return errors.New("invalid practice answer data")
	}
	questionID, questionIDOK := practiceRequiredString(result, "question_id")
	questionVersionID, questionVersionIDOK := practiceRequiredString(result, "question_version_id")
	_, correctOK := practiceRequiredBool(result, "correct")
	_, replayedOK := practiceRequiredBool(result, "replayed")
	_, expectedAnswerOK := practiceRequiredRaw(result, "expected_answer")
	_, analysisOK := practiceRequiredString(result, "analysis")
	if !questionIDOK || !validPracticeCommandUUID(questionID) || !questionVersionIDOK || !validPracticeCommandUUID(questionVersionID) || !correctOK || !replayedOK || !expectedAnswerOK || !analysisOK {
		return errors.New("invalid practice answer data")
	}
	return nil
}

func validPracticeSessionMode(value string) bool {
	return value == "random" || value == "difficult" || value == "chapter"
}

func validPracticeQuestionType(value string) bool {
	return value == "single" || value == "multi" || value == "judge" || value == "blank"
}

// The Portal contract deliberately closes both envelopes. Decode through raw
// object maps so absent, null, incorrectly typed, and unknown fields can be
// distinguished before a Core response is permitted to reach the browser.
func practiceRequiredObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if isPracticeJSONNull(raw) {
		return nil, false
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, false
	}
	return value, true
}

func practiceOnlyKeys(value map[string]json.RawMessage, allowed ...string) bool {
	for key := range value {
		permitted := false
		for _, candidate := range allowed {
			if key == candidate {
				permitted = true
				break
			}
		}
		if !permitted {
			return false
		}
	}
	return true
}

func practiceRequiredRaw(value map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	raw, ok := value[key]
	return raw, ok && len(raw) != 0
}

func practiceRequiredString(value map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := practiceRequiredRaw(value, key)
	if !ok || isPracticeJSONNull(raw) {
		return "", false
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", false
	}
	return result, true
}

func practiceRequiredInt(value map[string]json.RawMessage, key string) (int64, bool) {
	raw, ok := practiceRequiredRaw(value, key)
	if !ok || isPracticeJSONNull(raw) {
		return 0, false
	}
	var result int64
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, false
	}
	return result, true
}

func practiceRequiredBool(value map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := practiceRequiredRaw(value, key)
	if !ok || isPracticeJSONNull(raw) {
		return false, false
	}
	var result bool
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, false
	}
	return result, true
}

func practiceRequiredArray(value map[string]json.RawMessage, key string) ([]json.RawMessage, bool) {
	raw, ok := practiceRequiredRaw(value, key)
	if !ok || isPracticeJSONNull(raw) {
		return nil, false
	}
	var result []json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		return nil, false
	}
	return result, true
}

func practiceStringArray(raw json.RawMessage) bool {
	if isPracticeJSONNull(raw) {
		return false
	}
	var values []string
	return json.Unmarshal(raw, &values) == nil && values != nil
}

func isPracticeJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}
