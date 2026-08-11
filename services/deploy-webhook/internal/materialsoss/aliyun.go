package materialsoss

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

type AliyunStore struct {
	client     *oss.Client
	bucket     string
	region     string
	endpoint   string
	httpClient *http.Client
}

func NewAliyunStore(bucket, region, endpoint, ramRole string) (*AliyunStore, error) {
	if bucket != "henukit" || region != "cn-beijing" || endpoint != "https://oss-cn-beijing-internal.aliyuncs.com" || strings.TrimSpace(ramRole) == "" {
		return nil, errors.New("fixed OSS connection configuration is invalid")
	}
	credential, credentialErr := openapicred.NewCredential(new(openapicred.Config).
		SetType("ecs_ram_role").SetRoleName(ramRole).SetDisableIMDSv1(true))
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
	cfg := oss.LoadDefaultConfig().WithCredentialsProvider(provider).WithRegion(region).WithEndpoint(endpoint)
	return &AliyunStore{client: oss.NewClient(cfg), bucket: bucket, region: region, endpoint: endpoint, httpClient: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (s *AliyunStore) BucketState(ctx context.Context) (BucketState, error) {
	result, err := s.client.GetBucketInfo(ctx, &oss.GetBucketInfoRequest{Bucket: oss.Ptr(s.bucket)})
	if err != nil {
		return BucketState{}, safeOSSFailure("get_bucket_info", err)
	}
	info := result.BucketInfo
	location := oss.ToString(info.Location)
	location = strings.TrimPrefix(location, "oss-")
	return BucketState{Region: location, ACL: oss.ToString(info.ACL), StorageClass: oss.ToString(info.StorageClass), Redundancy: oss.ToString(info.DataRedundancyType), Versioning: oss.ToString(info.Versioning), Encryption: oss.ToString(info.SseRule.SSEAlgorithm)}, nil
}

func (s *AliyunStore) Head(ctx context.Context, key, versionID string) (ObjectState, bool, error) {
	req := &oss.HeadObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key)}
	if versionID != "" {
		req.VersionId = oss.Ptr(versionID)
	}
	result, err := s.client.HeadObject(ctx, req)
	if err != nil {
		if isNotFound(err) {
			return ObjectState{}, false, nil
		}
		return ObjectState{}, false, safeOSSFailure("head_object", err)
	}
	return ObjectState{Bytes: result.ContentLength, SHA256: result.Metadata["sha256"], Encryption: oss.ToString(result.ServerSideEncryption), VersionID: oss.ToString(result.VersionId)}, true, nil
}

func (s *AliyunStore) Put(ctx context.Context, key string, body io.Reader, size int64, sha string) (string, error) {
	result, err := s.client.PutObject(ctx, &oss.PutObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key), Body: body, ContentLength: oss.Ptr(size), ServerSideEncryption: oss.Ptr("AES256"), StorageClass: oss.StorageClassStandard, Metadata: map[string]string{"sha256": sha}})
	if err != nil {
		return "", safeOSSFailure("put_object", err)
	}
	return oss.ToString(result.VersionId), nil
}

func (s *AliyunStore) Get(ctx context.Context, key, versionID string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(key), VersionId: oss.Ptr(versionID)})
	if err != nil {
		return nil, safeOSSFailure("get_object", err)
	}
	return result.Body, nil
}

func (s *AliyunStore) AnonymousDenied(ctx context.Context, key, versionID string) error {
	u := url.URL{Scheme: "https", Host: s.bucket + "." + strings.TrimPrefix(s.endpoint, "https://"), Path: "/" + key}
	query := u.Query()
	query.Set("versionId", versionID)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("could not construct anonymous OSS verification")
	}
	response, err := s.httpClient.Do(req)
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

func isNotFound(err error) bool {
	var serviceError *oss.ServiceError
	if !errors.As(err, &serviceError) {
		return false
	}
	return serviceError.StatusCode == http.StatusNotFound && (serviceError.Code == "NoSuchKey" || serviceError.Code == "NoSuchVersion")
}

func safeOSSFailure(operation string, err error) error {
	var serviceError *oss.ServiceError
	if errors.As(err, &serviceError) {
		return fmt.Errorf("operation=%s category=service status=%d code=%s request_id=%s", operation, serviceError.StatusCode, safeToken(serviceError.Code), safeToken(serviceError.RequestID))
	}
	return fmt.Errorf("operation=%s category=transport", operation)
}

func safeToken(value string) string {
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
