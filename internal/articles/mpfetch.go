// mpfetch.go：抓公开的 mp.weixin.qq.com 文章页，解析出正文 HTML。
//
// 为什么走公开页面而不是走 weread /book/shareChapter：
//
//   - 不占用 weread 私有接口的配额——批量抓正文本身就是比拉列表重得多的
//     动作，如果全压在 weread 上，很容易把账号推到风控阈值；
//   - 公开 mp 页面对我们来说是一个稳定入口，协议变化概率低；
//   - shareChapter 当前返回 data:null 比例极高，协议疑似已变，先走公开页能立刻可用。
//
// 风控关键点（全部实测验证过，缺一不可）：
//
//   - User-Agent 必须是桌面浏览器；iPhone/Android UA 会被重定向到
//     secitptpage/verify.html（微信的环境异常验证页），拿到 17KB 的空壳；
//   - 必须带 Referer: https://mp.weixin.qq.com/；
//   - 响应如命中验证页标记，直接返错不重试——retry 救不回来，只会放大风险。
//
// 调用节奏（jitter sleep）完全由上层 FetchLatest 控制（首次 2-5s / 增量 7-8s），
// 本文件不做任何 sleep，避免控制流分散。
package articles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	// mpDesktopUA 是一枚主流桌面 Chrome UA。不要换成移动 UA，也不要随机化——
	// 固定一个看起来无异常的 UA 比"每次都不一样"更像真实浏览器。
	mpDesktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

	// mpMaxBodyBytes 是单次响应 body 的硬上限。mp 文章经验值 3MB 左右，
	// 留 10MB 防止个别图文密集号拉爆内存，也能挡住意外的无限流。
	mpMaxBodyBytes = 10 * 1024 * 1024

	// mpHTTPTimeout 覆盖 DNS + TCP + TLS + 下行。
	// mp 页面约 3MB，走国内一般 2-3s，给到 20s 足够容错网络抖动。
	mpHTTPTimeout = 20 * time.Second
)

// mpClient 专用于抓公开 mp 页面的 HTTP client。
//
//   - 没有挂 cookie jar，跨请求完全无状态；
//   - 不自定义 transport，保留默认的 gzip auto-decompress；
//   - 限制 5 跳重定向，防御异常 redirect 链。
var mpClient = &http.Client{
	Timeout: mpHTTPTimeout,
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// mpVerifyMarker 出现这个字符串意味着本次请求被微信判定为"环境异常"，
// 返回的是 secitptpage/verify.html 空壳，而不是真的文章。
var mpVerifyMarker = []byte("mmbizwap:secitptpage/verify.html")

// buildMpURL 把 weread 的 originalId 映射回公开的 mp.weixin.qq.com 链接。
//
// 实测规则：originalId 就是 mp.weixin.qq.com/s/{token} 的 22 字符 base64url token，
// 唯一差异是 weread 把原本的 `_` 替换成了 `~`（可能是为了避开 URL 处理中
// 下划线的歧义）。反向 `~` → `_` 即可还原，整段 token 其它字符不动。
//
// 空 originalId 直接返回空串，由调用方决定是否走兜底链路。
func buildMpURL(originalID string) string {
	if originalID == "" {
		return ""
	}
	token := strings.ReplaceAll(originalID, "~", "_")
	return "https://mp.weixin.qq.com/s/" + token
}

// fetchMpContent 抓 mp 文章页并返回 #js_content 的 inner HTML。
//
// 返回 (html, nil) 表示成功；任何失败都返错，由上层决定是否回退 shareChapter。
// 返回的 HTML 已做懒加载图片清洗（data-src → src），直接塞进 RSS 的
// content:encoded 就能在阅读器里正常展示图片。
func fetchMpContent(ctx context.Context, articleURL string) (string, error) {
	if articleURL == "" {
		return "", errors.New("empty mp URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", err
	}
	// 下面这几个 header 组合是实测通过验证页拦截的最小集，不要随意删减。
	req.Header.Set("User-Agent", mpDesktopUA)
	req.Header.Set("Referer", "https://mp.weixin.qq.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	// 故意不设 Accept-Encoding：一旦显式设置，net/http 不会自动 gzip 解压，
	// 得自己处理。交给默认 transport 省心。

	resp, err := mpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mp http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, mpMaxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("mp read body: %w", err)
	}

	// 关键反风控点：一旦被重定向到验证页，直接放弃。继续重试只会放大风险，
	// 且验证页一般需要用户端补画验证码才能解除，代码里解决不了。
	if bytes.Contains(body, mpVerifyMarker) {
		return "", errors.New("mp 返回环境验证页，疑似风控，放弃本次抓取")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("mp parse html: %w", err)
	}

	content := doc.Find("#js_content").First()
	if content.Length() == 0 {
		return "", errors.New("mp 正文节点 #js_content 不存在（页面结构可能已变）")
	}

	sanitizeMpContent(content)

	html, err := content.Html()
	if err != nil {
		return "", fmt.Errorf("mp extract html: %w", err)
	}
	html = strings.TrimSpace(html)
	if html == "" {
		return "", errors.New("mp 正文为空")
	}
	return html, nil
}

// sanitizeMpContent 原地修改 #js_content 子树，让正文 HTML 能直接丢给 RSS 阅读器。
//
// 需要处理的两类问题都跟微信图床 mmbiz.qpic.cn 有关：
//
//  1. 懒加载：真实图片挂在 data-src 上，页面上 <img src> 其实是占位灰块；
//     RSS 阅读器不会执行懒加载脚本，所以必须在我们这一侧把 data-src 升级为 src。
//     经验里 data-src 是主要字段，preview-src 出现在个别旧模板。
//
//  2. Referer 防盗链：mmbiz.qpic.cn 会检查请求来源，如果 Referer 不是
//     mp.weixin.qq.com/*，直接返回"此图片来自微信公众号未经允许不可引用"占位图。
//     给 img 加 referrerpolicy="no-referrer" 能让浏览器/RSS 客户端发不带
//     Referer 的请求，mmbiz 反而会放行——这是公开的标准绕法，不涉及伪造。
func sanitizeMpContent(sel *goquery.Selection) {
	sel.Find("img").Each(func(_ int, s *goquery.Selection) {
		for _, attr := range []string{"data-src", "preview-src"} {
			if src, ok := s.Attr(attr); ok && src != "" {
				s.SetAttr("src", src)
				s.RemoveAttr(attr)
				break
			}
		}
		s.SetAttr("referrerpolicy", "no-referrer")
	})
}
