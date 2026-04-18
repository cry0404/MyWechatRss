package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SessionTokenTTL = 30 * 24 * time.Hour

type SessionClaims struct {
	UID int64 `json:"uid"`
	Exp int64 `json:"exp"` // Unix 秒
}

type Signer struct {
	secret []byte
}

func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

func (s *Signer) Issue(uid int64, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = SessionTokenTTL
	}
	c := SessionClaims{UID: uid, Exp: time.Now().Add(ttl).Unix()}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	sig := s.sign(body)
	return body + "." + sig, nil
}

func (s *Signer) Verify(token string) (*SessionClaims, error) {
	body, sig, ok := strings.Cut(token, ".")
	if !ok {
		return nil, errors.New("bad token format")
	}
	expected := s.sign(body)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) != 1 {
		return nil, errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}
	var c SessionClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if c.Exp < time.Now().Unix() {
		return nil, errors.New("token expired")
	}
	return &c, nil
}

func (s *Signer) sign(body string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
