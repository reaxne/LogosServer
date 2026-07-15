package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port string

	DatabaseURL string
	PublicURL   string
	SiteOrigins []string
	AdminToken  string
	SuccessURL  string
	FailureURL  string

	FreedomPayMerchantID     string
	FreedomPaySecretKey      string
	FreedomPayInitURL        string
	FreedomPayInitScript     string
	FreedomPayCallbackScript string
	Currency                 string
	PaymentLifetime          time.Duration

	BunnyPullZone         string // e.g. vz-xxxxx.b-cdn.net (from Bunny Stream pull zone)
	BunnyLibraryID        string // numeric Stream Library ID
	BunnyAuthKey          string // pull zone "Token authentication key" (optional, for private videos)
	PlaybackTokenLifetime time.Duration
}

func Load() Config {
	cfg := Config{
		Port:                     getEnv("PORT", "8080"),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		PublicURL:                strings.TrimRight(os.Getenv("PUBLIC_URL"), "/"),
		SiteOrigins:              splitCSV(os.Getenv("SITE_ORIGINS")),
		AdminToken:               os.Getenv("ADMIN_TOKEN"),
		SuccessURL:               strings.TrimSpace(os.Getenv("PAYMENT_SUCCESS_URL")),
		FailureURL:               strings.TrimSpace(os.Getenv("PAYMENT_FAILURE_URL")),
		FreedomPayMerchantID:     os.Getenv("FREEDOMPAY_MERCHANT_ID"),
		FreedomPaySecretKey:      os.Getenv("FREEDOMPAY_SECRET_KEY"),
		FreedomPayInitURL:        getEnv("FREEDOMPAY_INIT_URL", "https://api.freedompay.money/init_payment.php"),
		FreedomPayInitScript:     getEnv("FREEDOMPAY_INIT_SCRIPT", "init_payment.php"),
		FreedomPayCallbackScript: getEnv("FREEDOMPAY_CALLBACK_SCRIPT", "payment.php"),
		Currency:                 getEnv("PAYMENT_CURRENCY", "KZT"),
		PaymentLifetime:          durationEnv("PAYMENT_LIFETIME", 30*time.Minute),
		BunnyPullZone:            os.Getenv("BUNNY_PULL_ZONE"),
		BunnyLibraryID:           os.Getenv("BUNNY_LIBRARY_ID"),
		BunnyAuthKey:             os.Getenv("BUNNY_AUTH_KEY"),
		PlaybackTokenLifetime:    durationEnv("PLAYBACK_TOKEN_LIFETIME", 24*time.Hour),
	}
	return cfg
}

func (c Config) FreedomPayConfigured() bool {
	return strings.TrimSpace(c.FreedomPayMerchantID) != "" && strings.TrimSpace(c.FreedomPaySecretKey) != ""
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
