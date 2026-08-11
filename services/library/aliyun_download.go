package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	osscred "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	openapicred "github.com/aliyun/credentials-go/credentials"
)

const (
	fixedDownloadBucket           = "henukit"
	fixedDownloadRegion           = "cn-beijing"
	fixedDownloadInternalEndpoint = "https://oss-cn-beijing-internal.aliyuncs.com"
	fixedDownloadPublicEndpoint   = "https://oss-cn-beijing.aliyuncs.com"
)

type AliyunDownloadStore struct {
	internalClient  *oss.Client
	publicClient    *oss.Client
	anonymousClient *http.Client
	bucket          string
}

type DownloadOSSConfig struct {
	Bucket           string
	Region           string
	InternalEndpoint string
	PublicEndpoint   string
	ECSRAMRole       string
}

func NewAliyunDownloadStore(config DownloadOSSConfig) (*AliyunDownloadStore, error) {
	if config.Bucket != fixedDownloadBucket || config.Region != fixedDownloadRegion || config.InternalEndpoint != fixedDownloadInternalEndpoint || config.PublicEndpoint != fixedDownloadPublicEndpoint || strings.TrimSpace(config.ECSRAMRole) == "" {
		return nil, errors.New("fixed OSS download configuration is invalid")
	}
	credential, credentialErr := openapicred.NewCredential(new(openapicred.Config).
		SetType("ecs_ram_role").SetRoleName(config.ECSRAMRole).SetDisableIMDSv1(true))
	provider := osscred.CredentialsProviderFunc(func(context.Context) (osscred.Credentials, error) {
		if credentialErr != nil {
			return osscred.Credentials{}, errors.New("could not initialize ECS RAM role credentials")
		}
		value, err := credential.GetCredential()
		if err != nil {
			return osscred.Credentials{}, errors.New("could not refresh ECS RAM role credentials")
		}
		if value.AccessKeyId == nil || value.AccessKeySecret == nil || value.SecurityToken == nil {
			return osscred.Credentials{}, errors.New("ECS RAM role returned incomplete temporary credentials")
		}
		return osscred.Credentials{AccessKeyID: *value.AccessKeyId, AccessKeySecret: *value.AccessKeySecret, SecurityToken: *value.SecurityToken}, nil
	})
	base := oss.LoadDefaultConfig().WithCredentialsProvider(provider).WithRegion(config.Region).WithSignatureVersion(oss.SignatureVersionV4)
	internalConfig := base.Copy()
	publicConfig := base.Copy()
	return &AliyunDownloadStore{
		internalClient:  oss.NewClient(internalConfig.WithEndpoint(config.InternalEndpoint)),
		publicClient:    oss.NewClient(publicConfig.WithEndpoint(config.PublicEndpoint)),
		anonymousClient: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		bucket:          config.Bucket,
	}, nil
}

func (s *AliyunDownloadStore) AnonymousDenied(ctx context.Context, key, versionID string) error {
	location := url.URL{Scheme: "https", Host: publicOSSHost, Path: "/" + key}
	query := location.Query()
	query.Set("versionId", versionID)
	location.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return errors.New("could not construct anonymous OSS verification")
	}
	response, err := s.anonymousClient.Do(request)
	if err != nil {
		return errors.New("anonymous OSS verification request failed")
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 4096)
	if response.StatusCode != http.StatusForbidden {
		return fmt.Errorf("anonymous OSS request returned status %d", response.StatusCode)
	}
	return nil
}

func (s *AliyunDownloadStore) Head(ctx context.Context, key, versionID string) (DownloadObjectState, bool, error) {
	result, err := s.internalClient.HeadObject(ctx, &oss.HeadObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key), VersionId: oss.Ptr(versionID)})
	if err != nil {
		if isDownloadObjectNotFound(err) {
			return DownloadObjectState{}, false, nil
		}
		return DownloadObjectState{}, false, safeDownloadOSSFailure("head_object", err)
	}
	return DownloadObjectState{Bytes: result.ContentLength, SHA256: result.Metadata["sha256"], Encryption: oss.ToString(result.ServerSideEncryption), VersionID: oss.ToString(result.VersionId)}, true, nil
}

func (s *AliyunDownloadStore) PresignGet(ctx context.Context, key, versionID, contentDisposition string, ttl time.Duration) (SignedDownload, error) {
	if ttl <= 0 || ttl > time.Minute {
		return SignedDownload{}, errors.New("download grant lifetime is invalid")
	}
	result, err := s.publicClient.Presign(ctx, &oss.GetObjectRequest{
		Bucket:                     oss.Ptr(s.bucket),
		Key:                        oss.Ptr(key),
		VersionId:                  oss.Ptr(versionID),
		ResponseContentDisposition: oss.Ptr(contentDisposition),
		ResponseCacheControl:       oss.Ptr("private, no-store"),
	}, oss.PresignExpires(ttl))
	if err != nil {
		return SignedDownload{}, safeDownloadOSSFailure("presign_get_object", err)
	}
	if result.Method != http.MethodGet {
		return SignedDownload{}, errors.New("OSS presign returned an invalid method")
	}
	return SignedDownload{URL: result.URL, ExpiresAt: result.Expiration}, nil
}

func isDownloadObjectNotFound(err error) bool {
	var serviceError *oss.ServiceError
	return errors.As(err, &serviceError) && serviceError.StatusCode == http.StatusNotFound && (serviceError.Code == "NoSuchKey" || serviceError.Code == "NoSuchVersion")
}

func safeDownloadOSSFailure(operation string, err error) error {
	var serviceError *oss.ServiceError
	if errors.As(err, &serviceError) {
		return fmt.Errorf("operation=%s category=service status=%d code=%s request_id=%s", operation, serviceError.StatusCode, safeDownloadToken(serviceError.Code), safeDownloadToken(serviceError.RequestID))
	}
	return fmt.Errorf("operation=%s category=transport", operation)
}

func safeDownloadToken(value string) string {
	if value == "" {
		return "none"
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '.' && char != '_' && char != '-' {
			return "redacted"
		}
	}
	return value
}
