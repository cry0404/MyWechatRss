package config

import "testing"

func TestLoadDefaultsContentFetchModeToSummary(t *testing.T) {
	t.Setenv("APP_SECRET", "test-secret-with-enough-length")
	t.Setenv("JWT_SECRET", "test-jwt-secret-with-enough-length")
	t.Setenv("UPSTREAM_BASE_URL", "http://upstream.test")
	t.Setenv("UPSTREAM_API_KEY_ID", "key-id")
	t.Setenv("UPSTREAM_API_SECRET", "api-secret")
	t.Setenv("CONTENT_FETCH_MODE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ContentFetchMode != FetchModeSummary {
		t.Fatalf("ContentFetchMode=%q want %q", cfg.ContentFetchMode, FetchModeSummary)
	}
}
