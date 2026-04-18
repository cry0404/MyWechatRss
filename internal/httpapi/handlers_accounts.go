package httpapi

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/accounts"
	"github.com/cry0404/MyWechatRss/internal/auth"
	"github.com/cry0404/MyWechatRss/internal/model"
)

type AccountHandlers struct {
	Svc *accounts.Service
	QR  *QRSessionStore
}

func (h *AccountHandlers) Start(c *gin.Context) {
	var body struct {
		DeviceName string `json:"device_name,omitempty"`
	}
	_ = c.ShouldBindJSON(&body)

	res, err := h.Svc.StartLogin(c.Request.Context(), body.DeviceName)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	h.QR.Save(res.QRID, res.DeviceID, res.InstallID, res.DeviceName)

	c.JSON(http.StatusOK, gin.H{
		"qr_id":     res.QRID,
		"qr_image":  "data:image/png;base64," + res.QRImageBase64,
		"expire_at": res.ExpireAt,
	})
}

func (h *AccountHandlers) Poll(c *gin.Context) {
	qrID := c.Query("qr_id")
	if qrID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "qr_id required"})
		return
	}
	sess, ok := h.QR.Get(qrID)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": "expired"})
		return
	}

	res, err := h.Svc.Poll(c.Request.Context(), auth.CurrentUserID(c),
		qrID, sess.DeviceID, sess.InstallID, sess.DeviceName)
	if err != nil {
		log.Printf("qr poll error qr_id=%s: %v", qrID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	switch res.Status {
	case "confirmed", "expired", "cancelled":
		h.QR.Delete(qrID)
	}

	payload := gin.H{"status": res.Status}
	if res.Confirmed && res.Account != nil {
		payload["credential"] = gin.H{
			"vid":      res.Account.VID,
			"nickname": res.Account.Nickname,
			"avatar":   res.Account.Avatar,
		}
	}
	c.JSON(http.StatusOK, payload)
}

func (h *AccountHandlers) List(c *gin.Context) {
	accs, err := h.Svc.List(c.Request.Context(), auth.CurrentUserID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if accs == nil {
		accs = []*model.WeReadAccount{}
	}
	c.JSON(http.StatusOK, accs)
}

func (h *AccountHandlers) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if err := h.Svc.Delete(c.Request.Context(), auth.CurrentUserID(c), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
