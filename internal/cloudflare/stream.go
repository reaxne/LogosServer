package cloudflare

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"logosserver/internal/config"
)

type StreamSigner struct {
	subdomain    string
	signingKeyID string
	privateKey   *rsa.PrivateKey
}

func NewStreamSigner(cfg config.Config) (StreamSigner, error) {
	if (cfg.CloudflareSigningKeyID == "") != (cfg.CloudflareSigningKeyPEM == "") {
		return StreamSigner{}, errors.New("Cloudflare signing key ID and PEM must be configured together")
	}
	key, err := parsePrivateKey(cfg.CloudflareSigningKeyPEM)
	if err != nil {
		return StreamSigner{}, fmt.Errorf("parse Cloudflare signing key: %w", err)
	}
	return StreamSigner{
		subdomain:    cfg.CloudflareStreamSubdomain,
		signingKeyID: cfg.CloudflareSigningKeyID,
		privateKey:   key,
	}, nil
}

func (s StreamSigner) PlaybackURL(streamUID string, expiresAt time.Time) (string, error) {
	streamUID = strings.TrimSpace(streamUID)
	if streamUID == "" {
		return "", errors.New("empty stream uid")
	}
	baseURL := s.playbackBaseURL(streamUID)
	if s.signingKeyID == "" || s.privateKey == nil {
		return baseURL, nil
	}
	token, err := s.token(streamUID, expiresAt)
	if err != nil {
		return "", err
	}
	return baseURL + "?token=" + token, nil
}

func (s StreamSigner) ThumbnailURL(streamUID string) string {
	streamUID = strings.TrimSpace(streamUID)
	if streamUID == "" {
		return ""
	}
	if s.subdomain == "" {
		return "https://videodelivery.net/" + streamUID + "/thumbnails/thumbnail.jpg"
	}
	return "https://" + strings.TrimRight(s.subdomain, "/") + "/" + streamUID + "/thumbnails/thumbnail.jpg"
}

func (s StreamSigner) playbackBaseURL(streamUID string) string {
	if s.subdomain == "" {
		return "https://iframe.videodelivery.net/" + streamUID
	}
	return "https://" + strings.TrimRight(s.subdomain, "/") + "/" + streamUID + "/iframe"
}

func (s StreamSigner) token(streamUID string, expiresAt time.Time) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"kid": s.signingKeyID,
		"typ": "JWT",
	}
	payload := map[string]any{
		"sub": streamUID,
		"kid": s.signingKeyID,
		"exp": expiresAt.Unix(),
	}
	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)
	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)

	hash := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parsePrivateKey(value string) (*rsa.PrivateKey, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("invalid PEM")
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
		return nil, fmt.Errorf("expected RSA private key, got %T", parsed)
	}
	return key, nil
}
