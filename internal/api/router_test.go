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
	_, err := NewRouter(config.Config{CloudflareSigningKeyID: "key-id"}, nil)
	if err == nil {
		t.Fatal("expected partial Cloudflare signing config to fail")
	}
}
