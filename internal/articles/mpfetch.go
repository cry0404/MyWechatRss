// mpfetch.go: 抓取 mp.weixin.qq.com 公开页面，提取正文 HTML。
// 正文链路第二优先级，仅在 weread 网页端不可用时回退。
package articles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	mpMaxBodyBytes = 10 * 1024 * 1024
	mpHTTPTimeout  = 20 * time.Second
)


const mpDesktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var mpUserAgents = []string{
	mpDesktopUA,
	// macOS Safari（微信在 macOS 上的默认浏览器）
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
	// Windows Chrome
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
}

var mpClient = func() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: mpHTTPTimeout,
		Jar:     jar,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}()

var mpVerifyMarker = []byte("mmbizwap:secitptpage/verify.html")

func buildMpURL(originalID string) string {
	if originalID == "" {
		return ""
	}
	token := strings.ReplaceAll(originalID, "~", "_")
	return "https://mp.weixin.qq.com/s/" + token
}

func fetchMpContent(ctx context.Context, articleURL string) (string, error) {
	if articleURL == "" {
		return "", errors.New("empty mp URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", err
	}

	ua := mpUserAgents[rand.Intn(len(mpUserAgents))]
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Referer", "https://mp.weixin.qq.com/")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if strings.Contains(ua, "Chrome") {
		req.Header.Set("Sec-Ch-Ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
		req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	}

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

	if bytes.Contains(body, mpVerifyMarker) {
		return "", errors.New("mp 返回环境验证页，疑似风控")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("mp parse html: %w", err)
	}

	content := doc.Find("#js_content").First()
	if content.Length() == 0 {
		return "", errors.New("mp 正文节点 #js_content 不存在")
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

// sanitizeMpContent 处理 #js_content 子树：
//   1. data-src / preview-src → src（RSS 阅读器不执行懒加载）
//   2. img 加 referrerpolicy="no-referrer" 绕过 mmbiz.qpic.cn 防盗链
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
