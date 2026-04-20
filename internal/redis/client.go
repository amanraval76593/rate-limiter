package redis

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
)

var (
	client *redis.Client
	once   sync.Once
)

func GetClient() *redis.Client {
	once.Do(func() {
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "localhost:6382"
		}

		password := os.Getenv("REDIS_PASSWORD")

		db := 0
		if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
			parsed, err := strconv.Atoi(dbStr)
			if err == nil {
				db = parsed
			}
		}

		client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		})

		if err := client.Ping(context.Background()).Err(); err != nil {
			fmt.Printf("WARNING: Redis ping failed: %v\n", err)
		} else {
			fmt.Println("Connected to Redis at", addr)
		}
	})

	return client
}
