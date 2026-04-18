package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type LogsHandlers struct {
	Store *store.Store
}

func (h *LogsHandlers) ListLogs(c *gin.Context) {
	reviewID := c.Query("review_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, err := h.Store.ListArticleFetchLogs(c.Request.Context(), reviewID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = []*model.ArticleFetchLog{}
	}
	c.JSON(http.StatusOK, logs)
}

func (h *LogsHandlers) Stats(c *gin.Context) {
	now := time.Now().Unix()
	since, _ := strconv.ParseInt(c.DefaultQuery("since", ""), 10, 64)
	until, _ := strconv.ParseInt(c.DefaultQuery("until", ""), 10, 64)
	if since == 0 {
		since = now - 24*3600 // 默认最近 24 小时
	}
	if until == 0 {
		until = now
	}

	stats, err := h.Store.GetFetchStats(c.Request.Context(), since, until)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if stats == nil {
		stats = []*model.FetchStats{}
	}

	windowSec, _ := strconv.ParseInt(c.DefaultQuery("window_sec", "1800"), 10, 64)
	if windowSec <= 0 {
		windowSec = 1800
	}
	failRate, err := h.Store.GetRecentFailureRate(c.Request.Context(), windowSec)
	if err != nil {
		failRate = -1
	}

	c.JSON(http.StatusOK, gin.H{
		"stats":       stats,
		"fail_rate":   failRate,
		"window_sec":  windowSec,
		"since":       since,
		"until":       until,
	})
}
