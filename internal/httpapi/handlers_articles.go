package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/articles"
	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/model"
)

type ArticleHandlers struct {
	Articles *articles.Service
}

func (h *ArticleHandlers) List(c *gin.Context) {
	uid := auth.CurrentUserID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	arts, err := h.Articles.ListByUser(c.Request.Context(), uid, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if arts == nil {
		arts = []*model.Article{}
	}
	// 文章流列表不需要返回 content_html，避免传输大量正文 HTML。
	for _, a := range arts {
		a.ContentHTML = ""
	}
	c.JSON(http.StatusOK, arts)
}
