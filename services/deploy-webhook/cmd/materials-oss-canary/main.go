package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"henukit.dev/deploy-webhook/internal/materialsoss"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "materials-oss-canary:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 7 || os.Args[1] != "--release-id" || os.Args[3] != "--receipt-sha256" || os.Args[5] != "--asset-sha256" {
		return errors.New("expected --release-id ID --receipt-sha256 SHA256 --asset-sha256 SHA256")
	}
	sealedRoot := os.Getenv("HENUKIT_MATERIALS_SEALED_ROOT")
	auditRoot := os.Getenv("HENUKIT_MATERIALS_OSS_AUDIT_ROOT")
	bucket := os.Getenv("HENUKIT_MATERIALS_OSS_BUCKET")
	region := os.Getenv("HENUKIT_MATERIALS_OSS_REGION")
	endpoint := os.Getenv("HENUKIT_MATERIALS_OSS_ENDPOINT")
	ramRole := os.Getenv("HENUKIT_MATERIALS_OSS_RAM_ROLE")
	if sealedRoot == "" || auditRoot == "" || bucket == "" || region == "" || endpoint == "" || ramRole == "" {
		return errors.New("fixed OSS publication configuration is incomplete")
	}
	store, err := materialsoss.NewAliyunStore(bucket, region, "https://"+endpoint, ramRole)
	if err != nil {
		return err
	}
	parent, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	result, err := materialsoss.Publish(ctx, materialsoss.Config{SealedRoot: sealedRoot, AuditRoot: auditRoot, Bucket: bucket, Region: region, Prefix: "releases"}, materialsoss.Request{ReleaseID: os.Args[2], ReceiptSHA256: os.Args[4], AssetSHA256: os.Args[6]}, store)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
