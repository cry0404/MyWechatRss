package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/articles"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/rss"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type RSSHandlers struct {
	Store         *store.Store
	Articles      *articles.Service
	FeedEncoder   *rss.FeedIDEncoder
	PublicBaseURL string
}

const aggregateFeedLimit = 100

func (h *RSSHandlers) Serve(c *gin.Context) {
	raw := c.Param("feedId")
	raw = strings.TrimSuffix(raw, ".xml")
	userID, subID, err := h.FeedEncoder.Decode(raw)
	if err != nil {
		c.String(http.StatusNotFound, "feed not found")
		return
	}
	selfURL := strings.TrimRight(h.PublicBaseURL, "/") + c.Request.URL.Path
	opt := rss.RenderOptions{PublicBaseURL: h.PublicBaseURL, SelfURL: selfURL}

	var xml []byte
	if subID == 0 {
		xml, err = h.renderAggregate(c, userID, opt)
	} else {
		xml, err = h.renderSingle(c, userID, subID, opt)
	}
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Data(http.StatusOK, "application/rss+xml; charset=utf-8", xml)
}

func (h *RSSHandlers) renderSingle(c *gin.Context, userID, subID int64, opt rss.RenderOptions) ([]byte, error) {
	sub, err := h.Store.GetSubscription(c.Request.Context(), userID, subID)
	if err != nil {
		return nil, err
	}
	arts, err := h.Store.ListArticlesByBook(c.Request.Context(), sub.BookID, 50, 0)
	if err != nil {
		return nil, err
	}
	return rss.RenderSubscription(sub, arts, opt)
}

func (h *RSSHandlers) renderAggregate(c *gin.Context, userID int64, opt rss.RenderOptions) ([]byte, error) {
	user, err := h.Store.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		return nil, err
	}
	subs, err := h.Store.ListSubscriptionsByUser(c.Request.Context(), userID)
	if err != nil {
		return nil, err
	}
	subByBook := make(map[string]*model.Subscription, len(subs))
	for _, s := range subs {
		subByBook[s.BookID] = s
	}
	arts, err := h.Store.ListArticlesByUser(c.Request.Context(), userID, aggregateFeedLimit, 0)
	if err != nil {
		return nil, err
	}
	return rss.RenderAggregate(user.Username, arts, subByBook, opt)
}
