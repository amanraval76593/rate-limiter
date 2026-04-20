package api

import (
	"net/http"

	"rate-limited-api/internal/handler"

	"github.com/gin-gonic/gin"
)

func SetUpRoutes(
	router *gin.Engine,
	requestHandler *handler.RequestHandler,
	statsHandler *handler.StatsHandler,
) {
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	router.POST("/request", requestHandler.Handle)
	router.GET("/stats", statsHandler.Handle)
}
