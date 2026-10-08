package handler

import (
	"context"
	"log"
	"net/http"
	"taskqueue/internal/store"
	"time"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func Ready(c *gin.Context) {
	sqlDB, err := store.DB.DB()
	if err != nil {
		log.Printf("ready: get sql.DB failed :%v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		log.Printf("ready :db ping failed :%v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
