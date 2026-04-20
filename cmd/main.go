package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"rate-limited-api/api"
	"rate-limited-api/internal/handler"
	"rate-limited-api/internal/limiter"
	redisclient "rate-limited-api/internal/redis"
	"rate-limited-api/internal/service"
	"rate-limited-api/internal/worker"
)

func main() {
	rc := redisclient.GetClient()

	rl, err := limiter.NewRateLimiter(rc)
	if err != nil {
		log.Fatalf("Failed to initialize rate limiter: %v", err)
	}
	log.Println("Rate limiter initialized")

	svc := service.NewRequestService(rl, rc)
	retryWorker := worker.NewRetryWorker(rc, svc)
	requestHandler := handler.NewRequestHandler(svc, retryWorker)
	statsHandler := handler.NewStatsHandler(svc)

	router := gin.Default()
	api.SetUpRoutes(router, requestHandler, statsHandler)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	var wg sync.WaitGroup
	wg.Go(func() {
		retryWorker.Start(workerCtx)
	})

	srv := &http.Server{
		Addr:    ":8081",
		Handler: router,
	}

	go func() {
		log.Println("Starting server on :8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	workerCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
	wg.Wait()
	log.Println("Server stopped")
}
