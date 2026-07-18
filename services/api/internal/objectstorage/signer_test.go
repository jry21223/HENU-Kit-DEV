package objectstorage

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPresignPutUsesS3SignatureV4WithoutLeakingSecret(t *testing.T) {
	signer, err := New(Config{Endpoint: "http://minio:9000", Region: "us-east-1", Bucket: "henu-kit", AccessKey: "access", SecretKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := signer.PresignPut("notice/2026/file.pdf", 10*time.Minute, time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/henu-kit/notice/2026/file.pdf" || parsed.Query().Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" || parsed.Query().Get("X-Amz-Expires") != "600" {
		t.Fatalf("unexpected presigned URL: %s", value)
	}
	if strings.Contains(value, "secret") || len(parsed.Query().Get("X-Amz-Signature")) != 64 {
		t.Fatalf("secret leaked or signature missing: %s", value)
	}
}
