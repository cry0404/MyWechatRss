package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/articles"
	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/config"
	"github.com/cry0404/MyWechatRss/internal/rss"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/subs"
)

// NewRouter 搭建 client 的完整 HTTP 路由。
//
// 拆分点：
//   - /api/*    前端 REST，需要 session
//   - /rss/*    RSS 阅读器订阅，无需 session（feedId 自带 HMAC 签名）
//   - 其他      静态资源 / healthcheck
func NewRouter(
	cfg *config.Config,
	st *store.Store,
	signer *auth.Signer,
	accSvc *accounts.Service,
	subSvc *subs.Service,
	artSvc *articles.Service,
	feedEnc *rss.FeedIDEncoder,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	secureCookie := strings.HasPrefix(cfg.PublicBaseURL, "https://")

	authH := &AuthHandlers{
		Store:         st,
		Signer:        signer,
		AllowRegister: cfg.AllowRegister,
		SecureCookie:  secureCookie,
		FeedEncoder:   feedEnc,
		PublicBaseURL: cfg.PublicBaseURL,
	}
	accH := &AccountHandlers{Svc: accSvc, QR: NewQRSessionStore()}
	subH := &SubsHandlers{
		Svc:           subSvc,
		ArticleSvc:    artSvc,
		FeedEncoder:   feedEnc,
		PublicBaseURL: cfg.PublicBaseURL,
	}
	rssH := &RSSHandlers{
		Store:         st,
		Articles:      artSvc,
		FeedEncoder:   feedEnc,
		PublicBaseURL: cfg.PublicBaseURL,
	}

	api := r.Group("/api")
	{
		api.POST("/auth/register", authH.Register)
		api.POST("/auth/login", authH.Login)
		api.POST("/auth/logout", authH.Logout)

		authed := api.Group("", auth.RequireUser(signer))
		{
			authed.GET("/auth/me", authH.Me)
			authed.PUT("/auth/me", authH.UpdateMe)
			authed.PUT("/auth/me/password", authH.UpdatePassword)

			authed.POST("/accounts/login/start", accH.Start)
			authed.GET("/accounts/login/poll", accH.Poll)
			authed.GET("/accounts", accH.List)
			authed.DELETE("/accounts/:id", accH.Delete)

			authed.GET("/search", subH.Search)
			authed.GET("/subscriptions", subH.List)
			authed.POST("/subscriptions", subH.Create)
			authed.PATCH("/subscriptions/:id", subH.Update)
			authed.DELETE("/subscriptions/:id", subH.Delete)
			authed.GET("/subscriptions/:id/articles", subH.Articles)
			authed.POST("/subscriptions/:id/refresh", subH.Refresh)

			artH := &ArticleHandlers{Articles: artSvc}
			authed.GET("/articles", artH.List)

			logsH := &LogsHandlers{Store: st}
			authed.GET("/fetch-logs", logsH.ListLogs)
			authed.GET("/fetch-stats", logsH.Stats)

			cfgH := &ConfigHandlers{Store: st}
			authed.GET("/config", cfgH.GetConfig)
			authed.PUT("/config", cfgH.PutConfig)
		}
	}

	r.GET("/rss/:feedId", rssH.Serve)

	serveStatic(r)

	return r
}
