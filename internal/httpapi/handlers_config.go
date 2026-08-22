package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/notify"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type ConfigHandlers struct {
	Store *store.Store
}

type smtpConfigReq struct {
	Host     string  `json:"smtp_host"`
	Port     int     `json:"smtp_port"`
	Username string  `json:"smtp_username"`
	Password *string `json:"smtp_password"`
	From     string  `json:"smtp_from"`
	UseTLS   bool    `json:"smtp_use_tls"`
}

func (h *ConfigHandlers) requireAdmin(c *gin.Context) (*store.SMTPConfig, bool) {
	user, err := h.Store.GetUserByID(c.Request.Context(), auth.CurrentUserID(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, false
	}
	if !user.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可以管理邮件告警配置"})
		return nil, false
	}
	cfg, err := h.Store.GetSMTPConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	return cfg, true
}

func (h *ConfigHandlers) GetConfig(c *gin.Context) {
	cfg, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"smtp_host":         cfg.Host,
		"smtp_port":         cfg.Port,
		"smtp_username":     cfg.Username,
		"smtp_password_set": cfg.Password != "",
		"smtp_from":         cfg.From,
		"smtp_use_tls":      cfg.UseTLS,
	})
}

func (h *ConfigHandlers) PutConfig(c *gin.Context) {
	current, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var req smtpConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Host != "" && req.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "smtp_port required when smtp_host is set"})
		return
	}
	password := current.Password
	if req.Password != nil {
		password = *req.Password
	}
	cfg := store.SMTPConfig{
		Host:     strings.TrimSpace(req.Host),
		Port:     req.Port,
		Username: strings.TrimSpace(req.Username),
		Password: password,
		From:     strings.TrimSpace(req.From),
		UseTLS:   req.UseTLS,
	}
	if err := h.Store.PutSMTPConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ConfigHandlers) TestEmail(c *gin.Context) {
	cfg, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	user, err := h.Store.GetUserByID(c.Request.Context(), auth.CurrentUserID(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if strings.TrimSpace(user.Email) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先设置告警邮箱"})
		return
	}
	if cfg.Host == "" || cfg.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先保存完整的 SMTP 配置"})
		return
	}
	mailer := notify.NewSMTP(notify.SMTPConfig{
		Host: cfg.Host, Port: cfg.Port, Username: cfg.Username,
		Password: cfg.Password, From: cfg.From, UseTLS: cfg.UseTLS,
	})
	if err := mailer.Test(c.Request.Context(), user.Email); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "测试邮件发送失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
