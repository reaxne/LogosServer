package bunny

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"logosserver/internal/config"
)

type StreamSigner struct {
	pullZone  string // e.g. "vz-abc123-def.b-cdn.net"
	libraryID string
	authKey   string // Bunny Stream "Token Authentication Key" for the pull zone
	tokenTTL  time.Duration
}

func NewStreamSigner(cfg config.Config) (StreamSigner, error) {
	if (cfg.BunnyAuthKey == "") != (cfg.BunnyPullZone == "" && cfg.BunnyLibraryID == "") {
		// auth key without pull/library info is fine to allow (validated below);
		// keep this permissive similar to old Cloudflare check style
	}
	if cfg.BunnyPullZone == "" {
		return StreamSigner{}, errors.New("BUNNY_PULL_ZONE must be configured")
	}
	return StreamSigner{
		pullZone:  strings.TrimRight(cfg.BunnyPullZone, "/"),
		libraryID: cfg.BunnyLibraryID,
		authKey:   cfg.BunnyAuthKey,
		tokenTTL:  0,
	}, nil
}

// PlaybackURL returns the iframe embed URL for a Bunny Stream video.
// If an auth key is configured, it appends a signed token + expiry for
// private/protected videos. Otherwise it returns a plain playback URL.
func (s StreamSigner) PlaybackURL(videoID string, expiresAt time.Time) (string, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return "", errors.New("empty video id")
	}
	baseURL := s.iframeBaseURL(videoID)
	if s.authKey == "" {
		return baseURL, nil
	}
	token, expUnix, err := s.token(videoID, expiresAt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?token=%s&expires=%d", baseURL, token, expUnix), nil
}

// ThumbnailURL returns the thumbnail URL for a Bunny Stream video.
// If an auth key is configured, it appends a signed pull-zone token + expiry.
func (s StreamSigner) ThumbnailURL(videoID string, expiresAt time.Time) (string, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return "", errors.New("empty video id")
	}
	path := fmt.Sprintf("/%s/thumbnail.jpg", videoID)
	baseURL := "https://" + s.pullZone + path
	if s.authKey == "" {
		return baseURL, nil
	}
	token, expUnix, err := s.pullZoneToken(path, expiresAt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?token=%s&expires=%d", baseURL, token, expUnix), nil
}

func (s StreamSigner) iframeBaseURL(videoID string) string {
	return fmt.Sprintf("https://iframe.mediadelivery.net/embed/%s/%s", s.libraryID, videoID)
}

// token implements Bunny's token authentication scheme:
// sha256(security_key + video_id + expiration_timestamp), hex-encoded.
// See: Bunny Stream "Token Authentication" docs for the pull zone.
func (s StreamSigner) token(videoID string, expiresAt time.Time) (string, int64, error) {
	if s.authKey == "" {
		return "", 0, errors.New("no auth key configured")
	}
	expUnix := expiresAt.Unix()
	raw := s.authKey + videoID + strconv.FormatInt(expUnix, 10)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), expUnix, nil
}

// pullZoneToken implements Bunny's generic Pull Zone (CDN) token authentication
// scheme — distinct from the Stream iframe embed token. It signs the URL path:
// base64url( sha256_raw(security_key + path + expires) ), no padding.
// See: https://docs.bunny.net/cdn/security/token-authentication
func (s StreamSigner) pullZoneToken(path string, expiresAt time.Time) (string, int64, error) {
	if s.authKey == "" {
		return "", 0, errors.New("no auth key configured")
	}
	expUnix := expiresAt.Unix()
	raw := s.authKey + path + strconv.FormatInt(expUnix, 10)
	sum := sha256.Sum256([]byte(raw))
	token := base64.RawURLEncoding.EncodeToString(sum[:]) // already -/_ , no padding
	return token, expUnix, nil
}
