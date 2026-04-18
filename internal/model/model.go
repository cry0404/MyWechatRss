package model

import "time"

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"created_at"`
	IsAdmin      bool   `json:"is_admin"`
}

type WeReadAccount struct {
	ID            int64             `json:"id"`
	UserID        int64             `json:"user_id"`
	VID           int64             `json:"vid"`
	SKey          string            `json:"-"`
	RefreshToken  string            `json:"-"`
	Cookies       map[string]string `json:"-"`
	Nickname      string            `json:"nickname"`
	Avatar        string            `json:"avatar"`
	Status        AccountStatus     `json:"status"`
	CooldownUntil int64             `json:"cooldown_until"`
	LastOkAt      int64             `json:"last_ok_at"`
	LastErr       string            `json:"last_err"`
	CreatedAt     int64             `json:"created_at"`

	// 设备身份。与账号 1:1 绑定，每次扫码登录时自造，后续调用都复用同一组。
	// 这样"一个 weread 账号 = 一台虚拟设备"，行为上最接近真实 App。
	// device_id / install_id 是指纹性质，不给前端；device_name 是展示用的可以露出。
	DeviceID   string `json:"-"`
	InstallID  string `json:"-"`
	DeviceName string `json:"device_name"`
}

type AccountStatus string

const (
	AccountActive   AccountStatus = "active"
	AccountCooldown AccountStatus = "cooldown"
	AccountDead     AccountStatus = "dead"
)

type Subscription struct {
	ID               int64  `json:"id"`
	UserID           int64  `json:"user_id"`
	BookID           string `json:"book_id"` // MP_WXS_ 开头
	Alias            string `json:"alias"`
	MPName           string `json:"mp_name"`
	CoverURL         string `json:"cover_url"`
	FetchIntervalSec int64 `json:"fetch_interval_sec"`
	// FetchWindowStartMin / EndMin 为当日 0..1439 的分钟，表示仅在该时段内由调度器拉取；-1 表示不限制（全天）。
	// 与服务器本地时区一致（部署时可设 TZ）。
	FetchWindowStartMin int64 `json:"fetch_window_start_min"`
	FetchWindowEndMin   int64 `json:"fetch_window_end_min"`
	LastFetchAt         int64 `json:"last_fetch_at"`
	LastReviewTime   int64  `json:"last_review_time"`
	CreatedAt        int64  `json:"created_at"`
	Disabled         bool   `json:"disabled"`
}

// Article 对应 articles 表。ContentHTML 允许为空（正文延迟抓取）。
type Article struct {
	ID          int64  `json:"id"`
	BookID      string `json:"book_id"`
	ReviewID    string `json:"review_id"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	ContentHTML string `json:"content_html,omitempty"`
	CoverURL    string `json:"cover_url,omitempty"`
	URL         string `json:"url,omitempty"`
	PublishAt   int64  `json:"publish_at"`
	FetchedAt   int64  `json:"fetched_at"`
	ReadNum     int64  `json:"read_num"`
	LikeNum     int64  `json:"like_num"`
}

// ArticleFetchLog 对应 article_fetch_logs 表，记录单篇正文抓取的链路耗时与结果。
type ArticleFetchLog struct {
	ID        int64  `json:"id"`
	ReviewID  string `json:"review_id"`
	BookID    string `json:"book_id"`
	Chain     string `json:"chain"`
	Success   bool   `json:"success"`
	CostMs    int64  `json:"cost_ms"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// FetchStats 按 chain 汇总的抓取统计。
type FetchStats struct {
	Chain      string  `json:"chain"`
	Total      int64   `json:"total"`
	Success    int64   `json:"success"`
	Fail       int64   `json:"fail"`
	SuccessPct float64 `json:"success_pct"`
	AvgCostMs  int64   `json:"avg_cost_ms"`
}

// Now 统一的"当前时间"，单测里可替换；目前直接用 time.Now。
var Now = func() time.Time { return time.Now() }
