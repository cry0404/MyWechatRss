package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/articles"
	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/rss"
	"github.com/cry0404/MyWechatRss/internal/subs"
)

type SubsHandlers struct {
	Svc           *subs.Service
	ArticleSvc    *articles.Service
	FeedEncoder   *rss.FeedIDEncoder
	PublicBaseURL string
}

type subscriptionDTO struct {
	*model.Subscription
	FeedID  string `json:"feed_id"`
	FeedURL string `json:"feed_url"`
}

func (h *SubsHandlers) toDTO(s *model.Subscription) subscriptionDTO {
	fid := h.FeedEncoder.Encode(s.UserID, s.ID)
	return subscriptionDTO{
		Subscription: s,
		FeedID:       fid,
		FeedURL:      strings.TrimRight(h.PublicBaseURL, "/") + "/rss/" + fid + ".xml",
	}
}

func (h *SubsHandlers) Search(c *gin.Context) {
	q := c.Query("q")
	res, err := h.Svc.Search(c.Request.Context(), auth.CurrentUserID(c), q)
	if err != nil {
		respondErr(c, http.StatusBadGateway, err)
		return
	}
	if res == nil {
		res = []subs.SearchResultItem{}
	}
	c.JSON(http.StatusOK, res)
}

type createSubReq struct {
	BookID string `json:"book_id" binding:"required"`
	Alias  string `json:"alias,omitempty"`
}

func (h *SubsHandlers) Create(c *gin.Context) {
	var req createSubReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.Svc.Create(c.Request.Context(), auth.CurrentUserID(c), req.BookID, req.Alias)
	if err != nil {
		respondErr(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, h.toDTO(sub))
}

func (h *SubsHandlers) List(c *gin.Context) {
	uid := auth.CurrentUserID(c)
	list, err := h.Svc.List(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]subscriptionDTO, 0, len(list))
	for _, s := range list {
		out = append(out, h.toDTO(s))
	}
	c.JSON(http.StatusOK, out)
}

func (h *SubsHandlers) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var p subs.UpdatePayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.Svc.Update(c.Request.Context(), auth.CurrentUserID(c), id, p)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.toDTO(sub))
}

func (h *SubsHandlers) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := h.Svc.Delete(c.Request.Context(), auth.CurrentUserID(c), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

const defaultArticlesPageLimit = 20

func (h *SubsHandlers) Articles(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	sub, err := h.Svc.Get(c.Request.Context(), auth.CurrentUserID(c), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultArticlesPageLimit)))
	if limit <= 0 {
		limit = defaultArticlesPageLimit
	}
	arts, err := h.ArticleSvc.ListByBook(c.Request.Context(), sub.BookID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if arts == nil {
		arts = []*model.Article{}
	}
	c.JSON(http.StatusOK, arts)
}

func (h *SubsHandlers) Refresh(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	n, err := h.ArticleSvc.FetchLatest(c.Request.Context(), auth.CurrentUserID(c), id)
	if err != nil {
		respondErr(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"new_count": n})
}
