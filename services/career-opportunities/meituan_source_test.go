package career

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMeituanSourceFetchesAndMatchesOfficialJobs(t *testing.T) {
	var receivedKeyword string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		body := make([]byte, request.ContentLength)
		_, _ = request.Body.Read(body)
		receivedKeyword = string(body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"message":"成功",
			"data":{
				"page":{"pageNo":1,"pageSize":20,"totalCount":1,"totalPage":1},
				"list":[{
					"jobUnionId":"job-001",
					"name":"Go 后端开发工程师",
					"jobType":"1",
					"cityList":[{"name":"北京市"}],
					"jobFamily":"技术类",
					"refreshTime":1787389215000,
					"jobDuty":"负责 Go 服务端研发与 PostgreSQL 数据建模",
					"jobRequirement":"熟悉 Go、PostgreSQL，具备工程实践"
				}]
			}
		}`))
	}))
	defer server.Close()

	source, err := NewMeituanSource(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := source.Fetch(context.Background(), map[string]any{
		"target_roles": "后端开发",
		"tech_stack":   "Go, PostgreSQL",
		"locations":    "北京",
		"job_type":     "campus_recruit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(receivedKeyword, `"keywords":"后端开发"`) {
		t.Fatalf("official request body did not carry the frozen target role: %s", receivedKeyword)
	}
	if !strings.Contains(receivedKeyword, `"jobType":[{"code":"1"`) || strings.Contains(receivedKeyword, `"code":"2"`) {
		t.Fatalf("campus profile did not restrict the upstream job type: %s", receivedKeyword)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	job := jobs[0]
	if job.SourceKey != MeituanSourceKey || job.Company != "美团" || job.Title != "Go 后端开发工程师" {
		t.Fatalf("unexpected normalized job: %+v", job)
	}
	if job.Location != "北京市" || job.JobType != "campus_recruit" {
		t.Fatalf("unexpected location/type: %+v", job)
	}
	if job.URL != "https://zhaopin.meituan.com/web/position/detail?jobUnionId=job-001&highlightType=campus" {
		t.Fatalf("apply URL = %q", job.URL)
	}
	if job.MatchScore < 50 || len(job.MatchReasons) < 2 {
		t.Fatalf("job was not explainably matched: %+v", job)
	}
	if !strings.Contains(strings.Join(job.MatchReasons, ","), "匹配求职类型") {
		t.Fatalf("official jobType did not contribute an explainable match: %+v", job.MatchReasons)
	}
	if _, err := time.Parse(time.RFC3339, job.PublishedAt); err != nil {
		t.Fatalf("published_at = %q: %v", job.PublishedAt, err)
	}
}

func TestMeituanSourceExcludesJobsOutsideTheRequestedType(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requestBody = string(body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"message":"成功",
			"data":{"page":{"totalPage":1},"list":[
				{"jobUnionId":"campus-1","name":"后端开发工程师校招","jobType":"1","jobDuty":"后端开发"},
				{"jobUnionId":"intern-1","name":"后端开发实习生","jobType":"2","jobDuty":"后端开发"}
			]}
		}`))
	}))
	defer server.Close()

	source, err := NewMeituanSource(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := source.Fetch(context.Background(), map[string]any{
		"target_roles": "后端开发",
		"job_type":     "daily_intern",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(requestBody, `"jobType":[{"code":"2"`) || strings.Contains(requestBody, `"code":"1"`) {
		t.Fatalf("intern profile did not restrict the upstream job type: %s", requestBody)
	}
	if len(jobs) != 1 || jobs[0].JobType != "daily_intern" || jobs[0].Title != "后端开发实习生" {
		t.Fatalf("type-mismatched jobs were not excluded: %+v", jobs)
	}
}

func TestMeituanSourceRejectsUntrustedEndpoint(t *testing.T) {
	_, err := NewMeituanSource("http://example.com/jobs", http.DefaultClient)
	if err == nil {
		t.Fatal("untrusted non-TLS endpoint was accepted")
	}
	if _, err := NewMeituanSource("https://example.com/api/official/job/getJobList", http.DefaultClient); err == nil {
		t.Fatal("non-Meituan endpoint was accepted as an official source")
	}
}

func TestMeituanSourceDoesNotFollowRedirects(t *testing.T) {
	targetHit := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetHit = true }))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	source, err := NewMeituanSource(upstream.URL, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Fetch(context.Background(), map[string]any{"target_roles": "后端"}); err == nil {
		t.Fatal("redirecting official source was accepted")
	}
	if targetHit {
		t.Fatal("Meituan source followed a redirect to another origin")
	}
}
