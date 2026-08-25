package career

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetWorkMCPConfig configures the internal getWork MCP job source. The
// browser never controls this configuration: endpoint, credentials and source
// keys are all operator-owned deployment facts.
type GetWorkMCPConfig struct {
	Endpoint     string
	AccessToken  string
	AllowSources []string
	SinceDays    int
	HTTPClient   *http.Client
}

type getWorkMCPSourceList struct {
	Status  string `json:"status"`
	Sources []struct {
		Key string `json:"key"`
	} `json:"sources"`
}

type getWorkMCPCrawl struct {
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
	Jobs      []struct {
		Title       string `json:"title"`
		Company     string `json:"company"`
		Source      string `json:"source"`
		Location    string `json:"location"`
		JobType     string `json:"job_type"`
		Description string `json:"description"`
		Requirement string `json:"requirement"`
		ApplyURL    string `json:"apply_url"`
		PublishDate string `json:"publish_date"`
	} `json:"jobs"`
}

// NewGetWorkMCPWork verifies the MCP protocol and required tools, then returns
// the Career worker function that calls the authorized crawl_jobs tools. The
// upstream MCP owns crawling only; Career still owns matching and persistence.
func NewGetWorkMCPWork(ctx context.Context, config GetWorkMCPConfig) (WorkFunc, error) {
	endpoint, err := validGetWorkMCPEndpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	accessToken := strings.TrimSpace(config.AccessToken)
	normalizedToken := strings.ToLower(accessToken)
	if len(accessToken) < 32 || strings.ContainsAny(accessToken, " \t\r\n") ||
		strings.Contains(normalizedToken, "replace") || strings.Contains(normalizedToken, "example") ||
		strings.Contains(normalizedToken, "change-me") || strings.Contains(normalizedToken, "changeme") ||
		strings.Contains(normalizedToken, "test-secret") || strings.Contains(normalizedToken, "test-only") {
		return nil, errors.New("getWork MCP access token must be a non-placeholder value of at least 32 characters")
	}
	sources := normalizedSourceKeys(config.AllowSources)
	if len(sources) == 0 {
		return nil, errors.New("getWork MCP source allowlist is required")
	}
	for _, source := range sources {
		if _, ok := approvedGetWorkApplyHosts[source]; !ok {
			return nil, fmt.Errorf("getWork MCP source %q has no approved apply URL policy", source)
		}
	}
	if config.SinceDays < 0 || config.SinceDays > 365 {
		return nil, errors.New("getWork MCP since-days must be between 0 and 365")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	transport := clientCopy.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clientCopy.Transport = getWorkBearerTransport{next: transport, token: accessToken}
	connect := func(connectContext context.Context) (*mcpsdk.ClientSession, error) {
		client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "henukit-career", Version: "1.0.0"}, nil)
		return client.Connect(connectContext, &mcpsdk.StreamableClientTransport{
			Endpoint: endpoint, HTTPClient: &clientCopy, DisableStandaloneSSE: true,
		}, nil)
	}
	probe, err := connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect getWork MCP: %w", err)
	}
	if err := verifyGetWorkMCP(ctx, probe, sources); err != nil {
		_ = probe.Close()
		return nil, err
	}
	_ = probe.Close()

	return func(workContext context.Context, profile any) (WorkResult, error) {
		session, err := connect(workContext)
		if err != nil {
			return WorkResult{}, fmt.Errorf("connect getWork MCP: %w", err)
		}
		defer session.Close()
		frozen, err := careerProfileForMatching(profile)
		if err != nil {
			return WorkResult{}, err
		}
		jobs := make([]Job, 0, 64)
		states := make(map[string]any, len(sources))
		matched, succeeded := 0, 0
		for _, source := range sources {
			var crawl getWorkMCPCrawl
			err := callGetWorkTool(workContext, session, "crawl_jobs", map[string]any{
				"source": source, "since_days": config.SinceDays,
			}, &crawl)
			if err != nil || crawl.Status != "ok" {
				if err != nil {
					log.Printf("career: getWork MCP source %q failed: %s", source, safeGetWorkLogValue(err.Error()))
				} else {
					log.Printf("career: getWork MCP source %q returned status %q reason %q", source, safeGetWorkLogValue(crawl.Status), safeGetWorkLogValue(crawl.Reason))
				}
				states["getwork."+source] = map[string]any{"status": "failed"}
				continue
			}
			succeeded++
			states["getwork."+source] = map[string]any{"status": "success", "found": len(crawl.Jobs)}
			for _, raw := range crawl.Jobs {
				job := normalizeGetWorkMCPJob(raw, crawl.FetchedAt, frozen)
				if strings.TrimSpace(job.Title) == "" || strings.TrimSpace(job.URL) == "" {
					continue
				}
				if !approvedGetWorkApplyURL(source, job.URL) {
					continue
				}
				if !getWorkJobTypeMatches(frozen.JobType, job.JobType) {
					continue
				}
				if job.MatchScore >= 50 {
					matched++
				}
				jobs = append(jobs, job)
			}
		}
		if succeeded == 0 {
			return WorkResult{}, errors.New("all authorized getWork MCP sources failed")
		}
		return WorkResult{
			Payload: map[string]any{"jobs": jobs, "sources": states}, SourceCount: len(sources),
			JobCount: len(jobs), MatchedCount: matched,
			Summary: fmt.Sprintf("已扫描 %d 个来源，发现 %d 个岗位，%d 个推荐", len(sources), len(jobs), matched),
		}, nil
	}, nil
}

func validGetWorkMCPEndpoint(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "/mcp" {
		return "", errors.New("getWork MCP endpoint is invalid")
	}
	host := parsed.Hostname()
	loopback := strings.EqualFold(host, "localhost")
	if address := net.ParseIP(host); address != nil {
		loopback = address.IsLoopback()
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (loopback || host == "getwork-mcp")) {
		return "", errors.New("getWork MCP endpoint must use HTTPS, loopback, or the internal getwork-mcp service")
	}
	return parsed.String(), nil
}

func normalizedSourceKeys(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key != "" {
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func verifyGetWorkMCP(ctx context.Context, session *mcpsdk.ClientSession, allowlist []string) error {
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list getWork MCP tools: %w", err)
	}
	required := map[string]bool{"list_sources": false, "crawl_jobs": false}
	for _, tool := range listed.Tools {
		if _, ok := required[tool.Name]; !ok {
			return fmt.Errorf("getWork MCP exposed unexpected tool %q", tool.Name)
		}
		required[tool.Name] = true
	}
	for name, found := range required {
		if !found {
			return fmt.Errorf("getWork MCP is missing required tool %q", name)
		}
	}
	var sources getWorkMCPSourceList
	if err := callGetWorkTool(ctx, session, "list_sources", map[string]any{}, &sources); err != nil {
		return err
	}
	available := map[string]bool{}
	for _, source := range sources.Sources {
		available[source.Key] = true
	}
	for _, source := range allowlist {
		if !available[source] {
			return fmt.Errorf("getWork MCP source %q is not configured", source)
		}
	}
	return nil
}

func callGetWorkTool(ctx context.Context, session *mcpsdk.ClientSession, name string, arguments map[string]any, target any) error {
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return fmt.Errorf("call getWork MCP tool %s: %w", name, err)
	}
	if result.IsError {
		return fmt.Errorf("getWork MCP tool %s failed", name)
	}
	return decodeGetWorkToolResult(name, result, target)
}

func decodeGetWorkToolResult(name string, result *mcpsdk.CallToolResult, target any) error {
	raw, err := json.Marshal(result.StructuredContent)
	if err == nil && string(raw) != "null" {
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("getWork MCP tool %s returned invalid structured content: %w", name, err)
		}
		return nil
	}
	for _, content := range result.Content {
		textContent, ok := content.(*mcpsdk.TextContent)
		if !ok || strings.TrimSpace(textContent.Text) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(textContent.Text), target); err == nil {
			return nil
		}
	}
	return fmt.Errorf("getWork MCP tool %s returned no JSON content", name)
}

func careerProfileForMatching(profile any) (meituanProfile, error) {
	raw, err := json.Marshal(profile)
	if err != nil {
		return meituanProfile{}, errors.New("career profile cannot be encoded")
	}
	var frozen meituanProfile
	if err := json.Unmarshal(raw, &frozen); err != nil {
		return meituanProfile{}, errors.New("career profile cannot be decoded")
	}
	return frozen, nil
}

func normalizeGetWorkMCPJob(raw struct {
	Title       string `json:"title"`
	Company     string `json:"company"`
	Source      string `json:"source"`
	Location    string `json:"location"`
	JobType     string `json:"job_type"`
	Description string `json:"description"`
	Requirement string `json:"requirement"`
	ApplyURL    string `json:"apply_url"`
	PublishDate string `json:"publish_date"`
}, fetchedAt string, profile meituanProfile) Job {
	jobType := strings.TrimSpace(raw.JobType)
	if strings.Contains(jobType, "实习") {
		jobType = "daily_intern"
	} else if strings.Contains(jobType, "校招") || strings.Contains(jobType, "应届") {
		jobType = "campus_recruit"
	}
	score, reasons := scoreMeituanJob(raw.Title, raw.Description, raw.Requirement, raw.Location, jobType, profile)
	return Job{
		SourceKey: "getwork." + strings.TrimSpace(raw.Source), Company: strings.TrimSpace(raw.Company),
		Title: strings.TrimSpace(raw.Title), Location: strings.TrimSpace(raw.Location), JobType: jobType,
		Description: strings.TrimSpace(raw.Description), Requirements: nonEmptyStrings(raw.Requirement),
		URL: strings.TrimSpace(raw.ApplyURL), PublishedAt: strings.TrimSpace(raw.PublishDate), FetchedAt: strings.TrimSpace(fetchedAt),
		MatchScore: score, MatchReasons: reasons,
	}
}

func getWorkJobTypeMatches(profileType, jobType string) bool {
	if jobType != "daily_intern" && jobType != "campus_recruit" {
		// The pinned upstream's Meituan source currently emits job families
		// such as "技术类" here. Keep that unknown fact without awarding a
		// type-match score; only reject an explicitly identified opposite type.
		return true
	}
	return meituanJobTypeMatches(profileType, jobType)
}

var approvedGetWorkApplyHosts = map[string]map[string]bool{
	"meituan": {"zhaopin.meituan.com": true},
	"tencent": {"join.qq.com": true},
}

func approvedGetWorkApplyURL(source, value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return false
	}
	hosts, ok := approvedGetWorkApplyHosts[source]
	return ok && hosts[strings.ToLower(parsed.Hostname())]
}

func safeGetWorkLogValue(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(value))
	if len(value) > 200 {
		value = value[:200]
	}
	return value
}

type getWorkBearerTransport struct {
	next  http.RoundTripper
	token string
}

func (transport getWorkBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copyRequest := request.Clone(request.Context())
	copyRequest.Header = request.Header.Clone()
	copyRequest.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.next.RoundTrip(copyRequest)
}
