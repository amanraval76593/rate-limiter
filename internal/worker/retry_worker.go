package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"rate-limited-api/internal/service"
)

const (
	RetryQueue   = "retry_queue"
	PollInterval = 2 * time.Second
	MaxRetries   = 3
)

type RetryEntry struct {
	UserID  string                 `json:"user_id"`
	Payload map[string]interface{} `json:"payload"`
	RetryAt int64                  `json:"retry_at"`
	Retries int                    `json:"retries"`
}

type RetryWorker struct {
	redisClient *redis.Client
	service     *service.RequestService
}

func NewRetryWorker(rc *redis.Client, svc *service.RequestService) *RetryWorker {
	return &RetryWorker{
		redisClient: rc,
		service:     svc,
	}
}

func (w *RetryWorker) EnqueueRetry(ctx context.Context, userID string, payload map[string]interface{}, retryAfter int64) error {
	entry := RetryEntry{
		UserID:  userID,
		Payload: payload,
		RetryAt: time.Now().Unix() + retryAfter,
		Retries: 0,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal retry entry: %w", err)
	}

	return w.redisClient.LPush(ctx, RetryQueue, data).Err()
}

func (w *RetryWorker) Start(ctx context.Context) {
	log.Println("Retry worker started")

	for {
		select {
		case <-ctx.Done():
			log.Println("Retry worker shutting down")
			return
		default:
			w.processOne(ctx)
			time.Sleep(PollInterval)
		}
	}
}

func (w *RetryWorker) processOne(ctx context.Context) {
	data, err := w.redisClient.RPop(ctx, RetryQueue).Result()
	if err == redis.Nil {
		return
	}
	if err != nil {
		log.Printf("Retry worker: failed to pop from queue: %v", err)
		return
	}

	var entry RetryEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		log.Printf("Retry worker: failed to unmarshal entry: %v", err)
		return
	}

	now := time.Now().Unix()
	if now < entry.RetryAt {
		w.redisClient.LPush(ctx, RetryQueue, data)
		return
	}

	allowed, retryAfter, err := w.service.ProcessRequest(ctx, entry.UserID, entry.Payload)
	if err != nil {
		log.Printf("Retry worker: error processing request for user %s: %v", entry.UserID, err)
		entry.Retries++
		if entry.Retries < MaxRetries {
			entry.RetryAt = now + retryAfter + int64(entry.Retries*5)
			reData, _ := json.Marshal(entry)
			w.redisClient.LPush(ctx, RetryQueue, reData)
		} else {
			log.Printf("Retry worker: max retries reached for user %s, dropping request", entry.UserID)
		}
		return
	}

	if allowed {
		log.Printf("Retry worker: request for user %s succeeded on retry", entry.UserID)
	} else {
		entry.Retries++
		if entry.Retries < MaxRetries {
			entry.RetryAt = now + retryAfter
			reData, _ := json.Marshal(entry)
			w.redisClient.LPush(ctx, RetryQueue, reData)
			log.Printf("Retry worker: user %s still rate-limited, re-queued (attempt %d/%d)",
				entry.UserID, entry.Retries, MaxRetries)
		} else {
			log.Printf("Retry worker: max retries reached for user %s, dropping request", entry.UserID)
		}
	}
}
