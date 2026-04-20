package api

import (
	"fmt"
	"log"
	"rate-limited-api/internal/handler"
	"rate-limited-api/internal/limiter"
	redisclient "rate-limited-api/internal/redis"
	"rate-limited-api/internal/service"
	"rate-limited-api/internal/worker"

	"github.com/gin-gonic/gin"
)

func SetUpRoutes(router *gin.Engine) {
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{
			"status": "ok",
		})
	})
	rc := redisclient.GetClient()
	rl, err := limiter.NewRateLimiter(rc)
	if err != nil {
		log.Fatalf("Failed to initialize rate limiter: %v", err)
	}
	fmt.Println("Rate limiter initialized")
	svc := service.NewRequestService(rl, rc)
	retryWorker := worker.NewRetryWorker(rc, svc)

	requestHandler := handler.NewRequestHandler(svc, retryWorker)
	statsHandler := handler.NewStatsHandler(svc)

	router.POST("/request", requestHandler.Handle)
	router.GET("/stats", statsHandler.Handle)
}
