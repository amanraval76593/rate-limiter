package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"rate-limited-api/internal/service"
)

const (
	RetryQueue   = "retry_queue"
	PollInterval = 2 * time.Second
	MaxRetries   = 3
)

type RetryEntry struct {
	RequestID string                 `json:"request_id"`
	UserID    string                 `json:"user_id"`
	Payload   map[string]interface{} `json:"payload"`
	RetryAt   int64                  `json:"retry_at"`
	Retries   int                    `json:"retries"`
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
		RequestID: uuid.NewString(),
		UserID:    userID,
		Payload:   payload,
		RetryAt:   time.Now().Unix() + retryAfter,
		Retries:   0,
	}

	if err := w.enqueueEntry(ctx, entry); err != nil {
		return err
	}

	log.Printf(
		"Retry worker: queued request %s for user %s at retry_at=%d",
		entry.RequestID,
		entry.UserID,
		entry.RetryAt,
	)

	return nil
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
	entry, data, err := w.peekNextEntry(ctx)
	if err != nil {
		log.Printf("Retry worker: failed to inspect queue: %v", err)
		return
	}
	if entry == nil {
		return
	}

	now := time.Now().Unix()
	if now < entry.RetryAt {
		log.Printf(
			"Retry worker: request %s for user %s not ready yet (retry_at=%d, now=%d), leaving queued",
			entry.RequestID,
			entry.UserID,
			entry.RetryAt,
			now,
		)
		return
	}

	removed, err := w.redisClient.ZRem(ctx, RetryQueue, data).Result()
	if err != nil {
		log.Printf("Retry worker: failed to claim request %s for user %s: %v", entry.RequestID, entry.UserID, err)
		return
	}
	if removed == 0 {
		return
	}

	allowed, retryAfter, err := w.service.ProcessRequest(ctx, entry.UserID, entry.Payload)
	if err != nil {
		log.Printf("Retry worker: error processing request %s for user %s: %v", entry.RequestID, entry.UserID, err)
		entry.Retries++
		if entry.Retries < MaxRetries {
			entry.RetryAt = now + retryAfter + int64(entry.Retries*5)
			if err := w.enqueueEntry(ctx, *entry); err != nil {
				log.Printf("Retry worker: failed to re-queue request %s for user %s: %v", entry.RequestID, entry.UserID, err)
				return
			}
			log.Printf(
				"Retry worker: request %s for user %s failed, re-queued (attempt %d/%d, retry_at=%d)",
				entry.RequestID,
				entry.UserID,
				entry.Retries,
				MaxRetries,
				entry.RetryAt,
			)
		} else {
			log.Printf("Retry worker: max retries reached for request %s user %s, dropping request", entry.RequestID, entry.UserID)
		}
		return
	}

	if allowed {
		log.Printf("Retry worker: request %s for user %s succeeded on retry", entry.RequestID, entry.UserID)
	} else {
		entry.Retries++
		if entry.Retries < MaxRetries {
			entry.RetryAt = now + retryAfter
			if err := w.enqueueEntry(ctx, *entry); err != nil {
				log.Printf("Retry worker: failed to re-queue request %s for user %s: %v", entry.RequestID, entry.UserID, err)
				return
			}
			log.Printf(
				"Retry worker: request %s for user %s still rate-limited, re-queued (attempt %d/%d, retry_at=%d)",
				entry.RequestID,
				entry.UserID,
				entry.Retries,
				MaxRetries,
				entry.RetryAt,
			)
		} else {
			log.Printf("Retry worker: max retries reached for request %s user %s, dropping request", entry.RequestID, entry.UserID)
		}
	}
}

func (w *RetryWorker) enqueueEntry(ctx context.Context, entry RetryEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal retry entry: %w", err)
	}

	return w.redisClient.ZAdd(ctx, RetryQueue, redis.Z{
		Score:  float64(entry.RetryAt),
		Member: string(data),
	}).Err()
}

func (w *RetryWorker) peekNextEntry(ctx context.Context) (*RetryEntry, string, error) {
	results, err := w.redisClient.ZRangeWithScores(ctx, RetryQueue, 0, 0).Result()
	if err == redis.Nil || len(results) == 0 {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}

	data, ok := results[0].Member.(string)
	if !ok {
		return nil, "", fmt.Errorf("unexpected retry queue member type %T", results[0].Member)
	}

	var entry RetryEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal retry entry: %w", err)
	}

	return &entry, data, nil
}
