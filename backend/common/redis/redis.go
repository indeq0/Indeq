package redis

import (
	"context"
	"fmt"	
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	Client *redis.Client
}

func NewRedisClient(ctx context.Context, db int) (*RedisClient, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDRESS"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB: db,
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisClient{Client: rdb}, nil
}

func (c *RedisClient) StoreOAuthState(ctx context.Context, state string, userId string) error {
	log.Printf("Storing oauth state: %s for user %s", state, userId)
    key := fmt.Sprintf("oauth:state:%s", state)
    err := c.Client.Set(ctx, key, userId, 5*time.Minute).Err()
	if err != nil {
		return fmt.Errorf("failed to store oauth state: %w", err)
	}
    log.Printf("Stored oauth state %s for user %s", key, userId) 
	return nil
}

func (c *RedisClient) ValidateOAuthState(ctx context.Context, state string) (string, error) {
    key := fmt.Sprintf("oauth:state:%s", state)

    userId, err := c.Client.Get(ctx, key).Result()
    if err == redis.Nil {
        return "", fmt.Errorf("state not found or expired")
    }
    if err != nil {
        return "", fmt.Errorf("could not get state from Redis: %w", err)
    }

    delCount, err := c.Client.Del(ctx, key).Result()
    if err != nil {
        return "", fmt.Errorf("could not delete state from Redis: %w", err)
    }
    if delCount == 0 {
        return "", fmt.Errorf("state key was not deleted (key may not exist)")
    } else {
		log.Printf("Deleted state %s from Redis for user %s", key, userId)
	}

    return userId, nil
}

func (c *RedisClient) Incr(ctx context.Context, key string, amount int64) (int64, error) {
	count, err := c.Client.IncrBy(ctx, key, amount).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment key: %w", err)
	}
	return count, nil
}

func (c *RedisClient) Expire(ctx context.Context, key string, duration time.Duration) error {
	err := c.Client.Expire(ctx, key, duration).Err()
	if err != nil {
		return fmt.Errorf("failed to expire key: %w", err)
	}
	return nil
}

func (c *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	err := c.Client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set key: %w", err)
	}
	return nil
}

func (c *RedisClient) Get(ctx context.Context, key string) (string, error) {
	value, err := c.Client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("failed to get key: %w", err)
	}
	return value, nil
}

func (c *RedisClient) Del(ctx context.Context, key string) error {
	err := c.Client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

func (c *RedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	success, err := c.Client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return false, fmt.Errorf("failed to setnx key: %w", err)
	}
	return success, nil
}