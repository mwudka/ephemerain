package main

import (
	"context"
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
	// The delete needs to check if the supplied current value matches the actual value in the database. To avoid a race
	// condition, the check + delete happens in a lua script so that redis performs it atomically.
	// The lua script is sent for each delete rather than being cached because deletes are relatively rare, so the
	// performance hit is less painful than the complexities around replication with cached scripts.
	deleteLuaScript := `
local expectedCurrentValue = ARGV[1]
local actualCurrentValue = redis.call('GET', KEYS[1])
if expectedCurrentValue == actualCurrentValue then
  redis.call('DEL', KEYS[1])
  return true
else
  return error("attempted to delete with wrong current value")
end
`

	key := redisKey(fqdn, recordType)
	return r.client.Eval(ctx, deleteLuaScript, []string{key}, currentValue).Err()
}

func NewRedisRegistrar(redisAddress string) Registrar {
	return RedisRegistrar{
		client: redis.NewClient(&redis.Options{
			Addr: redisAddress,
		}),
	}
}
