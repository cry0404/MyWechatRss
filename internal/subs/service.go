package subs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
)

const mpBookIDPrefix = "MP_WXS_"

type Service struct {
	Store  *store.Store
	Caller *accounts.Caller
}

func NewService(st *store.Store, cr *accounts.Caller) *Service {
	return &Service{Store: st, Caller: cr}
}

type SearchResultItem struct {
	BookID string `json:"book_id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Cover  string `json:"cover"`
}

func (s *Service) Search(ctx context.Context, userID int64, keyword string) ([]SearchResultItem, error) {
	if strings.TrimSpace(keyword) == "" {
		return nil, errors.New("keyword required")
	}
	res, err := s.Caller.Do(ctx, userID, accounts.CallOptions{
		Method: http.MethodGet,
		Path:   "/store/search",
		Query: map[string]string{
			"keyword": keyword,
			"scope":   "2",
			"type":    "0",
			"v":       "2",
			"count":   "20",
		},
	})
	if err != nil {
		return nil, err
	}

	log.Printf("search raw body (keyword=%q): %s", keyword, truncateForLog(res.RawJSON, 2000))

	items, err := parseSearchResults(res.RawJSON)
	if err != nil {
		return nil, fmt.Errorf("parse search: %w", err)
	}
	return items, nil
}

type searchBookEntry struct {
	BookID string `json:"bookId"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Cover  string `json:"cover"`
}

type searchBookNode struct {
	BookInfo searchBookEntry `json:"bookInfo"`
	searchBookEntry
}

func (n searchBookNode) pick() searchBookEntry {
	if n.BookInfo.BookID != "" {
		return n.BookInfo
	}
	return n.searchBookEntry
}

func parseSearchResults(raw []byte) ([]SearchResultItem, error) {
	var envelope struct {
		ErrCode int              `json:"errcode"`
		ErrMsg  string           `json:"errmsg"`
		Books   []searchBookNode `json:"books"`
		Results []struct {
			Type  int              `json:"type"`
			Books []searchBookNode `json:"books"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.ErrCode != 0 {
		return nil, fmt.Errorf("weread search errcode=%d errmsg=%s", envelope.ErrCode, envelope.ErrMsg)
	}

	seen := map[string]struct{}{}
	out := make([]SearchResultItem, 0)

	add := func(b searchBookEntry) {
		if !strings.HasPrefix(b.BookID, mpBookIDPrefix) {
			return
		}
		if _, ok := seen[b.BookID]; ok {
			return
		}
		seen[b.BookID] = struct{}{}
		out = append(out, SearchResultItem{
			BookID: b.BookID,
			Title:  b.Title,
			Author: b.Author,
			Cover:  b.Cover,
		})
	}

	for _, n := range envelope.Books {
		add(n.pick())
	}
	for _, grp := range envelope.Results {
		for _, n := range grp.Books {
			add(n.pick())
		}
	}
	return out, nil
}

func (s *Service) Create(ctx context.Context, userID int64, bookID, alias string) (*model.Subscription, error) {
	if !strings.HasPrefix(bookID, mpBookIDPrefix) {
		return nil, fmt.Errorf("bookId 必须以 %s 开头", mpBookIDPrefix)
	}
	sub := &model.Subscription{
		UserID: userID,
		BookID: bookID,
		Alias:  strings.TrimSpace(alias),
	}

	if info, err := s.fetchBookInfo(ctx, userID, bookID); err == nil {
		sub.MPName = info.Title
		sub.CoverURL = info.Cover
		if sub.Alias == "" {
			sub.Alias = info.Title
		}
	}

	if err := s.Store.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

type bookInfo struct {
	Title string `json:"title"`
	Cover string `json:"cover"`
}

func (s *Service) fetchBookInfo(ctx context.Context, userID int64, bookID string) (*bookInfo, error) {
	res, err := s.Caller.Do(ctx, userID, accounts.CallOptions{
		Method: http.MethodGet,
		Path:   "/book/info",
		Query:  map[string]string{"bookId": bookID},
	})
	if err != nil {
		return nil, err
	}
	var info bookInfo
	if err := json.Unmarshal(res.RawJSON, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *Service) List(ctx context.Context, userID int64) ([]*model.Subscription, error) {
	return s.Store.ListSubscriptionsByUser(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id int64) (*model.Subscription, error) {
	return s.Store.GetSubscription(ctx, userID, id)
}

type UpdatePayload struct {
	Alias               *string `json:"alias,omitempty"`
	FetchIntervalSec    *int64  `json:"fetch_interval_sec,omitempty"`
	Disabled            *bool   `json:"disabled,omitempty"`
	FetchWindowStartMin *int64  `json:"fetch_window_start_min,omitempty"`
	FetchWindowEndMin   *int64  `json:"fetch_window_end_min,omitempty"`
}

const (
	minFetchIntervalSec = 60            // 1 分钟
	maxFetchIntervalSec = 7 * 24 * 3600 // 7 天
)

func (s *Service) Update(ctx context.Context, userID, id int64, p UpdatePayload) (*model.Subscription, error) {
	if p.FetchIntervalSec != nil {
		v := *p.FetchIntervalSec
		if v < minFetchIntervalSec || v > maxFetchIntervalSec {
			return nil, fmt.Errorf("fetch_interval_sec must be between %d and %d", minFetchIntervalSec, maxFetchIntervalSec)
		}
	}
	if p.FetchWindowStartMin != nil || p.FetchWindowEndMin != nil {
		a, b := p.FetchWindowStartMin, p.FetchWindowEndMin
		if a == nil || b == nil {
			return nil, errors.New("fetch_window_start_min and fetch_window_end_min must be set together")
		}
		sa, sb := *a, *b
		if sa == -1 && sb == -1 {
		} else {
			if sa < 0 || sa > 1439 || sb < 0 || sb > 1439 {
				return nil, errors.New("fetch window minutes must be -1 (disabled) or 0..1439")
			}
		}
	}
	if err := s.Store.UpdateSubscriptionMeta(ctx, userID, id, p.Alias, p.FetchIntervalSec, p.Disabled, p.FetchWindowStartMin, p.FetchWindowEndMin); err != nil {
		return nil, err
	}
	return s.Store.GetSubscription(ctx, userID, id)
}

func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.Store.DeleteSubscription(ctx, userID, id)
}

func truncateForLog(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}
