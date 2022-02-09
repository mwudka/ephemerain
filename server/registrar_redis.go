package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"strings"
)

type RedisRegistrar struct {
	client *redis.Client
}

func redisKey(fqdn Domain, recordType RecordType) string {
	return fmt.Sprintf("%s:%s", strings.ToLower(string(fqdn)), recordType)
}

func (r RedisRegistrar) SetRecord(ctx context.Context, fqdn Domain, recordType RecordType, value string) {
	key := redisKey(fqdn, recordType)
	// TODO: Error handling
	r.client.Set(ctx, key, value, 0)
}

func (r RedisRegistrar) GetRecord(ctx context.Context, fqdn Domain, recordType RecordType) (string, error) {
	key := redisKey(fqdn, recordType)
	status := r.client.Get(ctx, key)
	return status.Val(), status.Err()
}

func NewRedisRegistrar(redisAddress string) Registrar {
	return RedisRegistrar{
		client: redis.NewClient(&redis.Options{
			Addr: redisAddress,
		}),
	}
}