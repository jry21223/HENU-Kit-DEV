package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"final-review-platform/services/api/pkg/config"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Type         string `json:"type"`
	TokenVersion int    `json:"tokenVersion"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	issuer     string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(cfg config.Config) (*TokenManager, error) {
	privateKey, publicKey, err := loadRSAKeys(cfg)
	if err != nil {
		return nil, err
	}

	return &TokenManager{
		issuer:     cfg.JWT.Issuer,
		privateKey: privateKey,
		publicKey:  publicKey,
		accessTTL:  time.Duration(cfg.JWT.AccessTTLMinutes) * time.Minute,
		refreshTTL: time.Duration(cfg.JWT.RefreshTTLHours) * time.Hour,
	}, nil
}

func (m *TokenManager) Issue(userID string, email string, role string, tokenType string, tokenVersion int) (string, time.Time, error) {
	ttl := m.accessTTL
	if tokenType == TokenTypeRefresh {
		ttl = m.refreshTTL
	}
	expiresAt := time.Now().Add(ttl)
	claims := Claims{
		UserID:       userID,
		Email:        email,
		Role:         role,
		Type:         tokenType,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(m.privateKey)
	return signed, expiresAt, err
}

func (m *TokenManager) Parse(tokenText string, expectedType string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenText, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, errors.New("unexpected jwt signing method")
		}
		return m.publicKey, nil
	}, jwt.WithIssuer(m.issuer))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	if claims.Type != expectedType {
		return nil, errors.New("unexpected token type")
	}
	return claims, nil
}

func loadRSAKeys(cfg config.Config) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privatePEM, err := readSecret(cfg.JWT.PrivateKeyPEM, cfg.JWT.PrivateKeyPath)
	if err != nil {
		return nil, nil, err
	}
	publicPEM, err := readSecret(cfg.JWT.PublicKeyPEM, cfg.JWT.PublicKeyPath)
	if err != nil {
		return nil, nil, err
	}

	if privatePEM == "" {
		if cfg.Environment == "production" {
			return nil, nil, errors.New("JWT_PRIVATE_KEY or JWT_PRIVATE_KEY_PATH is required in production")
		}
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
		return key, &key.PublicKey, nil
	}

	privateKey, err := parsePrivateKey([]byte(privatePEM))
	if err != nil {
		return nil, nil, err
	}
	if publicPEM == "" {
		return privateKey, &privateKey.PublicKey, nil
	}
	publicKey, err := parsePublicKey([]byte(publicPEM))
	if err != nil {
		return nil, nil, err
	}
	return privateKey, publicKey, nil
}

func readSecret(value string, path string) (string, error) {
	if value != "" {
		return value, nil
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid private key pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return key, nil
}

func parsePublicKey(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid public key pem")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not RSA")
	}
	return key, nil
}
