package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"logosserver/internal/config"
)

func TestRouterHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := NewRouter(config.Config{}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRouterReadyReportsUnavailableWhenDependenciesMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := NewRouter(config.Config{}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestRouterCORSPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := NewRouter(config.Config{SiteOrigins: []string{"https://example.com"}}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/orders", nil)
	req.Header.Set("Origin", "https://example.com")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("expected CORS origin header, got %q", got)
	}
}

func TestRouterMethodNotAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := NewRouter(config.Config{}, nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/orders", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestRouterRejectsPartialCloudflareSigningConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, err := NewRouter(config.Config{BunnyAuthKey: "key-id"}, nil)
	if err == nil {
		t.Fatal("expected partial Cloudflare signing config to fail")
	}
}

func TestPaymentReturnURL(t *testing.T) {
	server := Server{cfg: config.Config{PublicURL: "https://api.example.com"}}
	ctx := &gin.Context{Request: httptest.NewRequest(http.MethodPost, "/api/orders", nil)}

	got := server.paymentReturnURL(ctx, "https://site.example.com/success?source=pay", "/payment/success", 42)
	want := "https://site.example.com/success?order_id=42&source=pay"
	if got != want {
		t.Fatalf("paymentReturnURL() = %q, want %q", got, want)
	}

	got = server.paymentReturnURL(ctx, "", "/payment/success", 42)
	want = "https://api.example.com/payment/success?order_id=42"
	if got != want {
		t.Fatalf("fallback paymentReturnURL() = %q, want %q", got, want)
	}
}

func TestPublicURLFallsBackToForwardedHeaders(t *testing.T) {
	server := Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/orders", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "server.up.railway.app")
	ctx := &gin.Context{Request: req}

	got := server.publicURL(ctx)
	want := "https://server.up.railway.app"
	if got != want {
		t.Fatalf("publicURL() = %q, want %q", got, want)
	}
}

func TestNormalizePhoneNumber(t *testing.T) {
	tests := map[string]string{
		"+7 700 123 45 67": "+77001234567",
		"87001234567":      "+77001234567",
		"7001234567":       "+77001234567",
		"0077001234567":    "+77001234567",
	}

	for input, want := range tests {
		got, err := normalizePhoneNumber(input)
		if err != nil {
			t.Fatalf("normalizePhoneNumber(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizePhoneNumber(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizePhoneNumberRejectsInvalidValue(t *testing.T) {
	if _, err := normalizePhoneNumber("not-a-phone"); err == nil {
		t.Fatal("expected invalid phone number to fail")
	}
}
