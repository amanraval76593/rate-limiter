package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	"rate-limited-api/internal/limiter"
	"rate-limited-api/internal/model"
)

const (
	StatsKeyPrefix = "stats:"
)

type RequestService struct {
	limiter     *limiter.RateLimiter
	redisClient *redis.Client
}

func NewRequestService(rl *limiter.RateLimiter, rc *redis.Client) *RequestService {
	return &RequestService{
		limiter:     rl,
		redisClient: rc,
	}
}

func (s *RequestService) ProcessRequest(ctx context.Context, userID string, payload map[string]interface{}) (bool, int64, error) {
	result, err := s.limiter.Check(ctx, userID)
	if err != nil {
		return false, 0, fmt.Errorf("rate limiter error: %w", err)
	}

	statsKey := StatsKeyPrefix + userID

	pipe := s.redisClient.Pipeline()
	pipe.HIncrBy(ctx, statsKey, "total_requests", 1)

	if result.Allowed {
		pipe.HIncrBy(ctx, statsKey, "allowed", 1)
	} else {
		pipe.HIncrBy(ctx, statsKey, "blocked", 1)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("stats update failed: %w", err)
	}

	return result.Allowed, result.RetryAfter, nil
}

func (s *RequestService) GetStats(ctx context.Context, userID string) (*model.StatsResponse, error) {
	statsKey := StatsKeyPrefix + userID

	vals, err := s.redisClient.HGetAll(ctx, statsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	stats := &model.StatsResponse{
		UserID: userID,
	}

	if v, ok := vals["total_requests"]; ok {
		stats.TotalRequests, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["allowed"]; ok {
		stats.Allowed, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, ok := vals["blocked"]; ok {
		stats.Blocked, _ = strconv.ParseInt(v, 10, 64)
	}

	return stats, nil
}
