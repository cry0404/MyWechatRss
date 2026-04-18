package httpapi

import (
	"sync"
	"time"
)

type qrSession struct {
	DeviceID   string
	InstallID  string
	DeviceName string
	ExpireAt   time.Time
}

type QRSessionStore struct {
	mu       sync.Mutex
	sessions map[string]qrSession
}

func NewQRSessionStore() *QRSessionStore {
	return &QRSessionStore{sessions: make(map[string]qrSession)}
}

const qrSessionTTL = 5 * time.Minute

func (s *QRSessionStore) Save(qrID, deviceID, installID, deviceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[qrID] = qrSession{
		DeviceID:   deviceID,
		InstallID:  installID,
		DeviceName: deviceName,
		ExpireAt:   time.Now().Add(qrSessionTTL),
	}
	s.gcLocked()
}

func (s *QRSessionStore) Get(qrID string) (qrSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[qrID]
	if !ok {
		return qrSession{}, false
	}
	if time.Now().After(sess.ExpireAt) {
		delete(s.sessions, qrID)
		return qrSession{}, false
	}
	return sess, true
}

func (s *QRSessionStore) Delete(qrID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, qrID)
}

func (s *QRSessionStore) gcLocked() {
	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.ExpireAt) {
			delete(s.sessions, id)
		}
	}
}
