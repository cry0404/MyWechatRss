package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/rss"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type AuthHandlers struct {
	Store         *store.Store
	Signer        *auth.Signer
	AllowRegister bool
	SecureCookie bool

	FeedEncoder   *rss.FeedIDEncoder
	PublicBaseURL string
}

func (h *AuthHandlers) globalFeedURL(userID int64) string {
	if h.FeedEncoder == nil {
		return ""
	}
	fid := h.FeedEncoder.Encode(userID, 0)
	return strings.TrimRight(h.PublicBaseURL, "/") + "/rss/" + fid + ".xml"
}

type registerReq struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

func (h *AuthHandlers) Register(c *gin.Context) {
	if !h.AllowRegister {
		c.JSON(http.StatusForbidden, gin.H{"error": "registration disabled"})
		return
	}
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hadUser, err := h.Store.HasAnyUser(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user := &model.User{
		Username:     req.Username,
		Email:        strings.TrimSpace(req.Email),
		PasswordHash: hash,
		IsAdmin:      !hadUser, // 第一个注册的人自动成为 admin
	}
	if err := h.Store.CreateUser(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tok, err := h.issueSession(c, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_admin":        user.IsAdmin,
		"token":           tok,
		"global_feed_url": h.globalFeedURL(user.ID),
	})
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandlers) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.Store.GetUserByUsername(c.Request.Context(), req.Username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	tok, err := h.issueSession(c, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_admin":        user.IsAdmin,
		"token":           tok,
		"global_feed_url": h.globalFeedURL(user.ID),
	})
}

func (h *AuthHandlers) Logout(c *gin.Context) {
	c.SetCookie(auth.SessionCookieName, "", -1, "/", "", h.SecureCookie, true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandlers) Me(c *gin.Context) {
	uid := auth.CurrentUserID(c)
	user, err := h.Store.GetUserByID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_admin":        user.IsAdmin,
		"global_feed_url": h.globalFeedURL(user.ID),
	})
}

type updateMeReq struct {
	Email    string `json:"email,omitempty" binding:"omitempty,email,max=254"`
	Username string `json:"username,omitempty" binding:"omitempty,min=3,max=32"`
}

func (h *AuthHandlers) UpdateMe(c *gin.Context) {
	uid := auth.CurrentUserID(c)
	var req updateMeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	if req.Email != "" {
		email := strings.TrimSpace(req.Email)
		if err := h.Store.UpdateUserEmail(ctx, uid, email); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.Username != "" {
		username := strings.TrimSpace(req.Username)
		if err := h.Store.UpdateUserUsername(ctx, uid, username); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"email":    req.Email,
		"username": req.Username,
	})
}

type updatePasswordReq struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=128"`
}

func (h *AuthHandlers) UpdatePassword(c *gin.Context) {
	uid := auth.CurrentUserID(c)
	var req updatePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.Store.GetUserByID(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !auth.VerifyPassword(user.PasswordHash, req.CurrentPassword) {
		c.JSON(http.StatusForbidden, gin.H{"error": "当前密码不正确"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.Store.UpdateUserPassword(c.Request.Context(), uid, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandlers) issueSession(c *gin.Context, uid int64) (string, error) {
	tok, err := h.Signer.Issue(uid, auth.SessionTokenTTL)
	if err != nil {
		return "", err
	}
	c.SetCookie(
		auth.SessionCookieName, tok,
		int(auth.SessionTokenTTL/time.Second),
		"/", "",
		h.SecureCookie,
		true, // httpOnly
	)
	return tok, nil
}
