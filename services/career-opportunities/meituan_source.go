package career

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	MeituanSourceKey     = "official.meituan"
	defaultMeituanAPIURL = "https://zhaopin.meituan.com/api/official/job/getJobList"
	meituanPageSize      = 50
	meituanMaxPages      = 3
)

// MeituanSource reads the public campus-recruitment JSON endpoint owned by
// Meituan. It is an independently written adapter: the operator controls only
// whether this fixed source key is allowlisted, never an arbitrary browser URL.
type MeituanSource struct {
	endpoint   string
	httpClient *http.Client
	now        func() time.Time
}

func NewMeituanSource(endpoint string, client *http.Client) (*MeituanSource, error) {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultMeituanAPIURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" {
		return nil, errors.New("Meituan source endpoint is invalid")
	}
	loopback := parsed.Hostname() == "localhost"
	if address := net.ParseIP(parsed.Hostname()); address != nil {
		loopback = address.IsLoopback()
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return nil, errors.New("Meituan source endpoint must use HTTPS")
	}
	if !loopback && (parsed.Hostname() != "zhaopin.meituan.com" || (parsed.Port() != "" && parsed.Port() != "443") || parsed.Path != "/api/official/job/getJobList") {
		return nil, errors.New("Meituan source endpoint must be the official job API")
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &MeituanSource{endpoint: strings.TrimRight(parsed.String(), "/"), httpClient: &clientCopy, now: time.Now}, nil
}

func (source *MeituanSource) Key() string { return MeituanSourceKey }

type meituanProfile struct {
	TargetRoles string `json:"target_roles"`
	TechStack   string `json:"tech_stack"`
	Locations   string `json:"locations"`
	JobType     string `json:"job_type"`
}

type meituanJob struct {
	JobUnionID     string `json:"jobUnionId"`
	Name           string `json:"name"`
	JobFamily      string `json:"jobFamily"`
	JobType        string `json:"jobType"`
	RefreshTime    int64  `json:"refreshTime"`
	JobDuty        string `json:"jobDuty"`
	JobRequirement string `json:"jobRequirement"`
	CityList       []struct {
		Name string `json:"name"`
	} `json:"cityList"`
}

type meituanResponse struct {
	Message string `json:"message"`
	Data    struct {
		List []meituanJob `json:"list"`
		Page struct {
			TotalPage int `json:"totalPage"`
		} `json:"page"`
	} `json:"data"`
}

func (source *MeituanSource) Fetch(ctx context.Context, profile any) ([]Job, error) {
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return nil, errors.New("career profile cannot be encoded")
	}
	var frozen meituanProfile
	if err := json.Unmarshal(profileJSON, &frozen); err != nil {
		return nil, errors.New("career profile cannot be decoded")
	}

	jobs := make([]Job, 0, meituanPageSize)
	for page := 1; page <= meituanMaxPages; page++ {
		response, err := source.fetchPage(ctx, frozen, page)
		if err != nil {
			return nil, err
		}
		for _, item := range response.Data.List {
			if normalized, ok := source.normalizeJob(item, frozen); ok {
				jobs = append(jobs, normalized)
			}
		}
		if len(response.Data.List) == 0 || response.Data.Page.TotalPage <= page {
			break
		}
	}
	return jobs, nil
}

func (source *MeituanSource) fetchPage(ctx context.Context, profile meituanProfile, page int) (meituanResponse, error) {
	body := map[string]any{
		"page":         map[string]int{"pageNo": page, "pageSize": meituanPageSize},
		"jobShareType": "1",
		"keywords":     strings.TrimSpace(profile.TargetRoles),
		"cityList":     []string{},
		"department":   []string{},
		"jfJgList":     []string{},
		"jobType":      meituanJobTypeFilter(profile.JobType),
		"typeCode":     []string{},
		"specialCode":  []string{},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return meituanResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, source.endpoint, bytes.NewReader(raw))
	if err != nil {
		return meituanResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "HENU-Kit-Career/1.0")
	response, err := source.httpClient.Do(request)
	if err != nil {
		return meituanResponse{}, fmt.Errorf("Meituan source request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return meituanResponse{}, fmt.Errorf("Meituan source returned status %d", response.StatusCode)
	}
	var result meituanResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(&result); err != nil {
		return meituanResponse{}, fmt.Errorf("Meituan source response is invalid: %w", err)
	}
	if result.Message != "" && result.Message != "成功" {
		return meituanResponse{}, fmt.Errorf("Meituan source rejected the request: %s", result.Message)
	}
	return result, nil
}

func (source *MeituanSource) normalizeJob(item meituanJob, profile meituanProfile) (Job, bool) {
	title := strings.TrimSpace(item.Name)
	id := strings.TrimSpace(item.JobUnionID)
	if title == "" || id == "" {
		return Job{}, false
	}
	locations := make([]string, 0, len(item.CityList))
	for _, city := range item.CityList {
		if name := strings.TrimSpace(city.Name); name != "" {
			locations = append(locations, name)
		}
	}
	location := strings.Join(locations, "、")
	jobType := strings.TrimSpace(item.JobFamily)
	if strings.Contains(title, "实习") {
		jobType = "daily_intern"
	} else if item.JobType == "1" || strings.Contains(title, "校招") || strings.Contains(title, "应届") {
		jobType = "campus_recruit"
	} else if item.JobType == "2" {
		jobType = "daily_intern"
	}
	if !meituanJobTypeMatches(profile.JobType, jobType) {
		return Job{}, false
	}
	score, reasons := scoreMeituanJob(title, item.JobDuty, item.JobRequirement, location, jobType, profile)
	publishedAt := ""
	if item.RefreshTime > 0 {
		publishedAt = time.UnixMilli(item.RefreshTime).UTC().Format(time.RFC3339)
	}
	return Job{
		SourceKey:    MeituanSourceKey,
		Company:      "美团",
		Title:        title,
		Location:     location,
		JobType:      jobType,
		Description:  strings.TrimSpace(item.JobDuty),
		Requirements: nonEmptyStrings(item.JobRequirement),
		URL:          "https://zhaopin.meituan.com/web/position/detail?jobUnionId=" + url.QueryEscape(id) + "&highlightType=campus",
		PublishedAt:  publishedAt,
		FetchedAt:    source.now().UTC().Format(time.RFC3339),
		MatchScore:   score,
		MatchReasons: reasons,
	}, true
}

func meituanJobTypeFilter(profileType string) []map[string]any {
	codes := []string{"1", "2"}
	switch profileType {
	case "daily_intern", "summer_intern":
		codes = []string{"2"}
	case "campus_recruit":
		codes = []string{"1"}
	}
	filters := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		filters = append(filters, map[string]any{"code": code, "subCode": []string{}})
	}
	return filters
}

func meituanJobTypeMatches(profileType, jobType string) bool {
	switch profileType {
	case "daily_intern", "summer_intern":
		return jobType == "daily_intern"
	case "campus_recruit":
		return jobType == "campus_recruit"
	default:
		return true
	}
}

func scoreMeituanJob(title, description, requirement, location, jobType string, profile meituanProfile) (int, []string) {
	text := strings.ToLower(strings.Join([]string{title, description, requirement}, " "))
	score := 0
	reasons := make([]string, 0, 4)
	if matchesAny(text, splitCareerTerms(profile.TargetRoles)) {
		score += 55
		reasons = append(reasons, "匹配目标岗位 "+strings.TrimSpace(profile.TargetRoles))
	}
	techHits := matchingTerms(text, splitCareerTerms(profile.TechStack))
	if len(techHits) > 0 {
		score += min(24, len(techHits)*8)
		reasons = append(reasons, "匹配技术栈 "+strings.Join(techHits, "、"))
	}
	locationHits := matchingTerms(strings.ToLower(location), splitCareerTerms(profile.Locations))
	if len(locationHits) > 0 {
		score += 15
		reasons = append(reasons, "匹配目标城市 "+strings.Join(locationHits, "、"))
	}
	jobTypeMatched := (profile.JobType == "daily_intern" || profile.JobType == "summer_intern") && jobType == "daily_intern" ||
		profile.JobType == "campus_recruit" && jobType == "campus_recruit"
	if jobTypeMatched {
		score += 6
		reasons = append(reasons, "匹配求职类型")
	}
	return min(100, score), reasons
}

func splitCareerTerms(value string) []string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '/', '、', '\n', '\t':
			return true
		}
		return false
	})
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		if term := strings.TrimSpace(part); term != "" {
			terms = append(terms, term)
		}
	}
	return terms
}

func matchingTerms(text string, terms []string) []string {
	matches := make([]string, 0, len(terms))
	for _, term := range terms {
		if strings.Contains(text, term) {
			matches = append(matches, term)
		}
	}
	return matches
}

func matchesAny(text string, terms []string) bool { return len(matchingTerms(text, terms)) > 0 }

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
