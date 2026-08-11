package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	library "henukit.dev/library"
)

const maxBundleBytes = 32 << 20

func main() {
	if len(os.Args) != 3 || os.Args[1] != "--bundle" {
		fail("usage: library-activate-public-release --bundle PATH")
	}
	if unsupportedEnvironment() != "" {
		fail("caller-supplied OSS credentials, authority, or proxy configuration is unsupported")
	}
	bundle, err := readBundle(os.Args[2])
	if err != nil {
		fail("activation bundle is invalid")
	}
	databaseURL := os.Getenv("LIBRARY_DATABASE_URL")
	role := os.Getenv("LIBRARY_OSS_ECS_RAM_ROLE")
	if databaseURL == "" || role == "" {
		fail("Library activation configuration is incomplete")
	}
	store, err := library.NewAliyunDownloadStore(library.DownloadOSSConfig{
		Bucket: "henukit", Region: "cn-beijing",
		InternalEndpoint: "https://oss-cn-beijing-internal.aliyuncs.com",
		PublicEndpoint:   "https://oss-cn-beijing.aliyuncs.com",
		ECSRAMRole:       role,
	})
	if err != nil {
		fail("Library activation OSS configuration is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fail("Library activation database is unavailable")
	}
	defer database.Close()
	result, err := library.ActivatePublicRelease(ctx, database, store, bundle, time.Now)
	if err != nil {
		fail("Library public release activation failed")
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail("Library activation result could not be encoded")
	}
}

func unsupportedEnvironment() string {
	for _, name := range []string{
		"ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET", "ALIBABA_CLOUD_SECURITY_TOKEN",
		"LIBRARY_OSS_BUCKET", "LIBRARY_OSS_REGION", "LIBRARY_OSS_INTERNAL_ENDPOINT", "LIBRARY_OSS_PUBLIC_ENDPOINT",
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy",
	} {
		if os.Getenv(name) != "" {
			return name
		}
	}
	return ""
}

func readBundle(name string) (library.PublicReleaseActivation, error) {
	metadata, err := os.Lstat(name)
	if err != nil || !metadata.Mode().IsRegular() || metadata.Mode()&os.ModeSymlink != 0 || metadata.Size() <= 0 || metadata.Size() > maxBundleBytes {
		return library.PublicReleaseActivation{}, errors.New("unsafe activation bundle")
	}
	file, err := os.Open(name)
	if err != nil {
		return library.PublicReleaseActivation{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(metadata, opened) {
		return library.PublicReleaseActivation{}, errors.New("activation bundle changed while opening")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxBundleBytes+1))
	decoder.DisallowUnknownFields()
	var bundle library.PublicReleaseActivation
	if err := decoder.Decode(&bundle); err != nil {
		return library.PublicReleaseActivation{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return library.PublicReleaseActivation{}, errors.New("activation bundle has trailing content")
	}
	return bundle, nil
}

func fail(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
