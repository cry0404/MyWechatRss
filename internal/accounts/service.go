package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cry0404/MyWechatRss/internal/model"
	"github.com/cry0404/MyWechatRss/internal/store"
	"github.com/cry0404/MyWechatRss/internal/upstream"
)

type Service struct {
	Store        *store.Store
	Upstream     *upstream.Client
	DefaultDeviceName string
}

func NewService(st *store.Store, up *upstream.Client, defaultDeviceName string) *Service {
	return &Service{Store: st, Upstream: up, DefaultDeviceName: defaultDeviceName}
}

type StartResult struct {
	QRID          string `json:"qr_id"`
	QRImageBase64 string `json:"qr_image_base64"`
	ExpireAt      int64  `json:"expire_at"`

	DeviceID   string `json:"device_id"`
	InstallID  string `json:"install_id"`
	DeviceName string `json:"device_name"`
}

func (s *Service) StartLogin(ctx context.Context, deviceNameHint string) (*StartResult, error) {
	qr, err := s.Upstream.LoginQRCode(ctx)
	if err != nil {
		return nil, err
	}
	deviceName := deviceNameHint
	if deviceName == "" {
		deviceName = s.DefaultDeviceName
	}
	return &StartResult{
		QRID:          qr.QRID,
		QRImageBase64: qr.QRImageBase64,
		ExpireAt:      qr.ExpireAt,
		DeviceID:      randomHex(16),
		InstallID:     randomHex(16),
		DeviceName:    deviceName,
	}, nil
}

type PollResult struct {
	Status    string             `json:"status"`
	Confirmed bool               `json:"confirmed"`
	Account   *model.WeReadAccount `json:"account,omitempty"`
}

func (s *Service) Poll(ctx context.Context, userID int64, qrID, deviceID, installID, deviceName string) (*PollResult, error) {
	if qrID == "" || deviceID == "" || installID == "" {
		return nil, errors.New("qr_id / device_id / install_id 必填")
	}

	resp, err := s.Upstream.LoginCheck(ctx, upstream.LoginCheckReq{
		QRID:       qrID,
		DeviceID:   deviceID,
		InstallID:  installID,
		DeviceName: deviceName,
	})
	if err != nil {
		return nil, err
	}

	switch resp.Status {
	case "confirmed":
		if resp.Credential == nil {
			return nil, fmt.Errorf("upstream 报 confirmed 但缺 credential")
		}
		acc := &model.WeReadAccount{
			UserID:       userID,
			VID:          resp.Credential.VID,
			SKey:         resp.Credential.SKey,
			RefreshToken: resp.Credential.RefreshToken,
			Cookies:      resp.Credential.Cookies,
			Nickname:     resp.Credential.Nickname,
			Avatar:       resp.Credential.Avatar,
			Status:       model.AccountActive,
			DeviceID:     deviceID,
			InstallID:    installID,
			DeviceName:   deviceName,
		}
		if err := s.Store.CreateAccount(ctx, acc); err != nil {
			return nil, fmt.Errorf("save account: %w", err)
		}
		safe := *acc
		safe.SKey = ""
		safe.RefreshToken = ""
		safe.Cookies = nil
		return &PollResult{Status: "confirmed", Confirmed: true, Account: &safe}, nil

	case "pending", "scanned", "expired", "cancelled":
		return &PollResult{Status: resp.Status}, nil
	default:
		return &PollResult{Status: resp.Status}, nil
	}
}

func (s *Service) List(ctx context.Context, userID int64) ([]*model.WeReadAccount, error) {
	accs, err := s.Store.ListAccountsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, a := range accs {
		a.SKey = ""
		a.RefreshToken = ""
		a.Cookies = nil
	}
	return accs, nil
}

func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.Store.DeleteAccount(ctx, userID, id)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
