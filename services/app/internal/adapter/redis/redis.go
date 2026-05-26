package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	redis *redis.Client
}

func NewRedis(addr, pass string) (*Cache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       0,
	})

	ctx := context.Background()

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}
	return &Cache{redis: rdb}, nil
}

func (c *Cache) SaveToCache(ctx context.Context, key string, value interface{}) error {
	return c.redis.Set(ctx, key, value, 3*time.Hour).Err()
}

func (c *Cache) GetFromCache(ctx context.Context, key string) (string, error) {
	return c.redis.GetDel(ctx, key).Result() // redis >= 6.2.0
}
