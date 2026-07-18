package objectstorage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

type Signer struct{ config Config }

func New(config Config) (*Signer, error) {
	if config.Endpoint == "" || config.Bucket == "" || config.AccessKey == "" || config.SecretKey == "" {
		return nil, errors.New("object storage endpoint, bucket and credentials are required")
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("invalid object storage endpoint")
	}
	if config.Region == "" {
		config.Region = "us-east-1"
	}
	return &Signer{config: config}, nil
}

func (signer *Signer) PresignPut(objectKey string, expires time.Duration, now time.Time) (string, error) {
	if signer == nil || objectKey == "" || strings.Contains(objectKey, "..") || strings.HasPrefix(objectKey, "/") {
		return "", errors.New("invalid object key")
	}
	if expires <= 0 || expires > 15*time.Minute {
		return "", errors.New("presign expiry must be between 1s and 15m")
	}
	endpoint, _ := url.Parse(signer.config.Endpoint)
	canonicalURI := "/" + strings.TrimPrefix(path.Join(endpoint.Path, signer.config.Bucket, objectKey), "/")
	date := now.UTC().Format("20060102")
	amzDate := now.UTC().Format("20060102T150405Z")
	scope := date + "/" + signer.config.Region + "/s3/aws4_request"
	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", signer.config.AccessKey+"/"+scope)
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", strconv.FormatInt(int64(expires/time.Second), 10))
	query.Set("X-Amz-SignedHeaders", "host")
	canonicalRequest := "PUT\n" + canonicalURI + "\n" + query.Encode() + "\nhost:" + endpoint.Host + "\n\nhost\nUNSIGNED-PAYLOAD"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	signature := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+signer.config.SecretKey), date), signer.config.Region), "s3"), "aws4_request")
	finalSignature := hmacSHA256(signature, stringToSign)
	query.Set("X-Amz-Signature", hex.EncodeToString(finalSignature))
	endpoint.Path = canonicalURI
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
