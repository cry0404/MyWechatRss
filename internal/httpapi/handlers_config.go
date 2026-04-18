package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/store"
)

type ConfigHandlers struct {
	Store *store.Store
}

type smtpConfigReq struct {
	Host     string `json:"smtp_host"`
	Port     int    `json:"smtp_port"`
	Username string `json:"smtp_username"`
	Password string `json:"smtp_password"`
	From     string `json:"smtp_from"`
	UseTLS   bool   `json:"smtp_use_tls"`
}

func (h *ConfigHandlers) GetConfig(c *gin.Context) {
	cfg, err := h.Store.GetSMTPConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"smtp_host":     cfg.Host,
		"smtp_port":     cfg.Port,
		"smtp_username": cfg.Username,
		"smtp_password": cfg.Password,
		"smtp_from":     cfg.From,
		"smtp_use_tls":  cfg.UseTLS,
	})
}

func (h *ConfigHandlers) PutConfig(c *gin.Context) {
	var req smtpConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Host != "" && req.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "smtp_port required when smtp_host is set"})
		return
	}
	cfg := store.SMTPConfig{
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		From:     req.From,
		UseTLS:   req.UseTLS,
	}
	if err := h.Store.PutSMTPConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
