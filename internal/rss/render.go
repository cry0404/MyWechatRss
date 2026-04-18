package rss

import (
	"encoding/xml"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/cry0404/MyWechatRss/internal/model"
)

type Feed struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Content string   `xml:"xmlns:content,attr"`
	Atom    string   `xml:"xmlns:atom,attr"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	AtomLink   AtomLink `xml:"atom:link"`
	LastBuild  string   `xml:"lastBuildDate"`
	Generator  string   `xml:"generator"`
	Image      *Image   `xml:"image,omitempty"`
	Items      []Item   `xml:"item"`
}

type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type Image struct {
	URL   string `xml:"url"`
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        GUID   `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description,omitempty"`
	ContentEncoded *CDATA `xml:"content:encoded,omitempty"`
}

type GUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr"`
}

type CDATA struct {
	Value string `xml:",cdata"`
}

type RenderOptions struct {
	PublicBaseURL string // 如 https://read.example.com
	SelfURL       string // 本 feed 自身的完整 URL，用于 atom:link
}

func RenderSubscription(sub *model.Subscription, articles []*model.Article, opt RenderOptions) ([]byte, error) {
	title := displayTitle(sub)
	ch := Channel{
		Title:       title,
		Link:        opt.PublicBaseURL,
		Description: fmt.Sprintf("公众号「%s」的文章订阅 (via WeChatRead RSS)", title),
		Language:    "zh-cn",
		AtomLink: AtomLink{
			Href: opt.SelfURL,
			Rel:  "self",
			Type: "application/rss+xml",
		},
		LastBuild: time.Now().UTC().Format(time.RFC1123Z),
		Generator: "wechatread-rss",
	}
	if sub.CoverURL != "" {
		ch.Image = &Image{URL: sub.CoverURL, Title: title, Link: opt.PublicBaseURL}
	}
	subByBook := map[string]*model.Subscription{sub.BookID: sub}
	ch.Items = buildItems(articles, subByBook, opt.PublicBaseURL)

	return marshalFeed(ch)
}

func RenderAggregate(
	userDisplayName string,
	articles []*model.Article,
	subByBook map[string]*model.Subscription,
	opt RenderOptions,
) ([]byte, error) {
	title := "全部订阅聚合"
	if userDisplayName != "" {
		title = userDisplayName + " · 全部订阅聚合"
	}
	ch := Channel{
		Title:       title,
		Link:        opt.PublicBaseURL,
		Description: "所有订阅公众号的文章聚合 (via WeChatRead RSS)",
		Language:    "zh-cn",
		AtomLink: AtomLink{
			Href: opt.SelfURL,
			Rel:  "self",
			Type: "application/rss+xml",
		},
		LastBuild: time.Now().UTC().Format(time.RFC1123Z),
		Generator: "wechatread-rss",
	}
	ch.Items = buildItems(articles, subByBook, opt.PublicBaseURL)
	return marshalFeed(ch)
}

func buildItems(
	articles []*model.Article,
	subByBook map[string]*model.Subscription,
	publicBase string,
) []Item {
	items := make([]Item, 0, len(articles))
	for _, a := range articles {
		item := Item{
			Title:   stripControlChars(a.Title),
			Link:    articleLink(publicBase, a),
			GUID:    GUID{Value: a.ReviewID, IsPermaLink: "false"},
			PubDate: time.Unix(a.PublishAt, 0).UTC().Format(time.RFC1123Z),
		}
		if sub, ok := subByBook[a.BookID]; ok && len(subByBook) > 1 {
			name := displayTitle(sub)
			item.Title = "[" + name + "] " + item.Title
		}
		if a.Summary != "" {
			item.Description = html.EscapeString(a.Summary)
		}
		if a.ContentHTML != "" {
			item.ContentEncoded = &CDATA{Value: a.ContentHTML}
		}
		items = append(items, item)
	}
	return items
}

func marshalFeed(ch Channel) ([]byte, error) {
	feed := Feed{
		Version: "2.0",
		Content: "http://purl.org/rss/1.0/modules/content/",
		Atom:    "http://www.w3.org/2005/Atom",
		Channel: ch,
	}
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func displayTitle(sub *model.Subscription) string {
	if sub.Alias != "" {
		return sub.Alias
	}
	if sub.MPName != "" {
		return sub.MPName
	}
	return sub.BookID
}

func articleLink(base string, a *model.Article) string {
	if a.URL != "" {
		return a.URL
	}
	return fmt.Sprintf("%s/article/%s", strings.TrimRight(base, "/"), a.ReviewID)
}

func stripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return r
		case r < 0x20:
			return -1
		}
		return r
	}, s)
}
