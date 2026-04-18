package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/cry0404/MyWechatRss/internal/accounts"
)

func respondErr(c *gin.Context, fallbackStatus int, err error) {
	if errors.Is(err, accounts.ErrNoAccount) {
		c.JSON(http.StatusConflict, gin.H{
			"error": err.Error(),
			"code": "no_active_account",
		})
		return
	}
	c.JSON(fallbackStatus, gin.H{"error": err.Error()})
}
