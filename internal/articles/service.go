package articles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
)

type Service struct {
	Store  *store.Store
	Caller *accounts.Caller
	Mode   string
}

func NewService(st *store.Store, cr *accounts.Caller, mode string) *Service {
	return &Service{Store: st, Caller: cr, Mode: mode}
}

var (
	firstFetchSleepMin = 5 * time.Second
	firstFetchSleepMax = 12 * time.Second
	incrFetchSleepMin = 15 * time.Second
	incrFetchSleepMax = 25 * time.Second
	firstFetchMax = 30
	incrFetchPage = 20
)

func (s *Service) FetchLatest(ctx context.Context, userID, subID int64) (int, error) {
	sub, err := s.Store.GetSubscription(ctx, userID, subID)
	if err != nil {
		return 0, err
	}
	firstRun := sub.LastReviewTime == 0

	acc, err := s.Store.PickActiveAccount(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("pick account: %w", err)
	}
	preferID := acc.ID

	reviews, err := s.fetchReviewList(ctx, userID, preferID, sub.BookID, incrFetchPage, 0)
	if err != nil {
		return 0, err
	}

	if firstRun && len(reviews) < firstFetchMax {
		more, err := s.fetchReviewList(ctx, userID, preferID, sub.BookID, incrFetchPage, len(reviews))
		if err == nil {
			reviews = append(reviews, more...)
		}
	}

	sortReviewsDesc(reviews)

	var todo []reviewItem
	for _, r := range reviews {
		if r.ReviewID == "" {
			continue
		}
		if !firstRun && r.PublishAt <= sub.LastReviewTime {
			continue
		}
		todo = append(todo, r)
		if firstRun && len(todo) >= firstFetchMax {
			break
		}
	}

	sleepMin, sleepMax := incrFetchSleepMin, incrFetchSleepMax
	if firstRun {
		sleepMin, sleepMax = firstFetchSleepMin, firstFetchSleepMax
	}

	var newCount int
	var maxTime int64 = sub.LastReviewTime

	for i, r := range todo {
		if ctx.Err() != nil {
			break
		}
		if i > 0 {
			jitterSleep(ctx, sleepMin, sleepMax)
			if ctx.Err() != nil {
				break
			}
		}

		if r.PublishAt > maxTime {
			maxTime = r.PublishAt
		}

		article := &model.Article{
			BookID:    sub.BookID,
			ReviewID:  r.ReviewID,
			Title:     r.Title,
			Summary:   r.Summary,
			CoverURL:  r.CoverURL,
			URL:       r.URL,
			PublishAt: r.PublishAt,
			ReadNum:   r.ReadNum,
			LikeNum:   r.LikeNum,
		}
		isNew, err := s.Store.UpsertArticle(ctx, article)
		if err != nil {
			log.Printf("fetch sub %d: upsert article %s: %v", sub.ID, r.ReviewID, err)
			continue
		}
		if isNew {
			newCount++
		}

		if err := s.fetchAndStoreContent(ctx, userID, preferID, r.ReviewID, r.URL); err != nil {
			log.Printf("fetch sub %d: content %s: %v", sub.ID, r.ReviewID, err)
		}
	}

	if err := s.Store.UpdateSubscriptionFetchState(ctx, sub.ID, time.Now().Unix(), maxTime); err != nil {
		return newCount, err
	}
	return newCount, nil
}

type reviewItem struct {
	ReviewID  string
	Title     string
	Summary   string
	CoverURL  string
	URL       string // 原文 mp.weixin.qq.com 链接，由 buildMpURL(mpInfo.originalId) 得到
	PublishAt int64
	ReadNum   int64
	LikeNum   int64
}

func (s *Service) fetchReviewList(ctx context.Context, userID, preferAccountID int64, bookID string, count, offset int) ([]reviewItem, error) {
	res, err := s.Caller.Do(ctx, userID, accounts.CallOptions{
		Method: http.MethodGet,
		Path:   "/book/articles",
		Query: map[string]string{
			"bookId":  bookID,
			"count":   strconv.Itoa(count),
			"offset":  strconv.Itoa(offset),
			"synckey": "0",
			"version": "2",
		},
		PreferAccountID: preferAccountID,
	})
	if err != nil {
		return nil, err
	}

	var raw struct {
		Reviews []struct {
			SubReviews []struct {
				Review struct {
					ReviewID   string                 `json:"reviewId"`
					CreateTime int64                  `json:"createTime"`
					MpInfo     map[string]interface{} `json:"mpInfo"`
				} `json:"review"`
			} `json:"subReviews"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(res.RawJSON, &raw); err != nil {
		return nil, fmt.Errorf("parse /book/articles: %w", err)
	}

	if len(raw.Reviews) > 0 && len(raw.Reviews[0].SubReviews) > 0 {
		r0 := raw.Reviews[0].SubReviews[0].Review
		dumpMpInfoOnce(r0.ReviewID, r0.CreateTime, r0.MpInfo)
	}

	var items []reviewItem
	for _, review := range raw.Reviews {
		for _, sub := range review.SubReviews {
			r := sub.Review
			mp := r.MpInfo
			publishAt := asInt64(mp["time"])
			if publishAt == 0 {
				publishAt = r.CreateTime
			}
			items = append(items, reviewItem{
				ReviewID:  r.ReviewID,
				Title:     asString(mp["title"]),
				Summary:   asString(mp["content"]),
				CoverURL:  asString(mp["pic_url"]),
				URL:       buildMpURL(asString(mp["originalId"])),
				PublishAt: publishAt,
				ReadNum:   asInt64(mp["readNum"]),
				LikeNum:   asInt64(mp["likeNum"]),
			})
		}
	}
	return items, nil
}

func (s *Service) fetchAndStoreContent(ctx context.Context, userID, preferAccountID int64, reviewID, mpURL string) error {
	if s.Mode == "summary" {
		return nil
	}

	var lastErr error
	var html string

	html, lastErr = s.fetchContentViaWebContent(ctx, userID, reviewID)
	if lastErr == nil {
		return s.Store.UpdateArticleContent(ctx, reviewID, html)
	}
	log.Printf("fetch content %s: web failed: %v", reviewID, lastErr)

	if mpURL != "" {
		html, lastErr = fetchMpContent(ctx, mpURL)
		if lastErr == nil {
			return s.Store.UpdateArticleContent(ctx, reviewID, html)
		}
		log.Printf("fetch content %s: mp failed: %v", reviewID, lastErr)
	}

	lastErr = s.fetchContentViaShareChapter(ctx, userID, preferAccountID, reviewID)
	if lastErr != nil {
		log.Printf("fetch content %s: all chains failed", reviewID)
	}
	return lastErr
}

// webContentClient 用于访问 weread 网页端接口（weread.qq.com/web/*）。
// 与 mpClient 不同：这里需要 Cookie jar 维持 wr_vid / wr_skey 会话。
var webContentClient = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

func init() {

	jar, err := cookiejar.New(nil)
	if err == nil {
		webContentClient.Jar = jar
	}
}

// fetchContentViaWebContent 通过 weread 网页端 /web/mp/content 获取公众号正文。
func (s *Service) fetchContentViaWebContent(ctx context.Context, userID int64, reviewID string) (string, error) {
	acc, err := s.Store.PickActiveAccount(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("pick account: %w", err)
	}

	url := "https://weread.qq.com/web/mp/content?reviewId=" + reviewID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	// 网页端用桌面浏览器 UA（不是 iOS App UA）
	req.Header.Set("User-Agent", mpDesktopUA)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://weread.qq.com/web/reader/")
	cookieVal := fmt.Sprintf("wr_vid=%d; wr_skey=%s", acc.VID, acc.SKey)
	req.Header.Set("Cookie", cookieVal)

	resp, err := webContentClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("web content request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web content http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, mpMaxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("web content read body: %w", err)
	}

	// 网页端 /web/mp/content 返回的是微信公众号文章页的完整 HTML（约 3MB），
	// 不是 JSON。需要用 goquery 提取 #js_content 节点，和 mpfetch.go 一样。
	// 先检查是不是验证页。
	if bytes.Contains(body, mpVerifyMarker) {
		return "", errors.New("web content 返回环境验证页，疑似风控")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("web content parse html: %w", err)
	}

	content := doc.Find("#js_content").First()
	if content.Length() == 0 {
		return "", errors.New("web content 正文节点 #js_content 不存在")
	}

	sanitizeMpContent(content)

	html, err := content.Html()
	if err != nil {
		return "", fmt.Errorf("web content extract html: %w", err)
	}
	html = strings.TrimSpace(html)
	if html == "" {
		return "", errors.New("web content 正文为空")
	}
	return html, nil
}

func (s *Service) fetchContentViaShareChapter(ctx context.Context, userID, preferAccountID int64, reviewID string) error {
	// 尝试多种参数组合，因为 shareChapter 协议可能已变
	attempts := []struct {
		name  string
		query map[string]string
	}{
		{"original", map[string]string{"cmd": "get", "reviewId": reviewID}},
		{"with_version", map[string]string{"cmd": "get", "reviewId": reviewID, "version": "2"}},
		{"with_synckey", map[string]string{"cmd": "get", "reviewId": reviewID, "synckey": "0"}},
		{"with_both", map[string]string{"cmd": "get", "reviewId": reviewID, "version": "2", "synckey": "0"}},
	}

	var lastErr error
	for _, att := range attempts {
		res, err := s.Caller.Do(ctx, userID, accounts.CallOptions{
			Method:          http.MethodGet,
			Path:            "/book/shareChapter",
			Query:           att.query,
			PreferAccountID: preferAccountID,
		})
		if err != nil {
			lastErr = fmt.Errorf("shareChapter %s: %w", att.name, err)
			continue
		}

		var raw struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(res.RawJSON, &raw); err != nil {
			lastErr = fmt.Errorf("shareChapter %s parse: %w", att.name, err)
			continue
		}

		content := asString(raw.Data["content"])
		if content != "" {
			if err := s.Store.UpdateArticleContent(ctx, reviewID, content); err != nil {
				return err
			}
			for _, k := range []string{"url", "mpUrl", "shareUrl", "wxUrl", "link"} {
				if u := asString(raw.Data[k]); u != "" {
					_ = s.Store.UpdateArticleURL(ctx, reviewID, u)
					break
				}
			}
			return nil
		}
		lastErr = fmt.Errorf("shareChapter %s: 响应无正文", att.name)
	}

	if lastErr != nil {
		return lastErr
	}
	return errors.New("shareChapter 所有参数组合均失败")
}

func (s *Service) EnsureContent(ctx context.Context, userID int64, reviewID string) (*model.Article, error) {
	a, err := s.Store.GetArticleByReviewID(ctx, reviewID)
	if err != nil {
		return nil, err
	}
	if a.ContentHTML != "" {
		return a, nil
	}
	if err := s.fetchAndStoreContent(ctx, userID, 0, reviewID, a.URL); err != nil {
		return a, err
	}
	return s.Store.GetArticleByReviewID(ctx, reviewID)
}

func (s *Service) ListByBook(ctx context.Context, bookID string, limit, offset int) ([]*model.Article, error) {
	return s.Store.ListArticlesByBook(ctx, bookID, limit, offset)
}

func jitterSleep(ctx context.Context, min, max time.Duration) {
	if max <= min {
		time.Sleep(min)
		return
	}
	d := min + time.Duration(rand.Int63n(int64(max-min)))
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func sortReviewsDesc(rs []reviewItem) {
	for i := 0; i < len(rs); i++ {
		for j := i + 1; j < len(rs); j++ {
			if rs[j].PublishAt > rs[i].PublishAt {
				rs[i], rs[j] = rs[j], rs[i]
			}
		}
	}
}

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func asInt64(v interface{}) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	default:
		return 0
	}
}

var dumpMpInfoDone = false

func dumpMpInfoOnce(reviewID string, createTime int64, mpInfo map[string]interface{}) {
	if dumpMpInfoDone || mpInfo == nil {
		return
	}
	dumpMpInfoDone = true
	mp := make(map[string]interface{}, len(mpInfo))
	for k, v := range mpInfo {
		if k == "coverBoxInfo" {
			continue
		}
		mp[k] = v
	}
	b, _ := json.MarshalIndent(map[string]interface{}{
		"reviewId":   reviewID,
		"createTime": createTime,
		"mpInfo":     mp,
	}, "", "  ")
	log.Printf("[dump once] /book/articles first review (coverBoxInfo stripped):\n%s", string(b))
}

