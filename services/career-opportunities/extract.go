package career

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// Profile field limits mirror the career_profiles table constraints. The
// extractor truncates to these limits so an arbitrarily long model response
// can never overflow the stored profile.
const (
	profileTargetRolesLimit = 500
	profileTechStackLimit   = 1000
	profileLocationsLimit   = 500
	profileResumeTextLimit  = 4000
)

// ExtractedProfile is the structured resume extraction result. It mirrors the
// career profile fields so the browser can prefill the profile form; the user
// reviews and confirms before anything is saved.
type ExtractedProfile struct {
	TargetRoles    string `json:"target_roles"`
	TechStack      string `json:"tech_stack"`
	Locations      string `json:"locations"`
	JobType        string `json:"job_type"`
	GraduationYear *int   `json:"graduation_year"`
	ResumeText     string `json:"resume_text"`
}

// ExtractFunc runs one resume extraction: it reads the uploaded file and
// returns the structured profile fields. This is the decoupling seam for the
// AI provider, in the same spirit as the GetWork Source seam: the service only
// ever calls it with owner-uploaded bytes and a sanitized file name.
type ExtractFunc func(ctx context.Context, fileName string, content []byte) (ExtractedProfile, error)

var (
	// ErrAIUnconfigured marks an extraction that cannot run because no AI
	// provider is configured. It is surfaced as a stable browser-safe code.
	ErrAIUnconfigured = errors.New("career AI extractor is not configured")
	// ErrExtractionFailed marks an extraction that ran but produced no usable
	// profile. The underlying cause is logged, never surfaced to the browser.
	ErrExtractionFailed = errors.New("career resume extraction failed")
	// errExtractionProviderFailed keeps a provider transport/protocol category
	// in server logs while preserving the existing browser-safe failure code.
	errExtractionProviderFailed = fmt.Errorf("%w: extraction provider failed", ErrExtractionFailed)
)

// NewMockExtractor returns the deterministic test/development extractor: it
// turns a plain-text resume into a fixed profile, proving the whole
// upload → extract → prefill path before any real AI provider is configured.
func NewMockExtractor() ExtractFunc {
	return func(ctx context.Context, fileName string, content []byte) (ExtractedProfile, error) {
		text, err := resumeDocumentText(ctx, fileName, content)
		if err != nil {
			return ExtractedProfile{}, err
		}
		year := 2027
		return ExtractedProfile{
			TargetRoles:    "后端开发、数据分析",
			TechStack:      "go、postgres、vue",
			Locations:      "郑州、北京",
			JobType:        "daily_intern",
			GraduationYear: &year,
			ResumeText:     truncateProfileText(text, profileResumeTextLimit),
		}, nil
	}
}

// ExtractConfig builds the OpenAI-compatible extractor. All fields are
// operator-supplied (base URL, API key, model); the user fills in the real
// provider specifics. The empty config is the production-safe off state and
// construction fails rather than pretending to work.
type ExtractConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

const extractorPrompt = `你是简历信息提取助手。从用户上传的简历中提取求职画像字段，只输出一个 JSON 对象，不要输出任何其他文字。

JSON 字段与要求：
- target_roles: 字符串，求职目标岗位/方向，逗号分隔，500 字以内
- tech_stack: 字符串，技术栈/技能关键词，逗号分隔，1000 字以内
- locations: 字符串，期望工作城市，逗号分隔，500 字以内
- job_type: 字符串，只能是 ""（不限）、"daily_intern"（日常实习）、"summer_intern"（暑期实习）、"campus_recruit"（校招）之一
- graduation_year: 整数或 null，毕业年份（4 位数字）
- resume_text: 字符串，从简历提炼的项目、竞赛、实习经历摘要，4000 字以内

信息在简历中找不到时，字符串字段给空字符串，graduation_year 给 null。`

// NewOpenAICompatibleExtractor returns an ExtractFunc that calls any
// OpenAI-compatible /chat/completions endpoint (custom provider). It fails
// construction when the provider is not configured, so an operator mistake is
// loud at startup instead of silently failing every job at runtime.
func NewOpenAICompatibleExtractor(cfg ExtractConfig) (ExtractFunc, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, ErrAIUnconfigured
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("%w: provider URL is invalid", ErrAIUnconfigured)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	endpoint := baseURL + "/chat/completions"
	apiKey := strings.TrimSpace(cfg.APIKey)
	model := strings.TrimSpace(cfg.Model)
	return func(ctx context.Context, fileName string, content []byte) (ExtractedProfile, error) {
		userContent, err := resumeProviderContent(ctx, fileName, content)
		if err != nil {
			return ExtractedProfile{}, err
		}
		requestBody, err := json.Marshal(map[string]any{
			"model": model,
			"messages": []map[string]any{
				{"role": "system", "content": extractorPrompt},
				{"role": "user", "content": userContent},
			},
			"temperature": 0.1,
		})
		if err != nil {
			return ExtractedProfile{}, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
		if err != nil {
			return ExtractedProfile{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+apiKey)
		response, err := clientCopy.Do(request)
		if err != nil {
			return ExtractedProfile{}, fmt.Errorf("%w: provider call failed", errExtractionProviderFailed)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			providerCode := readProviderErrorCode(response.Body)
			if providerCode != "" {
				return ExtractedProfile{}, fmt.Errorf("%w: provider returned %d (%s)", errExtractionProviderFailed, response.StatusCode, providerCode)
			}
			return ExtractedProfile{}, fmt.Errorf("%w: provider returned %d", errExtractionProviderFailed, response.StatusCode)
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
		if err != nil {
			return ExtractedProfile{}, fmt.Errorf("%w: provider response unreadable", errExtractionProviderFailed)
		}
		var envelope struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Choices) == 0 {
			return ExtractedProfile{}, fmt.Errorf("%w: provider response is invalid", errExtractionProviderFailed)
		}
		return normalizeExtracted(envelope.Choices[0].Message.Content)
	}, nil
}

// readProviderErrorCode keeps provider diagnostics useful without copying a
// provider message (which can echo prompt or resume content) into logs. Only a
// short machine code made from conservative characters is accepted.
func readProviderErrorCode(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, 16<<10))
	if err != nil {
		return ""
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return ""
	}
	code := strings.TrimSpace(envelope.Error.Code)
	if code == "" || len(code) > 80 {
		return ""
	}
	for _, char := range code {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return ""
	}
	return code
}

func resumeProviderContent(ctx context.Context, fileName string, content []byte) (any, error) {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(fileName)), ".pdf") {
		images, err := pdfResumeImages(ctx, content)
		if err != nil {
			return nil, err
		}
		parts := make([]map[string]any, 0, len(images)+1)
		parts = append(parts, map[string]any{"type": "text", "text": "请从这些简历 PDF 页面中提取求职画像。"})
		for _, image := range images {
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]string{
					"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image),
				},
			})
		}
		return parts, nil
	}
	text, err := resumeDocumentText(ctx, fileName, content)
	if err != nil {
		return nil, err
	}
	return "请提取这份简历：" + text, nil
}

// normalizeExtracted parses the model's JSON answer (which may be wrapped in
// markdown fences) and clamps every field to the profile limits. Any deviation
// from the contract fails the extraction rather than storing garbage.
func normalizeExtracted(content string) (ExtractedProfile, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	var modelResult struct {
		TargetRoles    json.RawMessage `json:"target_roles"`
		TechStack      json.RawMessage `json:"tech_stack"`
		Locations      json.RawMessage `json:"locations"`
		JobType        json.RawMessage `json:"job_type"`
		GraduationYear *int            `json:"graduation_year"`
		ResumeText     json.RawMessage `json:"resume_text"`
	}
	if err := json.Unmarshal([]byte(trimmed), &modelResult); err != nil {
		return ExtractedProfile{}, fmt.Errorf("%w: model output is not JSON", ErrExtractionFailed)
	}
	targetRoles, err := normalizeModelString(modelResult.TargetRoles)
	if err != nil {
		return ExtractedProfile{}, fmt.Errorf("%w: model output has invalid target_roles", ErrExtractionFailed)
	}
	techStack, err := normalizeModelString(modelResult.TechStack)
	if err != nil {
		return ExtractedProfile{}, fmt.Errorf("%w: model output has invalid tech_stack", ErrExtractionFailed)
	}
	locations, err := normalizeModelString(modelResult.Locations)
	if err != nil {
		return ExtractedProfile{}, fmt.Errorf("%w: model output has invalid locations", ErrExtractionFailed)
	}
	jobType, err := normalizeModelString(modelResult.JobType)
	if err != nil {
		return ExtractedProfile{}, fmt.Errorf("%w: model output has invalid job_type", ErrExtractionFailed)
	}
	resumeText, err := normalizeModelString(modelResult.ResumeText)
	if err != nil {
		return ExtractedProfile{}, fmt.Errorf("%w: model output has invalid resume_text", ErrExtractionFailed)
	}
	extracted := ExtractedProfile{
		TargetRoles: targetRoles, TechStack: techStack, Locations: locations,
		JobType: jobType, GraduationYear: modelResult.GraduationYear, ResumeText: resumeText,
	}
	switch extracted.JobType {
	case "", "daily_intern", "summer_intern", "campus_recruit":
	default:
		return ExtractedProfile{}, fmt.Errorf("%w: model output has an invalid job_type", ErrExtractionFailed)
	}
	if extracted.GraduationYear != nil && (*extracted.GraduationYear < 1900 || *extracted.GraduationYear > 2200) {
		return ExtractedProfile{}, fmt.Errorf("%w: model output has an invalid graduation_year", ErrExtractionFailed)
	}
	extracted.TargetRoles = truncateProfileText(extracted.TargetRoles, profileTargetRolesLimit)
	extracted.TechStack = truncateProfileText(extracted.TechStack, profileTechStackLimit)
	extracted.Locations = truncateProfileText(extracted.Locations, profileLocationsLimit)
	extracted.ResumeText = truncateProfileText(extracted.ResumeText, profileResumeTextLimit)
	return extracted, nil
}

// Some OpenAI-compatible multimodal servers return a list for keyword fields
// even when the prompt requests a comma-separated string. Accept only strings,
// string lists, null, or an omitted field; every other shape fails closed.
func normalizeModelString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return "", err
	}
	clean := make([]string, 0, len(values))
	for _, item := range values {
		if item = strings.TrimSpace(item); item != "" {
			clean = append(clean, item)
		}
	}
	return strings.Join(clean, "、"), nil
}

// truncateProfileText clamps a field to its byte limit while keeping the text
// valid UTF-8 (a cut mid-rune would corrupt the stored profile).
func truncateProfileText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	kept := 0
	total := 0
	for _, r := range value {
		size := utf8.RuneLen(r)
		if total+size > limit {
			break
		}
		total += size
		kept++
	}
	return string([]rune(value)[:kept])
}
