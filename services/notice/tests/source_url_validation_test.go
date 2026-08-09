package tests

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	notice "henukit.dev/notice"
)

func TestNoticeWriteBoundsSourceURLBytesAndAllowsUTF8Paths(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := notice.New(notice.Config{
		Database:       pool,
		Redis:          redisClient,
		ClientID:       "console-gateway",
		Keys:           map[string]string{"active": testSecret},
		PortalClientID: "portal-gateway-notice-read",
		PortalKeys:     map[string]string{"portal-active": portalFeedSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	const publicOrigin = "https://example.edu/"
	const sourceURLLimit = 2048
	// Use a deterministic, incompressible path instead of repeated characters:
	// notice_versions has a unique btree index containing source_url, so this
	// verifies the advertised 2,048-byte boundary can really persist.
	atLimit := publicOrigin + incompressibleURLPath(sourceURLLimit-len(publicOrigin))
	overLimit := atLimit + "a"
	if len(atLimit) != sourceURLLimit {
		t.Fatalf("limit fixture length = %d, want %d", len(atLimit), sourceURLLimit)
	}
	actor := uuid.NewString()
	encode := func(t *testing.T, value map[string]string) string {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	validSource := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", encode(t, map[string]string{
		"code":          "url-bound-" + uuid.NewString()[:8],
		"name":          "学校办公室",
		"canonical_url": atLimit,
	}), "idem_url_bound_source")
	sourceID := dataString(t, validSource, "id")
	_ = send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources/"+sourceID+"/versions", encode(t, map[string]string{
		"title":      "长度边界通知",
		"body":       "恰好位于公开来源 URL 长度上限的有效通知。",
		"source_url": atLimit,
	}), "idem_url_bound_version")
	// The source policy constrains the hostname, not a student-facing source
	// path or query. UTF-8 paths remain compatible as long as the raw URL fits
	// the byte bound and the canonical/version origins match.
	utf8Canonical := "https://example.edu/通知?栏目=教务"
	utf8Source := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", encode(t, map[string]string{
		"code":          "url-utf8-" + uuid.NewString()[:8],
		"name":          "学校办公室",
		"canonical_url": utf8Canonical,
	}), "idem_url_utf8_source")
	utf8SourceID := dataString(t, utf8Source, "id")
	_ = send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources/"+utf8SourceID+"/versions", encode(t, map[string]string{
		"title":      "中文来源地址通知",
		"body":       "来源路径和查询参数可以保留 UTF-8 文本。",
		"source_url": "https://example.edu/通知/详情?栏目=教务",
	}), "idem_url_utf8_version")
	validALabelCanonical := "https://xn--bcher-kva.example/通知?栏目=教务"
	validALabelSource := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", encode(t, map[string]string{
		"code":          "url-alabel-" + uuid.NewString()[:8],
		"name":          "学校办公室",
		"canonical_url": validALabelCanonical,
	}), "idem_url_alabel_source")
	validALabelSourceID := dataString(t, validALabelSource, "id")
	_ = send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources/"+validALabelSourceID+"/versions", encode(t, map[string]string{
		"title":      "有效 A-label 来源地址通知",
		"body":       "有效的 IDNA A-label 与 UTF-8 路径可以保留。",
		"source_url": "https://xn--bcher-kva.example/通知/详情?栏目=教务",
	}), "idem_url_alabel_version")
	// A bare 0x final label is an invalid IPv4 candidate in browsers, while
	// 0xg is an ordinary DNS label and must not be over-blocked with it.
	validHexLikeCanonical := "https://example.0xg/notices"
	validHexLikeSource := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", encode(t, map[string]string{
		"code":          "url-hex-like-" + uuid.NewString()[:8],
		"name":          "学校办公室",
		"canonical_url": validHexLikeCanonical,
	}), "idem_url_hex_like_source")
	validHexLikeSourceID := dataString(t, validHexLikeSource, "id")
	_ = send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources/"+validHexLikeSourceID+"/versions", encode(t, map[string]string{
		"title":      "有效十六进制样式尾部来源地址通知",
		"body":       "普通 DNS 标签 0xg 可以保留。",
		"source_url": validHexLikeCanonical + "/1",
	}), "idem_url_hex_like_version")

	for _, testCase := range []struct {
		name string
		url  string
	}{
		{name: "over byte limit", url: overLimit},
	} {
		t.Run("canonical "+testCase.name, func(t *testing.T) {
			code := "url-reject-" + uuid.NewString()[:8]
			if status := sendStatus(t, server.URL, actor, "notice.manage", "product", http.MethodPost, "/api/v1/sources", encode(t, map[string]string{
				"code":          code,
				"name":          "不应保存的来源",
				"canonical_url": testCase.url,
			}), "idem_"+code); status != http.StatusBadRequest {
				t.Fatalf("canonical URL write = %d, want 400", status)
			}
			var stored int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM notice_sources WHERE code=$1`, code).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != 0 {
				t.Fatalf("invalid canonical URL was stored: %q", testCase.url)
			}
		})

		t.Run("version "+testCase.name, func(t *testing.T) {
			if status := sendStatus(t, server.URL, actor, "notice.manage", "product", http.MethodPost, "/api/v1/sources/"+sourceID+"/versions", encode(t, map[string]string{
				"title":      "不应保存的 URL",
				"body":       "该来源 URL 超出公开契约的 UTF-8 字节上限。",
				"source_url": testCase.url,
			}), "idem_version_"+uuid.NewString()); status != http.StatusBadRequest {
				t.Fatalf("version URL write = %d, want 400", status)
			}
		})
	}

	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notice_versions WHERE source_id=$1`, sourceID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored versions = %d, want exactly the valid boundary version", stored)
	}
}

func incompressibleURLPath(length int) string {
	var path strings.Builder
	for counter := 0; path.Len() < length; counter++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("notice-source-url-boundary-%d", counter)))
		path.WriteString(base64.RawURLEncoding.EncodeToString(digest[:]))
	}
	return path.String()[:length]
}

func TestNoticeWriteRejectsMalformedNonPublicAndCrossOriginSourceURLs(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := notice.New(notice.Config{
		Database:       pool,
		Redis:          redisClient,
		ClientID:       "console-gateway",
		Keys:           map[string]string{"active": testSecret},
		PortalClientID: "portal-gateway-notice-read",
		PortalKeys:     map[string]string{"portal-active": portalFeedSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	actor := uuid.NewString()
	validSource := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", `{"code":"url-validation-`+uuid.NewString()+`","name":"学校办公室","canonical_url":"https://example.edu/notices"}`, "idem_url_validation_source_"+uuid.NewString())
	sourceID := dataString(t, validSource, "id")
	// The stricter shared validator must retain ordinary public source URLs on
	// both persistent write paths, so Portal can still render a valid feed.
	_ = send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources/"+sourceID+"/versions", `{"title":"可公开核对的通知","body":"这条有效版本应能继续进入 Portal 通知流。","source_url":"https://example.edu:0443/notices/1"}`, "idem_url_validation_version")

	for _, testCase := range []struct {
		name string
		code string
		url  string
	}{
		{name: "localhost", code: "invalid-localhost", url: "https://localhost/notices"},
		{name: "loopback IPv4", code: "invalid-loopback-ip", url: "https://127.0.0.1/notices"},
		{name: "public IPv4 literal", code: "invalid-public-ip", url: "https://8.8.8.8/notices"},
		// Chromium normalizes each of these non-canonical spellings to
		// 127.0.0.1. They must not fall through as dot-containing DNS names.
		{name: "abbreviated loopback IPv4", code: "invalid-abbreviated-loopback-ip", url: "https://127.1/notices"},
		{name: "abbreviated zero loopback IPv4", code: "invalid-abbreviated-zero-loopback-ip", url: "https://127.0.1/notices"},
		{name: "compact hexadecimal loopback IPv4", code: "invalid-compact-hex-loopback-ip", url: "https://0x7f000001/notices"},
		{name: "mixed hexadecimal loopback IPv4", code: "invalid-mixed-hex-loopback-ip", url: "https://0x7f.0.0.1/notices"},
		{name: "uppercase hexadecimal loopback IPv4", code: "invalid-uppercase-hex-loopback-ip", url: "https://0X7F.0X0.0X0.0X1/notices"},
		{name: "hexadecimal loopback IPv4", code: "invalid-hex-loopback-ip", url: "https://0x7f.0x0.0x0.0x1/notices"},
		{name: "octal loopback IPv4", code: "invalid-octal-loopback-ip", url: "https://0177.0.0.1/notices"},
		// WHATWG treats these Unicode separators as dots, so every mixed form
		// below canonicalizes to 127.0.0.1 in Chromium.
		{name: "fullwidth dot loopback IPv4", code: "invalid-fullwidth-dot-loopback-ip", url: "https://127\uFF0E0.0.1/notices"},
		{name: "ideographic dot loopback IPv4", code: "invalid-ideographic-dot-loopback-ip", url: "https://127\u30020.0.1/notices"},
		{name: "halfwidth ideographic dot loopback IPv4", code: "invalid-halfwidth-dot-loopback-ip", url: "https://127\uFF610.0.1/notices"},
		// Unicode case folding must not turn a non-ASCII hostname into an
		// apparently safe ASCII hostname before the public-DNS policy sees it.
		{name: "Kelvin sign hostname", code: "invalid-kelvin-host", url: "https://\u212A.example.edu/notices"},
		{name: "loopback IPv6", code: "invalid-loopback-ipv6", url: "https://[::1]/notices"},
		{name: "IPv4-mapped loopback", code: "invalid-mapped-loopback", url: "https://[::ffff:127.0.0.1]/notices"},
		{name: "private IPv4", code: "invalid-private-ip", url: "https://10.0.0.1/notices"},
		{name: "private IPv6", code: "invalid-private-ipv6", url: "https://[fc00::1]/notices"},
		{name: "link local IPv4", code: "invalid-link-local", url: "https://169.254.0.1/notices"},
		{name: "DNS underscore", code: "invalid-dns-underscore", url: "https://foo_.example.edu/notices"},
		{name: "DNS punctuation", code: "invalid-dns-punctuation", url: "https://foo!.example.edu/notices"},
		{name: "DNS label over 63 characters", code: "invalid-dns-label-size", url: "https://" + strings.Repeat("a", 64) + ".example.edu/notices"},
		{name: "invalid punycode A-label", code: "invalid-punycode-a-label", url: "https://xn--a.example/notices"},
		{name: "invalid short punycode A-label", code: "invalid-punycode-zero", url: "https://xn--0.example/notices"},
		{name: "numeric IPv4-like final DNS label", code: "invalid-dns-numeric-tail", url: "https://example.127/notices"},
		{name: "hexadecimal IPv4-like final DNS label", code: "invalid-dns-hex-tail", url: "https://foo.0x7f/notices"},
		{name: "bare hexadecimal IPv4-like final DNS label", code: "invalid-dns-bare-hex-tail", url: "https://example.0x/notices"},
		{name: "zero port", code: "invalid-zero-port", url: "https://example.edu:0/notices"},
		{name: "out of range port", code: "invalid-out-of-range-port", url: "https://example.edu:65536/notices"},
		{name: "empty port", code: "invalid-empty-port", url: "https://example.edu:/notices"},
		{name: "non numeric port", code: "invalid-numeric-port", url: "https://example.edu:not-a-port/notices"},
	} {
		t.Run("canonical "+testCase.name, func(t *testing.T) {
			code := testCase.code + "-" + uuid.NewString()[:8]
			if status := sendStatus(t, server.URL, actor, "notice.manage", "product", http.MethodPost, "/api/v1/sources", `{"code":"`+code+`","name":"不应公开的来源","canonical_url":"`+testCase.url+`"}`, "idem_"+code); status != http.StatusBadRequest {
				t.Fatalf("non-public canonical source write = %d, want 400", status)
			}
			var stored int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM notice_sources WHERE code=$1`, code).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != 0 {
				t.Fatalf("non-public canonical URL was stored: %q", testCase.url)
			}
		})
	}

	const missingHost = "https:///notices"
	const userInfo = "https://author@example.edu/notices"
	const mismatchedOrigin = "https://news.example.edu/notices/1"
	const invalidDNSLabel = "https://foo_.example.edu/notices/1"
	const kelvinHost = "https://\u212A.example.edu/notices/1"
	numericTailCanonical := "https://foo.0x7f/notices"
	numericTailSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧数字尾部来源', $3, $4)`, numericTailSourceID, "url-numeric-tail-"+uuid.NewString(), numericTailCanonical, actor); err != nil {
		t.Fatal(err)
	}
	bareHexTailCanonical := "https://example.0x/notices"
	bareHexTailSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧裸十六进制尾部来源', $3, $4)`, bareHexTailSourceID, "url-bare-hex-tail-"+uuid.NewString(), bareHexTailCanonical, actor); err != nil {
		t.Fatal(err)
	}
	invalidALabelCanonical := "https://xn--a.example/notices"
	invalidALabelSourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧无效 A-label 来源', $3, $4)`, invalidALabelSourceID, "url-invalid-alabel-"+uuid.NewString(), invalidALabelCanonical, actor); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name string
		path string
		body string
		key  string
	}{
		{name: "source missing host", path: "/api/v1/sources", body: `{"code":"invalid-source-host","name":"学校办公室","canonical_url":"` + missingHost + `"}`, key: "idem_url_source_host"},
		{name: "source user info", path: "/api/v1/sources", body: `{"code":"invalid-source-user","name":"学校办公室","canonical_url":"` + userInfo + `"}`, key: "idem_url_source_user"},
		{name: "version missing host", path: "/api/v1/sources/" + sourceID + "/versions", body: `{"title":"无主机地址","body":"不应被保存","source_url":"` + missingHost + `"}`, key: "idem_url_version_host"},
		{name: "version user info", path: "/api/v1/sources/" + sourceID + "/versions", body: `{"title":"含用户信息地址","body":"不应被保存","source_url":"` + userInfo + `"}`, key: "idem_url_version_user"},
		{name: "version invalid DNS label", path: "/api/v1/sources/" + sourceID + "/versions", body: `{"title":"无效 DNS 标签","body":"不应被保存","source_url":"` + invalidDNSLabel + `"}`, key: "idem_url_version_dns_label"},
		{name: "version Kelvin sign hostname", path: "/api/v1/sources/" + sourceID + "/versions", body: `{"title":"Unicode 主机名","body":"不应被保存","source_url":"` + kelvinHost + `"}`, key: "idem_url_version_kelvin_host"},
		{name: "version numeric IPv4-like final DNS label", path: "/api/v1/sources/" + numericTailSourceID.String() + "/versions", body: `{"title":"数字 DNS 尾部","body":"不应被保存","source_url":"` + numericTailCanonical + `/1"}`, key: "idem_url_version_numeric_tail"},
		{name: "version bare hexadecimal IPv4-like final DNS label", path: "/api/v1/sources/" + bareHexTailSourceID.String() + "/versions", body: `{"title":"裸十六进制 DNS 尾部","body":"不应被保存","source_url":"` + bareHexTailCanonical + `/1"}`, key: "idem_url_version_bare_hex_tail"},
		{name: "version invalid punycode A-label", path: "/api/v1/sources/" + invalidALabelSourceID.String() + "/versions", body: `{"title":"无效 A-label","body":"不应被保存","source_url":"` + invalidALabelCanonical + `/1"}`, key: "idem_url_version_invalid_alabel"},
		{name: "version different source origin", path: "/api/v1/sources/" + sourceID + "/versions", body: `{"title":"跨来源地址","body":"不应被保存","source_url":"` + mismatchedOrigin + `"}`, key: "idem_url_version_origin"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if status := sendStatus(t, server.URL, actor, "notice.manage", "product", http.MethodPost, testCase.path, testCase.body, testCase.key); status != http.StatusBadRequest {
				t.Fatalf("malformed source URL write = %d, want 400", status)
			}
		})
	}

	var stored int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM notice_versions
		WHERE source_id=$1 AND source_url IN ($2, $3, $4, $5)`, sourceID, missingHost, userInfo, invalidDNSLabel, mismatchedOrigin).Scan(&stored); err != nil {
		t.Fatalf("count malformed URLs: %v", err)
	}
	if stored != 0 {
		t.Fatalf("malformed source URLs were stored: %d", stored)
	}
}
