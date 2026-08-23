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

// releaseAttestation is the fixed, bounded stdout contract consumed by the
// privileged orchestrator. The complete per-object inventory remains only in
// the root-owned release-commit.json audit record.
type releaseAttestation struct {
	Version             int    `json:"version"`
	State               string `json:"state"`
	ReleaseID           string `json:"release_id"`
	ReceiptSHA256       string `json:"receipt_sha256"`
	ManifestSHA256      string `json:"manifest_sha256"`
	InventorySHA256     string `json:"inventory_sha256"`
	TreeSHA256          string `json:"tree_sha256"`
	AssetCount          int    `json:"asset_count"`
	ReleaseCommitSHA256 string `json:"release_commit_sha256"`
}

func newReleaseAttestation(result materialsoss.ReleaseResult) releaseAttestation {
	return releaseAttestation{
		Version:             result.Version,
		State:               result.State,
		ReleaseID:           result.ReleaseID,
		ReceiptSHA256:       result.ReceiptSHA256,
		ManifestSHA256:      result.ManifestSHA256,
		InventorySHA256:     result.InventorySHA256,
		TreeSHA256:          result.TreeSHA256,
		AssetCount:          result.AssetCount,
		ReleaseCommitSHA256: result.ReleaseCommitSHA256,
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "materials-oss-release:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 5 || os.Args[1] != "--release-id" || os.Args[3] != "--receipt-sha256" {
		return errors.New("expected --release-id ID --receipt-sha256 SHA256")
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
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	result, err := materialsoss.PublishRelease(ctx, materialsoss.Config{SealedRoot: sealedRoot, AuditRoot: auditRoot, Bucket: bucket, Region: region, Prefix: "releases"}, materialsoss.ReleaseRequest{ReleaseID: os.Args[2], ReceiptSHA256: os.Args[4]}, store)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(newReleaseAttestation(result))
}
