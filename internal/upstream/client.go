package upstream

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Credential struct {
	VID          int64             `json:"vid"`
	SKey         string            `json:"skey"`
	RefreshToken string            `json:"refreshToken,omitempty"`
	OpenID       string            `json:"openId,omitempty"`
	Nickname     string            `json:"nickname,omitempty"`
	Avatar       string            `json:"avatar,omitempty"`
	Cookies      map[string]string `json:"cookies,omitempty"`
}

type CredentialLite struct {
	VID      int64             `json:"vid"`
	SKey     string            `json:"skey"`
	Cookies  map[string]string `json:"cookies,omitempty"`
	DeviceID string            `json:"deviceId,omitempty"`
}

type Client struct {
	BaseURL   string
	APIKeyID  string
	APISecret string
	HTTP      *http.Client
}

func New(baseURL, apiKeyID, apiSecret string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKeyID:  apiKeyID,
		APISecret: apiSecret,
		HTTP:      &http.Client{Timeout: 40 * time.Second},
	}
}


type QRCodeResp struct {
	QRID          string `json:"qr_id"`
	QRImageBase64 string `json:"qr_image_base64"`
	ExpireAt      int64  `json:"expire_at"`
}

func (c *Client) LoginQRCode(ctx context.Context) (*QRCodeResp, error) {
	var out QRCodeResp
	if err := c.postJSON(ctx, "/proxy/weread/login/qrcode", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type LoginCheckReq struct {
	QRID       string `json:"qr_id"`
	DeviceID   string `json:"device_id,omitempty"`
	InstallID  string `json:"install_id,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
}

type LoginCheckResp struct {
	Status     string      `json:"status"`
	Credential *Credential `json:"credential,omitempty"`
}

func (c *Client) LoginCheck(ctx context.Context, req LoginCheckReq) (*LoginCheckResp, error) {
	var out LoginCheckResp
	if err := c.postJSON(ctx, "/proxy/weread/login/check", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type LoginRefreshReq struct {
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
	DeviceName   string `json:"device_name,omitempty"`
	RefCgi       string `json:"ref_cgi,omitempty"`
}

type LoginRefreshResp = LoginCheckResp

func (c *Client) LoginRefresh(ctx context.Context, req LoginRefreshReq) (*LoginRefreshResp, error) {
	var out LoginRefreshResp
	if err := c.postJSON(ctx, "/proxy/weread/login/refresh", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}


type CallReq struct {
	Credential CredentialLite    `json:"credential"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Query      map[string]string `json:"query,omitempty"`
	Body       json.RawMessage   `json:"body,omitempty"`
	BodyType   string            `json:"body_type,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
}

type CallResp struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`

	Cookies map[string]string `json:"cookies,omitempty"`
}

func (c *Client) Call(ctx context.Context, req CallReq) (*CallResp, error) {
	var out CallResp
	if err := c.postJSON(ctx, "/proxy/weread/call", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}



// postJSON 对 server 的 /proxy/weread/* 发一个带 HMAC 签名的 JSON POST。
func (c *Client) postJSON(ctx context.Context, path string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.signRequest(req, body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read upstream body: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("upstream unmarshal: %w (body=%s)", err, truncate(string(raw), 256))
	}
	return nil
}

// signRequest 按 server/internal/httpapi/middleware.go 约定的规则签名。
func (c *Client) signRequest(req *http.Request, body []byte) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	bodyHash := sha256.Sum256(body)
	canonical := ts + req.Method + req.URL.Path + hex.EncodeToString(bodyHash[:])

	mac := hmac.New(sha256.New, []byte(c.APISecret))
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	req.Header.Set("X-Api-Key", c.APIKeyID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", sig)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
