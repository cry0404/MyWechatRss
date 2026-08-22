package httpapi

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss"
)

// staticFS 去掉嵌入路径前缀，让 / 对应 web/dist/index.html。
var staticFS, _ = fs.Sub(app.DistFS, "web/dist")

// serveStatic 把前端 SPA 挂到 gin 的 NoRoute 上。
//
// 为什么不用 r.StaticFS("/", ...)：那会让 gin 注册 /*filepath 这个 catch-all
// 路由，跟已有的 /api/* 和 /rss/* 在同一个 radix tree 里打架（gin 会直接 panic：
// "catch-all wildcard '*filepath' conflicts with existing path segment 'api'"）。
//
// 用 NoRoute 就把 SPA 只放在"其它路由都没 match 上"的位置，天然避让 API 前缀。
// 同时手动实现 SPA 兜底：任何不存在的子路径（/rss-feeds、/subscriptions/...）
// 都回退到 index.html，让前端 router 接管。
func serveStatic(r *gin.Engine) {
	fsys := http.FS(staticFS)
	fileServer := http.FileServer(fsys)
	indexHTML, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		panic("embedded web/dist/index.html is missing: " + err.Error())
	}
	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}

	r.NoRoute(func(c *gin.Context) {
		// API 和 RSS 路径下的 404 保持语义化——不要给前端 HTML，避免客户端误解析。
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/rss/") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		// 根路径与不存在的前端路由直接返回 index.html 内容。不要把 URL.Path
		// 改成 /index.html 后再交给 FileServer；FileServer 会根据原始 RequestURI
		// 把 /accounts 之类的深链接 301 到 ./，导致浏览器丢失目标路由。
		if path == "/" {
			serveIndex(c)
			return
		}
		if _, err := fs.Stat(staticFS, strings.TrimPrefix(path, "/")); err != nil {
			serveIndex(c)
			return
		}

		// 存在的静态文件仍交给 FileServer，以保留 Content-Type、缓存与范围请求处理。
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
