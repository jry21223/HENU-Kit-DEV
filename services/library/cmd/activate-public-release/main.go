package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	if message, failed := activationOutcome(os.Stdout, result, err); failed {
		fail(message)
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

const activationFailurePrefix = "Library public release activation failed: "

// databaseCause replaces every driver-level cause. Driver errors name the
// internal address, hostname, database and user, and constraint or column
// names. The SQLSTATE is appended separately: it is a fixed SQL standard code,
// not deployment detail, and without it every database failure -- a unique
// violation, a refused connection, a missing grant -- reads the same.
const databaseCause = "database error during activation"

// databaseRemedy names the next step for a database-side failure. It does not
// claim the release is unchanged -- a connection lost during commit leaves
// that undecided -- and it does not describe what the orchestration wrapper
// does with its fence, which this binary neither controls nor observes. The
// retry is safe either way: an already-committed release takes the replay
// path and reports itself as replayed.
const databaseRemedy = "; retry with the same release id and receipt digest"

// activationOutcome encodes a successful activation and otherwise reports the
// operator-facing failure message, with failed saying which happened. main
// deliberately owns no part of this branch: routing the cause through
// operatorSafeCause is the point of the function, and a caller cannot report
// an activation failure without it. The message is operator copy that opens
// with the service name, so it stays a string rather than an error: ST1005
// governs error strings, and the prefix is fixed by contract.
func activationOutcome(out io.Writer, result library.PublicReleaseActivationResult, err error) (message string, failed bool) {
	if err != nil {
		return activationFailurePrefix + operatorSafeCause(err), true
	}
	if err := json.NewEncoder(out).Encode(result); err != nil {
		return "Library activation result could not be encoded", true
	}
	return "", false
}

// operatorSafeCause surfaces the causes ActivatePublicRelease raises itself --
// fixed rules naming a reviewed public path, which are what an operator needs
// to tell object verification from a binding failure -- and reduces database
// and network causes to a fixed classification. This message is captured by
// the materials orchestration wrapper and reprinted in its own operator
// output, so infrastructure detail must not reach it.
func operatorSafeCause(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE only. Message, Detail, ConstraintName and the schema names
		// they carry stay out.
		return databaseCause + " (SQLSTATE " + pgErr.Code + ")" + databaseRemedy
	}
	var connectErr *pgconn.ConnectError
	var parseErr *pgconn.ParseConfigError
	var scanErr pgx.ScanArgError
	var netErr net.Error
	var opErr *net.OpError
	if errors.As(err, &connectErr) || errors.As(err, &parseErr) || errors.As(err, &scanErr) ||
		errors.As(err, &netErr) || errors.As(err, &opErr) {
		return databaseCause + databaseRemedy
	}
	return err.Error()
}

func fail(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
