package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	context2 "golang.org/x/net/context"
	"strings"
)

type RedisRegistrar struct {
	client *redis.Client
}

func redisKey(fqdn Domain, recordType RecordType) string {
	return fmt.Sprintf("%s:%s", strings.ToLower(string(fqdn)), recordType)
}

func (r RedisRegistrar) SetRecord(ctx context2.Context, fqdn Domain, recordType RecordType, value string) error {
	key := redisKey(fqdn, recordType)
	return r.client.Set(ctx, key, value, 0).Err()
}

func (r RedisRegistrar) GetRecord(ctx context.Context, fqdn Domain, recordType RecordType) (string, error) {
	key := redisKey(fqdn, recordType)
	status := r.client.Get(ctx, key)
	return status.Val(), status.Err()
}

func (r RedisRegistrar) DeleteRecord(ctx context.Context, fqdn Domain, recordType RecordType, currentValue string) error {
	// TODO: This is racy, because there could be a write between the get and delete. To fix this,
	// probably need to implement a lua method that atomically deletes if value matches
	key := redisKey(fqdn, recordType)
	status := r.client.Get(ctx, key)
	if status.Err() != nil {
		return status.Err()
	}
	if status.Val() != currentValue {
		return errors.New("attempted to delete record but supplied wrong current value")
	}
	return r.client.Del(ctx, key).Err()
}

func NewRedisRegistrar(redisAddress string) Registrar {
	return RedisRegistrar{
		client: redis.NewClient(&redis.Options{
			Addr: redisAddress,
		}),
	}
}
