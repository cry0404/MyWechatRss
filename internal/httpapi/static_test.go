package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServeStaticKeepsSPADeepLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	serveStatic(router)

	for _, target := range []string{"/accounts", "/subscriptions/123"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", target, resp.Code, http.StatusOK)
		}
		if location := resp.Header().Get("Location"); location != "" {
			t.Fatalf("GET %s unexpectedly redirected to %q", target, location)
		}
		if contentType := resp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Fatalf("GET %s Content-Type = %q, want text/html", target, contentType)
		}
		if cacheControl := resp.Header().Get("Cache-Control"); cacheControl != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q, want no-cache", target, cacheControl)
		}
		if !strings.Contains(resp.Body.String(), "<div id=\"root\"></div>") {
			t.Fatalf("GET %s did not return the SPA index", target)
		}
	}
}

func TestServeStaticPreservesAPINotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	serveStatic(router)

	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET /api/missing status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}
