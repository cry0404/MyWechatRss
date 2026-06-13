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
	if mode == "" {
		mode = "summary"
	}
	return &Service{Store: st, Caller: cr, Mode: mode}
}

var (
	firstFetchSleepMin = 5 * time.Second
	firstFetchSleepMax = 12 * time.Second
	incrFetchSleepMin  = 15 * time.Second
	incrFetchSleepMax  = 25 * time.Second
	fetchAllSleepMin   = 30 * time.Second
	fetchAllSleepMax   = 120 * time.Second
	bookOpenSleepMin   = 200 * time.Millisecond
	bookOpenSleepMax   = 800 * time.Millisecond
	firstFetchMax      = 30
	incrFetchPage      = 20
)

func (s *Service) FetchLatest(ctx context.Context, userID, subID int64) (int, error) {
	sub, err := s.Store.GetSubscription(ctx, userID, subID)
	if err != nil {
		return 0, err
	}
	sourceStart := time.Now()
	recordSource := func(accountID int64, newCount int, fetchErr error) {
		rec := &model.SubscriptionFetchLog{
			SubscriptionID: sub.ID,
			AccountID:      accountID,
			StartedAt:      sourceStart.Unix(),
			CostMs:         time.Since(sourceStart).Milliseconds(),
			NewCount:       int64(newCount),
		}
		if fetchErr != nil {
			rec.Error = fetchErr.Error()
		}
		if logErr := s.Store.RecordSubscriptionFetchLog(ctx, rec); logErr != nil {
			log.Printf("record source fetch log sub=%d account=%d: %v", sub.ID, accountID, logErr)
		}
	}
	firstRun := sub.LastReviewTime == 0

	acc, err := s.Store.PickActiveAccount(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			wrapped := fmt.Errorf("pick account: %w", accounts.ErrNoAccount)
			recordSource(0, 0, wrapped)
			return 0, wrapped
		}
		wrapped := fmt.Errorf("pick account: %w", err)
		recordSource(0, 0, wrapped)
		return 0, wrapped
	}
	preferID := acc.ID
	recoveryProbe := isRateLimitRecoveryAccount(acc)

	pageSize := incrFetchPage
	if recoveryProbe {
		pageSize = 5
		log.Printf("fetch sub %d (%s): rate-limit recovery probe count=%d", sub.ID, sub.BookID, pageSize)
	}

	reviews, err := s.fetchReviewList(ctx, userID, preferID, sub.BookID, pageSize, 0, !recoveryProbe)
	if err != nil {
		recordSource(preferID, 0, err)
		return 0, err
	}

	if firstRun && !recoveryProbe && len(reviews) > 0 && len(reviews) < firstFetchMax {
		more, err := s.fetchReviewList(ctx, userID, preferID, sub.BookID, incrFetchPage, len(reviews), false)
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
		if recoveryProbe && len(todo) >= pageSize {
			break
		}
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

		if recoveryProbe {
			continue
		}
		if err := s.fetchAndStoreContent(ctx, userID, preferID, sub.BookID, r.ReviewID, r.URL); err != nil {
			log.Printf("fetch sub %d: content %s: %v", sub.ID, r.ReviewID, err)
		}
	}

	if err := s.Store.UpdateSubscriptionFetchState(ctx, sub.ID, time.Now().Unix(), maxTime); err != nil {
		recordSource(preferID, newCount, err)
		return newCount, err
	}
	recordSource(preferID, newCount, nil)
	return newCount, nil
}

func isRateLimitRecoveryAccount(acc *model.WeReadAccount) bool {
	return acc != nil && strings.HasPrefix(acc.LastErr, "errcode=-2041")
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

func (s *Service) fetchReviewList(ctx context.Context, userID, preferAccountID int64, bookID string, count, offset int, warmup bool) ([]reviewItem, error) {
	if warmup && offset == 0 {
		if err := s.warmupBookOpen(ctx, userID, preferAccountID, bookID); err != nil {
			return nil, err
		}
	}
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

func (s *Service) warmupBookOpen(ctx context.Context, userID, preferAccountID int64, bookID string) error {
	steps := []accounts.CallOptions{
		{
			Method: http.MethodGet,
			Path:   "/book/info",
			Query:  map[string]string{"bookId": bookID},
		},
		{
			Method: http.MethodGet,
			Path:   "/book/getProgress",
			Query:  map[string]string{"bookId": bookID},
		},
		{
			Method: http.MethodGet,
			Path:   "/book/readinfo",
			Query: map[string]string{
				"bookId":            bookID,
				"finishedBookCount": "1",
				"finishedBookIndex": "1",
				"finishedDate":      "1",
				"readingBookCount":  "1",
				"readingBookIndex":  "1",
				"vid":               strconv.FormatInt(s.pickVIDForLog(ctx, userID, preferAccountID), 10),
			},
		},
		{
			Method: http.MethodGet,
			Path:   "/book/detailinfo",
			Query: map[string]string{
				"bookId":    bookID,
				"count":     "3,0,1",
				"listtypes": "1,6,10",
				"maxidx":    "0,0,0",
				"synckey":   "0,0,0",
			},
		},
	}

	for i := range steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i > 0 {
			jitterSleep(ctx, bookOpenSleepMin, bookOpenSleepMax)
		}
		steps[i].PreferAccountID = preferAccountID
		if _, err := s.Caller.Do(ctx, userID, steps[i]); err != nil {
			log.Printf("book open warmup %s bookId=%s account=%d: %v", steps[i].Path, bookID, preferAccountID, err)
			if errors.Is(err, accounts.ErrSearchRateLimited) ||
				errors.Is(err, accounts.ErrHighRiskDeferred) ||
				errors.Is(err, accounts.ErrNoAccount) {
				return err
			}
		}
	}
	return nil
}

func (s *Service) pickVIDForLog(ctx context.Context, userID, preferAccountID int64) int64 {
	if preferAccountID > 0 {
		if acc, err := s.Store.GetActiveAccountByID(ctx, userID, preferAccountID); err == nil {
			return acc.VID
		}
	}
	if acc, err := s.Store.PickActiveAccount(ctx, userID); err == nil {
		return acc.VID
	}
	return 0
}

func (s *Service) fetchAndStoreContent(ctx context.Context, userID, preferAccountID int64, bookID, reviewID, mpURL string) error {
	if !strings.EqualFold(s.Mode, "full") {
		return nil
	}

	var lastErr error
	var html string

	html, lastErr = s.tryChain(ctx, bookID, reviewID, "web", func() (string, error) {
		return s.fetchContentViaWebContent(ctx, userID, reviewID)
	})
	if lastErr == nil {
		return s.Store.UpdateArticleContent(ctx, reviewID, html)
	}
	log.Printf("fetch content %s: web failed: %v", reviewID, lastErr)

	if mpURL != "" {
		html, lastErr = s.tryChain(ctx, bookID, reviewID, "mp", func() (string, error) {
			return fetchMpContent(ctx, mpURL)
		})
		if lastErr == nil {
			return s.Store.UpdateArticleContent(ctx, reviewID, html)
		}
		log.Printf("fetch content %s: mp failed: %v", reviewID, lastErr)
	}

	lastErr = s.fetchContentViaShareChapterWithLog(ctx, userID, preferAccountID, bookID, reviewID)
	if lastErr != nil {
		log.Printf("fetch content %s: all chains failed", reviewID)
	}
	return lastErr
}

func (s *Service) tryChain(ctx context.Context, bookID, reviewID, chain string, fn func() (string, error)) (string, error) {
	start := time.Now()
	html, err := fn()
	cost := time.Since(start).Milliseconds()
	logRec := &model.ArticleFetchLog{
		ReviewID: reviewID,
		BookID:   bookID,
		Chain:    chain,
		Success:  err == nil,
		CostMs:   cost,
	}
	if err != nil {
		logRec.Error = err.Error()
	}
	if logErr := s.Store.RecordArticleFetchLog(ctx, logRec); logErr != nil {
		log.Printf("record fetch log %s/%s: %v", reviewID, chain, logErr)
	}
	return html, err
}

func (s *Service) fetchContentViaShareChapterWithLog(ctx context.Context, userID, preferAccountID int64, bookID, reviewID string) error {
	start := time.Now()
	err := s.fetchContentViaShareChapter(ctx, userID, preferAccountID, reviewID)
	cost := time.Since(start).Milliseconds()
	logRec := &model.ArticleFetchLog{
		ReviewID: reviewID,
		BookID:   bookID,
		Chain:    "shareChapter",
		Success:  err == nil,
		CostMs:   cost,
	}
	if err != nil {
		logRec.Error = err.Error()
	}
	if logErr := s.Store.RecordArticleFetchLog(ctx, logRec); logErr != nil {
		log.Printf("record fetch log %s/shareChapter: %v", reviewID, logErr)
	}
	return err
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

	// 调试：如果找不到正文，把页面 title 和前几行 dump 出来
	content := doc.Find("#js_content").First()
	if content.Length() == 0 {
		title := strings.TrimSpace(doc.Find("title").First().Text())
		preview := string(body)
		if len(preview) > 600 {
			preview = preview[:600]
		}
		log.Printf("web content debug reviewId=%s title=%q preview=%q", reviewID, title, preview)
		return "", fmt.Errorf("web content 正文节点 #js_content 不存在 (title=%s)", title)
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
	// shareChapter 是最高风险的账号接口，只保留当前确认过的最小请求形态。
	// 协议漂移时不要在同一篇文章上连续试探多组参数，避免放大风控权重。
	s.warmupReviewOpen(ctx, userID, preferAccountID, reviewID)

	attempts := []struct {
		name  string
		query map[string]string
	}{
		{"original", map[string]string{"cmd": "get", "reviewId": reviewID}},
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
		log.Printf("shareChapter debug reviewId=%s variant=%s raw=%s", reviewID, att.name, string(res.RawJSON))
		lastErr = fmt.Errorf("shareChapter %s: 响应无正文", att.name)
	}

	if lastErr != nil {
		return lastErr
	}
	return errors.New("shareChapter 所有参数组合均失败")
}

func (s *Service) warmupReviewOpen(ctx context.Context, userID, preferAccountID int64, reviewID string) {
	_, err := s.Caller.Do(ctx, userID, accounts.CallOptions{
		Method: http.MethodGet,
		Path:   "/review/single",
		Query: map[string]string{
			"bookReviewCount":   "1",
			"commentsCount":     "100",
			"commentsDirection": "1",
			"likesCount":        "20",
			"likesDirection":    "0",
			"reviewId":          reviewID,
			"synckey":           "0",
		},
		PreferAccountID: preferAccountID,
	})
	if err != nil {
		log.Printf("review open warmup reviewId=%s account=%d: %v", reviewID, preferAccountID, err)
	}
}

func (s *Service) EnsureContent(ctx context.Context, userID int64, reviewID string) (*model.Article, error) {
	a, err := s.Store.GetArticleByReviewID(ctx, reviewID)
	if err != nil {
		return nil, err
	}
	if a.ContentHTML != "" {
		return a, nil
	}
	if err := s.fetchAndStoreContent(ctx, userID, 0, a.BookID, reviewID, a.URL); err != nil {
		return a, err
	}
	return s.Store.GetArticleByReviewID(ctx, reviewID)
}

func (s *Service) ListByBook(ctx context.Context, bookID string, limit, offset int) ([]*model.Article, error) {
	return s.Store.ListArticlesByBook(ctx, bookID, limit, offset)
}

func (s *Service) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]*model.Article, error) {
	return s.Store.ListArticlesByUser(ctx, userID, limit, offset)
}

// FetchAll 拉取指定用户的所有订阅（无视调度间隔）。
// 返回 {bookID: newCount} 的汇总结果，以及过程中遇到的最后一个错误。
func (s *Service) FetchAll(ctx context.Context, userID int64) (map[string]int, error) {
	subs, err := s.Store.ListSubscriptionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return map[string]int{}, nil
	}

	result := make(map[string]int, len(subs))
	var lastErr error

	for _, sub := range subs {
		if ctx.Err() != nil {
			break
		}
		if sub.Disabled {
			continue
		}
		n, err := s.FetchLatest(ctx, userID, sub.ID)
		if err != nil {
			lastErr = err
			log.Printf("fetch-all: sub %d (%s): %v", sub.ID, sub.BookID, err)
			if errors.Is(err, accounts.ErrNoAccount) ||
				errors.Is(err, accounts.ErrHighRiskDeferred) ||
				errors.Is(err, accounts.ErrSearchRateLimited) {
				break
			}
		}
		result[sub.BookID] = n
		jitterSleep(ctx, fetchAllSleepMin, fetchAllSleepMax)
	}

	return result, lastErr
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
