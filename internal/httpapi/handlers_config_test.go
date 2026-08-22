package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/crypto"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
)

func TestConfigHandlersProtectSMTPPasswordAndAdminAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	codec, err := crypto.New("test-secret-with-enough-length")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"), codec)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	admin := &model.User{Username: "admin", Email: "admin@example.com", PasswordHash: "hash", IsAdmin: true}
	member := &model.User{Username: "member", Email: "member@example.com", PasswordHash: "hash"}
	if err := st.CreateUser(ctx, admin); err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := st.CreateUser(ctx, member); err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if err := st.PutSMTPConfig(ctx, store.SMTPConfig{Host: "smtp.old.test", Port: 465, Username: "sender", Password: "top-secret", UseTLS: true}); err != nil {
		t.Fatalf("PutSMTPConfig: %v", err)
	}

	h := &ConfigHandlers{Store: st}
	requestAs := func(userID int64, method, path, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.CurrentUserIDKey, userID)
			c.Next()
		})
		r.Handle(method, path, handler)
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	memberGet := requestAs(member.ID, http.MethodGet, "/config", "", h.GetConfig)
	if memberGet.Code != http.StatusForbidden {
		t.Fatalf("member GET status=%d body=%s", memberGet.Code, memberGet.Body.String())
	}

	adminGet := requestAs(admin.ID, http.MethodGet, "/config", "", h.GetConfig)
	if adminGet.Code != http.StatusOK {
		t.Fatalf("admin GET status=%d body=%s", adminGet.Code, adminGet.Body.String())
	}
	if strings.Contains(adminGet.Body.String(), "top-secret") || strings.Contains(adminGet.Body.String(), "smtp_password\"") {
		t.Fatalf("GET leaked SMTP password: %s", adminGet.Body.String())
	}
	if !strings.Contains(adminGet.Body.String(), `"smtp_password_set":true`) {
		t.Fatalf("GET missing password-set marker: %s", adminGet.Body.String())
	}

	adminPut := requestAs(admin.ID, http.MethodPut, "/config", `{"smtp_host":"smtp.new.test","smtp_port":587,"smtp_username":"sender","smtp_from":"","smtp_use_tls":false}`, h.PutConfig)
	if adminPut.Code != http.StatusOK {
		t.Fatalf("admin PUT status=%d body=%s", adminPut.Code, adminPut.Body.String())
	}
	got, err := st.GetSMTPConfig(ctx)
	if err != nil {
		t.Fatalf("GetSMTPConfig: %v", err)
	}
	if got.Host != "smtp.new.test" || got.Password != "top-secret" {
		t.Fatalf("config after PUT = %#v, want updated host and preserved password", got)
	}
}

func TestMustChangePasswordDetectsDefaultOnly(t *testing.T) {
	defaultHash, err := auth.HashPassword(defaultBootstrapPassword)
	if err != nil {
		t.Fatalf("HashPassword default: %v", err)
	}
	customHash, err := auth.HashPassword("a-custom-password")
	if err != nil {
		t.Fatalf("HashPassword custom: %v", err)
	}
	if !mustChangePassword(&model.User{PasswordHash: defaultHash}) {
		t.Fatal("default password was not flagged")
	}
	if mustChangePassword(&model.User{PasswordHash: customHash}) {
		t.Fatal("custom password was incorrectly flagged")
	}
}
