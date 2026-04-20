package limiter

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	Window    = 60
	Limit     = 5
	KeyPrefix = "rate_limit:"
)

type RateLimiter struct {
	client *redis.Client
	script *redis.Script
}

func NewRateLimiter(client *redis.Client) (*RateLimiter, error) {
	scriptBytes, err := os.ReadFile("scripts/limiter.lua")
	if err != nil {
		return nil, fmt.Errorf("failed to read Lua script: %w", err)
	}

	script := redis.NewScript(string(scriptBytes))

	return &RateLimiter{
		client: client,
		script: script,
	}, nil
}

type Result struct {
	Allowed    bool
	RetryAfter int64
}

func (rl *RateLimiter) Check(ctx context.Context, userID string) (*Result, error) {
	key := KeyPrefix + userID
	now := time.Now().Unix()
	uniqueID := uuid.New().String()

	res, err := rl.script.Run(ctx, rl.client, []string{key},
		now,
		Window,
		Limit,
		uniqueID,
	).Int64Slice()

	if err != nil {
		return nil, fmt.Errorf("lua script execution failed: %w", err)
	}

	if len(res) < 2 {
		return nil, fmt.Errorf("unexpected Lua script response: %v", res)
	}

	return &Result{
		Allowed:    res[0] == 1,
		RetryAfter: res[1],
	}, nil
}
