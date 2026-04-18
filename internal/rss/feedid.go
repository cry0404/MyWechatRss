package rss

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

type FeedIDEncoder struct {
	secret []byte
}

func NewFeedIDEncoder(salt string) *FeedIDEncoder {
	return &FeedIDEncoder{secret: []byte(salt)}
}

func (e *FeedIDEncoder) Encode(userID, subID int64) string {
	payload := strconv.FormatInt(userID, 16) + "-" + strconv.FormatInt(subID, 16)
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))[:16]
	return payload + "-" + sig
}

func (e *FeedIDEncoder) Decode(feedID string) (userID, subID int64, err error) {
	parts := strings.Split(feedID, "-")
	if len(parts) != 3 {
		return 0, 0, errors.New("bad feed id")
	}
	payload := parts[0] + "-" + parts[1]
	mac := hmac.New(sha256.New, e.secret)
	mac.Write([]byte(payload))
	if hex.EncodeToString(mac.Sum(nil))[:16] != parts[2] {
		return 0, 0, errors.New("bad feed id signature")
	}
	uid, err := strconv.ParseInt(parts[0], 16, 64)
	if err != nil {
		return 0, 0, err
	}
	sid, err := strconv.ParseInt(parts[1], 16, 64)
	if err != nil {
		return 0, 0, err
	}
	return uid, sid, nil
}
