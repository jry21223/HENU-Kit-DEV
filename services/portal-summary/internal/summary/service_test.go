package summary

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildReportsRealProbeAndFeedbackState(t *testing.T) {
	feedbackTime := time.Now().UTC().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/ready", "/", "/learn":
			writer.WriteHeader(http.StatusOK)
		case "/broken":
			writer.WriteHeader(http.StatusServiceUnavailable)
		case "/feedback":
			_ = json.NewEncoder(writer).Encode(Feedback{PendingCount: 2, RecentCount: 5, AsOf: feedbackTime})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	deployedAt := time.Now().Add(-time.Hour).UTC()
	service, err := New(Config{
		Version: "2026.07.19", CommitSHA: "0123456789abcdef0123456789abcdef01234567", DeployedAt: deployedAt,
		ReadinessURL: server.URL + "/ready", KeyProbes: []Probe{{Name: "首页", URL: server.URL + "/"}, {Name: "故障页", URL: server.URL + "/broken"}},
		EntryProbes: []Probe{{Name: "学习", URL: server.URL + "/learn"}}, FeedbackURL: server.URL + "/feedback",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result := service.Build(t.Context())
	if result.ID != "portal" || result.Status != "partial" || len(result.Metrics) != 8 || result.Metrics[3].Value != "ready" || result.Metrics[4].Value != "1/2" || result.Metrics[5].Value != "1/1" || result.Metrics[6].Value != "2 待处理" || result.Metrics[7].Value != "1" {
		t.Fatalf("unexpected Portal summary: %+v", result)
	}
}

func TestBuildIsHonestWhenFeedbackIsNotConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) }))
	defer server.Close()
	service, err := New(Config{
		Version: "1.0.0", CommitSHA: "0123456", DeployedAt: time.Now(), ReadinessURL: server.URL,
		KeyProbes: []Probe{{Name: "首页", URL: server.URL}}, EntryProbes: []Probe{{Name: "学习", URL: server.URL}},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result := service.Build(t.Context())
	if result.Status != "partial" || result.Metrics[6].Value != "未接入" || result.Metrics[4].Value != "1/1" || result.Metrics[5].Value != "1/1" {
		t.Fatalf("unconfigured sources were not represented honestly: %+v", result)
	}
}

func TestProbeDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(writer, request, "/target", http.StatusFound)
			return
		}
		if request.URL.Path == "/target" {
			redirected.Add(1)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	service, err := New(Config{
		Version: "1.0.0", CommitSHA: "0123456", DeployedAt: time.Now(), ReadinessURL: server.URL,
		KeyProbes: []Probe{{Name: "跳转入口", URL: server.URL + "/redirect"}}, EntryProbes: []Probe{{Name: "学习", URL: server.URL}},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result := service.Build(t.Context())
	if redirected.Load() != 0 || result.Metrics[4].Value != "1/1" {
		t.Fatalf("redirect was followed or misreported: hits=%d summary=%+v", redirected.Load(), result)
	}
}
