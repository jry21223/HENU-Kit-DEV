package practice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"henukit.dev/portal-gateway/internal/serviceauth"
	"henukit.dev/portal-gateway/internal/trainingcontract"
)

const TrainingOwnerPrefix = "/api/v1/portal/practice/training"

// TrainingFailure carries only a closed, validated owner error. Service
// authentication failures become unavailable at the browser boundary.
type TrainingFailure struct {
	Status   int
	Envelope trainingcontract.TrainingErrorEnvelope
}

func (e *TrainingFailure) Error() string { return string(e.Envelope.Error.Code) }

// TrainingRead relays only the exact frozen training reads with the existing
// independent read credential. Catalog has neither an actor nor a cookie.
func (c *Client) TrainingRead(ctx context.Context, suffix, actor, requestID string, cookie *http.Cookie) (CommandResult, error) {
	if c == nil {
		return CommandResult{}, ErrPortalReadUnavailable
	}
	schema := ""
	switch {
	case suffix == "/catalog":
		schema = "TrainingCatalogEnvelope"
		actor = ""
		cookie = nil
	case suffix == "/review-items" || strings.HasPrefix(suffix, "/review-items?"):
		schema = "TrainingReviewItemsEnvelope"
	case strings.HasPrefix(suffix, "/sessions/"):
		id := strings.TrimPrefix(suffix, "/sessions/")
		schema = "TrainingSessionEnvelope"
		if strings.HasSuffix(id, "/review") {
			id = strings.TrimSuffix(id, "/review")
			schema = "TrainingReviewEnvelope"
		}
		if !ValidUUID(id) {
			return CommandResult{}, ErrPracticeCommandBadRequest
		}
	default:
		return CommandResult{}, ErrPracticeCommandBadRequest
	}

	if schema == "TrainingReviewEnvelope" {
		pinned, err := c.TrainingRead(ctx, strings.TrimSuffix(suffix, "/review"), actor, requestID, cookie)
		if err != nil {
			return CommandResult{}, err
		}
		var snapshot trainingcontract.TrainingSessionEnvelope
		if json.Unmarshal(pinned.Raw, &snapshot) != nil {
			return CommandResult{}, ErrPracticeCommandInvalid
		}
		if snapshot.Data.Phase != "completed" {
			return CommandResult{}, &TrainingFailure{Status: 409, Envelope: trainingcontract.TrainingErrorEnvelope{RequestId: requestID, Error: trainingcontract.TrainingError{Code: "UNIT_INCOMPLETE", Message: "请先保存整组答案，再查看解析"}}}
		}
		result, err := trainingRequest(ctx, c.httpClient, c.signer, c.baseURL, http.MethodGet, suffix, actor, requestID, "", nil, cookie, http.StatusOK, schema)
		if err != nil {
			return CommandResult{}, err
		}
		if validatePinnedTrainingReview(snapshot.Data, result.Raw) != nil {
			return CommandResult{}, ErrPracticeCommandInvalid
		}
		return result, nil
	}
	return trainingRequest(ctx, c.httpClient, c.signer, c.baseURL, http.MethodGet, suffix, actor, requestID, "", nil, cookie, http.StatusOK, schema)
}

func (c *CommandClient) TrainingCommand(ctx context.Context, suffix, actor, requestID, key string, raw []byte, cookie *http.Cookie) (CommandResult, error) {
	if c == nil {
		return CommandResult{}, ErrPracticeCommandUnavailable
	}
	requestSchema, responseSchema, status := "", "", http.StatusOK
	if suffix == "/sessions" {
		requestSchema = "TrainingCreateSession"
		responseSchema = "TrainingSessionEnvelope"
		status = http.StatusCreated
	} else {
		if !strings.HasPrefix(suffix, "/sessions/") {
			return CommandResult{}, ErrPracticeCommandBadRequest
		}
		parts := strings.Split(strings.TrimPrefix(suffix, "/sessions/"), "/")
		if len(parts) != 2 || !ValidUUID(parts[0]) {
			return CommandResult{}, ErrPracticeCommandBadRequest
		}
		switch parts[1] {
		case "answers":
			requestSchema = "TrainingAnswerSubmission"
			responseSchema = "TrainingAnswerReceiptEnvelope"
		case "hints":
			requestSchema = "TrainingHintRequest"
			responseSchema = "TrainingHintDisclosureEnvelope"
		case "reveals":
			// Reveals must pass the pinned owner read in TrainingReveal.
			return CommandResult{}, ErrPracticeCommandBadRequest
		default:
			return CommandResult{}, ErrPracticeCommandBadRequest
		}
	}
	if !ValidIdempotencyKey(key) || trainingcontract.Validate(requestSchema, raw) != nil {
		return CommandResult{}, ErrPracticeCommandBadRequest
	}
	result, err := trainingRequest(ctx, c.httpClient, c.signer, c.baseURL, http.MethodPost, suffix, actor, requestID, key, raw, cookie, status, responseSchema)
	if err != nil {
		return CommandResult{}, err
	}
	if requestSchema == "TrainingAnswerSubmission" {
		var input trainingcontract.TrainingAnswerSubmission
		var output trainingcontract.TrainingAnswerReceiptEnvelope
		if json.Unmarshal(raw, &input) != nil || json.Unmarshal(result.Raw, &output) != nil || input.QuestionId != output.Data.QuestionId || input.QuestionVersionId != output.Data.QuestionVersionId || input.UnitVersionId != output.Data.UnitVersionId || input.Answer != output.Data.Answer {
			return CommandResult{}, ErrPracticeCommandInvalid
		}
	}
	if requestSchema == "TrainingCreateSession" {
		var input trainingcontract.TrainingCreateSession
		var output trainingcontract.TrainingSessionEnvelope
		_ = json.Unmarshal(raw, &input)
		_ = json.Unmarshal(result.Raw, &output)
		unitRaw, _ := output.Data.Unit.MarshalJSON()
		var identity struct {
			UnitId        string `json:"unit_id"`
			UnitVersionId string `json:"unit_version_id"`
		}
		_ = json.Unmarshal(unitRaw, &identity)
		if identity.UnitId != input.UnitId.String() || identity.UnitVersionId != input.UnitVersionId.String() {
			return CommandResult{}, ErrPracticeCommandInvalid
		}
	}
	return result, nil
}

// TrainingReveal verifies the committed disclosure against the complete pinned
// owner review. Its response has no session/version identity, so schema checking
// alone cannot establish that the text belongs to this actor's requested unit.
// Both existing command and private-read gates must be on before this is called.
func (c *CommandClient) TrainingReveal(ctx context.Context, reader *Client, suffix, actor, requestID, key string, raw []byte, cookie *http.Cookie) (CommandResult, error) {
	if c == nil || reader == nil {
		return CommandResult{}, ErrPracticeCommandUnavailable
	}
	parts := strings.Split(strings.TrimPrefix(suffix, "/sessions/"), "/")
	if !strings.HasPrefix(suffix, "/sessions/") || len(parts) != 2 || !ValidUUID(parts[0]) || parts[1] != "reveals" || !ValidIdempotencyKey(key) || trainingcontract.Validate("TrainingRevealRequest", raw) != nil {
		return CommandResult{}, ErrPracticeCommandBadRequest
	}
	result, err := trainingRequest(ctx, c.httpClient, c.signer, c.baseURL, http.MethodPost, suffix, actor, requestID, key, raw, cookie, http.StatusOK, "TrainingLayerDisclosureEnvelope")
	if err != nil {
		return CommandResult{}, err
	}
	verificationCookie := cookie
	if result.AnonymousCookie != nil {
		verificationCookie = result.AnonymousCookie
	}
	review, err := reader.TrainingRead(ctx, "/sessions/"+parts[0]+"/review", actor, requestID, verificationCookie)
	if err != nil {
		// The command may already be committed. No unverified text crosses the
		// boundary; recovery/retry uses the same owner and idempotency key.
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	var input trainingcontract.TrainingRevealRequest
	var output trainingcontract.TrainingLayerDisclosureEnvelope
	var pinned trainingcontract.TrainingReviewEnvelope
	if json.Unmarshal(raw, &input) != nil || json.Unmarshal(result.Raw, &output) != nil || json.Unmarshal(review.Raw, &pinned) != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	reviewRaw, _ := pinned.Data.MarshalJSON()
	var reviewContext struct {
		UnitVersionID string                                     `json:"unit_version_id"`
		Layers        []trainingcontract.TrainingLayerDisclosure `json:"layers"`
	}
	contentRaw, _ := output.Data.Content.MarshalJSON()
	var content struct {
		Layer string `json:"layer"`
	}
	if json.Unmarshal(reviewRaw, &reviewContext) != nil || json.Unmarshal(contentRaw, &content) != nil || reviewContext.UnitVersionID != input.UnitVersionId.String() || content.Layer != string(input.Layer) {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	var returnedContent any
	if json.Unmarshal(contentRaw, &returnedContent) != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	for _, layer := range reviewContext.Layers {
		if layer.DisclosureId != output.Data.DisclosureId || !layer.DisclosedAt.Equal(output.Data.DisclosedAt) {
			continue
		}
		pinnedRaw, _ := layer.Content.MarshalJSON()
		var pinnedContent any
		if json.Unmarshal(pinnedRaw, &pinnedContent) == nil && reflect.DeepEqual(returnedContent, pinnedContent) {
			return result, nil
		}
	}
	return CommandResult{}, ErrPracticeCommandInvalid
}

func trainingRequest(ctx context.Context, client *http.Client, signer *serviceauth.Signer, base, method, suffix, actor, requestID, key string, raw []byte, cookie *http.Cookie, status int, schema string) (CommandResult, error) {
	if client == nil || signer == nil || actor != "" && !ValidUUID(actor) {
		return CommandResult{}, ErrPracticeCommandUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, base+TrainingOwnerPrefix+suffix, bytes.NewReader(raw))
	if err != nil {
		return CommandResult{}, ErrPracticeCommandUnavailable
	}
	req.Header.Set("X-Request-Id", requestID)
	if method == http.MethodGet {
		req.Header.Set("X-Permission-Code", PortalReadPermission)
		req.Header.Set("X-Scope-Kind", "product")
		req.Header.Set("X-Product-Code", "quizcraft")
	} else {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
	}
	if cookie != nil && cookie.Name == anonymousCookieName {
		req.AddCookie(&http.Cookie{Name: anonymousCookieName, Value: cookie.Value})
	}
	if actor == "" {
		err = signer.Sign(req)
	} else {
		err = signer.SignWithActor(req, actor)
	}
	if err != nil {
		return CommandResult{}, ErrPracticeCommandUnavailable
	}
	response, err := client.Do(req)
	if err != nil {
		return CommandResult{}, ErrPracticeCommandUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20+1))
	contentType, _, typeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || len(body) > 2<<20 || typeErr != nil || contentType != "application/json" {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	if response.StatusCode != status {
		if response.StatusCode == 401 || response.StatusCode == 403 {
			return CommandResult{}, ErrPracticeCommandUnavailable
		}
		switch response.StatusCode {
		case 400, 404, 409, 503:
		default:
			return CommandResult{}, ErrPracticeCommandUnavailable
		}
		if trainingcontract.Validate("TrainingErrorEnvelope", body) != nil {
			return CommandResult{}, ErrPracticeCommandInvalid
		}
		var envelope trainingcontract.TrainingErrorEnvelope
		_ = json.Unmarshal(body, &envelope)
		if response.StatusCode == 404 && envelope.Error.Code != "NOT_FOUND" {
			return CommandResult{}, ErrPracticeCommandInvalid
		}
		return CommandResult{}, &TrainingFailure{Status: response.StatusCode, Envelope: envelope}
	}
	if trainingcontract.Validate(schema, body) != nil || validateTrainingProjection(schema, body, suffix) != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	permitted, err := permittedAnonymousCookie(response.Header.Values("Set-Cookie"))
	if err != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	if method == http.MethodGet && permitted != nil {
		return CommandResult{}, ErrPracticeCommandInvalid
	}
	return CommandResult{Raw: body, AnonymousCookie: permitted}, nil
}

// CanonicalTrainingReviewQuery admits only the two frozen cursor parameters.
func CanonicalTrainingReviewQuery(raw string) (string, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", ErrPracticeCommandBadRequest
	}
	for key, entries := range values {
		if key != "cursor" && key != "page_size" || len(entries) != 1 {
			return "", ErrPracticeCommandBadRequest
		}
	}
	parts := []string{}
	if entries, ok := values["cursor"]; ok {
		v := entries[0]
		if len(v) < 1 || len(v) > 512 {
			return "", ErrPracticeCommandBadRequest
		}
		if _, err := base64.RawURLEncoding.DecodeString(v); err != nil {
			return "", ErrPracticeCommandBadRequest
		}
		parts = append(parts, "cursor="+url.PathEscape(v))
	}
	if entries, ok := values["page_size"]; ok {
		v := entries[0]
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 || strconv.Itoa(n) != v {
			return "", ErrPracticeCommandBadRequest
		}
		parts = append(parts, "page_size="+v)
	}
	return strings.Join(parts, "&"), nil
}

func validateTrainingProjection(schema string, raw []byte, suffix string) error {
	switch schema {
	case "TrainingSessionEnvelope":
		var envelope trainingcontract.TrainingSessionEnvelope
		_ = json.Unmarshal(raw, &envelope)
		s := envelope.Data
		unitRaw, _ := s.Unit.MarshalJSON()
		var kind struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(unitRaw, &kind)
		var qs []trainingcontract.TrainingQuestion
		var version string
		if kind.Kind == "listening" {
			unit, err := s.Unit.AsTrainingListeningUnit()
			if err != nil {
				return err
			}
			qs = unit.Questions
			version = unit.UnitVersionId.String()
			if unit.Media.Url != "/practice-media/"+unit.Media.Sha256+".mp3" || len(s.HintDisclosures) != 0 {
				return errors.New("invalid listening projection")
			}
		} else {
			unit, err := s.Unit.AsTrainingClozeUnit()
			if err != nil {
				return err
			}
			qs = unit.Questions
			version = unit.UnitVersionId.String()
		}
		if s.TotalCount != len(qs) || s.SavedCount != len(s.Receipts) || !trainingPhase(string(s.Phase), s.SavedCount, s.TotalCount) || s.Phase != "completed" && len(s.RevealedLayers) != 0 {
			return errors.New("invalid training counts")
		}
		for i, layer := range s.RevealedLayers {
			expected := "english"
			if i == 1 {
				expected = "chinese"
			}
			if string(layer) != expected {
				return errors.New("invalid recovery language order")
			}
		}
		if strings.HasPrefix(suffix, "/sessions/") && s.SessionId.String() != strings.TrimPrefix(suffix, "/sessions/") {
			return errors.New("foreign training session")
		}
		seen := map[string]bool{}
		positions := map[string]int{}
		for i, q := range qs {
			id := q.QuestionId.String()
			if seen[id] {
				return errors.New("duplicate training membership")
			}
			seen[id] = true
			positions[id] = i
		}
		seen = map[string]bool{}
		previous := -1
		for _, r := range s.Receipts {
			position, ok := positions[r.QuestionId.String()]
			if !ok || seen[r.QuestionId.String()] || position <= previous || r.QuestionVersionId != qs[position].QuestionVersionId || r.UnitVersionId.String() != version || r.SessionId != s.SessionId || r.Answer >= len(qs[position].Options) || r.SavedCount != s.SavedCount || r.TotalCount != s.TotalCount || r.Phase != s.Phase || !r.Replayed || !validTrainingReceipt(r) {
				return errors.New("invalid saved training receipt")
			}
			seen[r.QuestionId.String()] = true
			previous = position
		}
	case "TrainingReviewEnvelope":
		var envelope trainingcontract.TrainingReviewEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		reviewRaw, _ := envelope.Data.MarshalJSON()
		var kind struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(reviewRaw, &kind)
		var sessionID string
		var questions []trainingcontract.TrainingQuestionReview
		var layers []trainingcontract.TrainingLayerDisclosure
		segmentIDs := map[string]bool{}
		if kind.Kind == "listening" {
			review, err := envelope.Data.AsTrainingListeningReview()
			if err != nil {
				return err
			}
			sessionID = review.SessionId.String()
			questions = review.Questions
			layers = review.Layers
			previousEnd := 0
			for _, segment := range review.Segments {
				if segmentIDs[segment.SegmentId] || segment.StartMs < previousEnd || segment.EndMs <= segment.StartMs {
					return errors.New("invalid review sentence ranges")
				}
				segmentIDs[segment.SegmentId] = true
				previousEnd = segment.EndMs
			}
		} else {
			review, err := envelope.Data.AsTrainingClozeReview()
			if err != nil {
				return err
			}
			sessionID = review.SessionId.String()
			questions = review.Questions
			layers = review.Layers
		}
		if sessionID != strings.TrimSuffix(strings.TrimPrefix(suffix, "/sessions/"), "/review") {
			return errors.New("foreign training review")
		}
		seen := map[string]bool{}
		for _, question := range questions {
			key := question.QuestionId.String()
			if seen[key] {
				return errors.New("duplicate review question")
			}
			seen[key] = true
			evidenceRaw, _ := question.Evidence.MarshalJSON()
			var evidenceKind struct {
				Kind string `json:"kind"`
			}
			_ = json.Unmarshal(evidenceRaw, &evidenceKind)
			if evidenceKind.Kind != kind.Kind {
				return errors.New("foreign training evidence kind")
			}
			if kind.Kind == "listening" {
				evidence, err := question.Evidence.AsTrainingListeningEvidence()
				if err != nil {
					return err
				}
				for _, id := range evidence.SegmentIds {
					if !segmentIDs[id] {
						return errors.New("unknown review sentence")
					}
				}
			}
		}
		if err := validateTrainingLayers(layers, kind.Kind, segmentIDs); err != nil {
			return err
		}
	case "TrainingAnswerReceiptEnvelope":
		var envelope trainingcontract.TrainingAnswerReceiptEnvelope
		_ = json.Unmarshal(raw, &envelope)
		if !validTrainingReceipt(envelope.Data) || envelope.Data.SessionId.String() != strings.TrimSuffix(strings.TrimPrefix(suffix, "/sessions/"), "/answers") {
			return errors.New("invalid training outcome")
		}
	}
	return nil
}
func trainingPhase(phase string, saved, total int) bool {
	return saved >= 0 && saved <= total && ((saved == 0 && phase == "pending") || (saved > 0 && saved < total && phase == "partial") || (saved == total && phase == "completed"))
}
func validTrainingReceipt(r trainingcontract.TrainingAnswerReceipt) bool {
	outcome := "review_required"
	review := true
	if r.Correct {
		if r.Assisted {
			outcome = "assisted_completion"
		} else {
			outcome = "independent_correct"
			review = false
		}
	}
	return string(r.Outcome) == outcome && r.ReviewRequired == review && trainingPhase(string(r.Phase), r.SavedCount, r.TotalCount)
}

func validateTrainingLayers(layers []trainingcontract.TrainingLayerDisclosure, kind string, segments map[string]bool) error {
	for i, layer := range layers {
		raw, _ := layer.Content.MarshalJSON()
		var content struct {
			Kind  string `json:"kind"`
			Layer string `json:"layer"`
		}
		if json.Unmarshal(raw, &content) != nil {
			return errors.New("invalid training layer")
		}
		expected := "english"
		if i == 1 {
			expected = "chinese"
		}
		if content.Kind != kind || content.Layer != expected || i > 0 && (layer.DisclosedAt.Before(layers[i-1].DisclosedAt) || layer.DisclosureId == layers[i-1].DisclosureId) {
			return errors.New("invalid training layer order")
		}
		if kind == "listening" {
			text, err := layer.Content.AsTrainingListeningLayer()
			if err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, segment := range text.Segments {
				if !segments[segment.SegmentId] || seen[segment.SegmentId] {
					return errors.New("invalid training layer sentences")
				}
				seen[segment.SegmentId] = true
			}
			if len(seen) != len(segments) {
				return errors.New("incomplete training text layer")
			}
		}
	}
	return nil
}

func validatePinnedTrainingReview(snapshot trainingcontract.TrainingSession, raw []byte) error {
	var envelope trainingcontract.TrainingReviewEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	unitRaw, _ := snapshot.Unit.MarshalJSON()
	reviewRaw, _ := envelope.Data.MarshalJSON()
	var unitKind, reviewKind struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(unitRaw, &unitKind)
	_ = json.Unmarshal(reviewRaw, &reviewKind)
	if unitKind.Kind != reviewKind.Kind {
		return errors.New("review kind differs from pinned unit")
	}
	var questions []trainingcontract.TrainingQuestion
	var reviews []trainingcontract.TrainingQuestionReview
	var layers []trainingcontract.TrainingLayerDisclosure
	var version, reviewVersion string
	if unitKind.Kind == "listening" {
		unit, err := snapshot.Unit.AsTrainingListeningUnit()
		if err != nil {
			return err
		}
		review, err := envelope.Data.AsTrainingListeningReview()
		if err != nil {
			return err
		}
		questions, reviews, layers = unit.Questions, review.Questions, review.Layers
		version, reviewVersion = unit.UnitVersionId.String(), review.UnitVersionId.String()
		for _, segment := range review.Segments {
			if segment.EndMs > unit.Media.DurationMs {
				return errors.New("review sentence exceeds pinned media")
			}
		}
	} else {
		unit, err := snapshot.Unit.AsTrainingClozeUnit()
		if err != nil {
			return err
		}
		review, err := envelope.Data.AsTrainingClozeReview()
		if err != nil {
			return err
		}
		questions, reviews, layers = unit.Questions, review.Questions, review.Layers
		version, reviewVersion = unit.UnitVersionId.String(), review.UnitVersionId.String()
	}
	if version != reviewVersion || len(questions) != len(reviews) || len(layers) != len(snapshot.RevealedLayers) {
		return errors.New("review does not match pinned unit")
	}
	for i, q := range questions {
		review := reviews[i]
		if q.QuestionId != review.QuestionId || q.QuestionVersionId != review.QuestionVersionId || review.ExpectedAnswer < 0 || review.ExpectedAnswer >= len(q.Options) {
			return errors.New("review question differs from pinned member")
		}
		for _, receipt := range snapshot.Receipts {
			if receipt.QuestionId == q.QuestionId && receipt.Correct != (receipt.Answer == review.ExpectedAnswer) {
				return errors.New("review key contradicts saved fact")
			}
		}
	}
	for i, layer := range layers {
		content, _ := layer.Content.MarshalJSON()
		var name struct {
			Layer string `json:"layer"`
		}
		_ = json.Unmarshal(content, &name)
		if name.Layer != string(snapshot.RevealedLayers[i]) {
			return errors.New("review layer has not been disclosed")
		}
	}
	return nil
}
