package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/redis/go-redis/v9"
)

// RedisClusterCache backs CacheStoreI with go-redis ClusterClient. Designed
// for AWS ElastiCache for Redis with cluster-mode enabled, but works against
// any Redis Cluster.
//
// Cluster correctness: every key passes through clusterPrefix() which wraps
// it in a single hash tag (default "{eruql}"). Redis Cluster routes keys by
// the hash tag substring, so all eru-ql traffic lands in one slot. That
// removes CROSSSLOT errors for the SET+SADD+EXPIRE pipeline and the
// SSCAN+UNLINK invalidation loop, at the cost of pinning the workload to
// one shard. If you need to spread across shards, run multiple datasources
// with different HashTag values.
type RedisClusterCache struct {
	CacheStore
	RedisAddr       string   `json:"redis_addr" eru:"required"`
	RedisPassword   string   `json:"redis_password"`
	RedisUsername   string   `json:"redis_username"`
	TLSEnabled      bool     `json:"tls_enabled"`
	TLSSkipVerify   bool     `json:"tls_skip_verify"`
	TLSServerName   string   `json:"tls_server_name"`
	HashTag         string   `json:"hash_tag"`
	PoolSize        int      `json:"pool_size"`
	MinIdleConns    int      `json:"min_idle_conns"`
	ReadTimeoutMs   int      `json:"read_timeout_ms"`
	WriteTimeoutMs  int      `json:"write_timeout_ms"`
	MaxRetries      int      `json:"max_retries"`
	TagSetTTLSecMax int      `json:"tag_set_ttl_sec_max"`
	Client          *redis.ClusterClient
}

func NewRedisClusterCache() (*RedisClusterCache, error) {
	rc := &RedisClusterCache{
		CacheStore: CacheStore{CacheStoreType: "REDIS_CLUSTER"},
	}
	addrs := os.Getenv("REDIS_CLUSTER_ADDRS")
	if addrs == "" {
		return rc, errors.New("REDIS_CLUSTER_ADDRS environment variable not set")
	}
	rc.RedisAddr = addrs
	rc.RedisPassword = os.Getenv("REDIS_CLUSTER_PASSWORD")
	rc.RedisUsername = os.Getenv("REDIS_CLUSTER_USERNAME")
	rc.TLSEnabled = os.Getenv("REDIS_CLUSTER_TLS") == "true"
	rc.TLSSkipVerify = os.Getenv("REDIS_CLUSTER_TLS_SKIP_VERIFY") == "true"
	rc.TLSServerName = os.Getenv("REDIS_CLUSTER_TLS_SERVER_NAME")
	rc.PoolSize = envInt("REDIS_CLUSTER_POOL_SIZE", 20)
	rc.MinIdleConns = envInt("REDIS_CLUSTER_MIN_IDLE_CONNS", 2)
	rc.ReadTimeoutMs = envInt("REDIS_CLUSTER_READ_TIMEOUT_MS", 3000)
	rc.WriteTimeoutMs = envInt("REDIS_CLUSTER_WRITE_TIMEOUT_MS", 3000)
	rc.MaxRetries = envInt("REDIS_CLUSTER_MAX_RETRIES", 2)
	rc.TagSetTTLSecMax = envInt("REDIS_CLUSTER_TAG_SET_TTL_SEC_MAX", 86400)

	if err := rc.Init(context.Background()); err != nil {
		return rc, err
	}
	return rc, nil
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (rc *RedisClusterCache) Init(ctx context.Context) error {
	addrs := splitAndTrim(rc.RedisAddr)
	if len(addrs) == 0 {
		return errors.New("redis_addr is empty")
	}
	if rc.HashTag == "" {
		rc.HashTag = "eruql"
	}
	opts := &redis.ClusterOptions{
		Addrs:        addrs,
		Username:     rc.RedisUsername,
		Password:     rc.RedisPassword,
		PoolSize:     rc.PoolSize,
		MinIdleConns: rc.MinIdleConns,
		ReadTimeout:  time.Duration(rc.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(rc.WriteTimeoutMs) * time.Millisecond,
		MaxRetries:   rc.MaxRetries,
	}
	if rc.TLSEnabled {
		opts.TLSConfig = &tls.Config{
			InsecureSkipVerify: rc.TLSSkipVerify,
			ServerName:         rc.TLSServerName,
		}
	}
	cli := redis.NewClusterClient(opts)
	if err := cli.Ping(context.Background()).Err(); err != nil {
		return logs.Err(ctx, err, "failed to connect to redis cluster")
	}
	rc.Client = cli
	return nil
}

// clusterPrefix wraps a raw key in the configured hash tag so that all keys
// hashing through this struct route to the same slot. Idempotent: if k is
// already prefixed, return as-is.
func (rc *RedisClusterCache) clusterPrefix(k string) string {
	tag := "{" + rc.HashTag + "}"
	if strings.HasPrefix(k, tag) {
		return k
	}
	return tag + k
}

func (rc *RedisClusterCache) Get(ctx context.Context, key string) (string, error) {
	return rc.Client.Get(ctx, rc.clusterPrefix(key)).Result()
}

func (rc *RedisClusterCache) Set(ctx context.Context, key string, value interface{}) error {
	return rc.SetWithTTL(ctx, key, value, 0)
}

func (rc *RedisClusterCache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	p, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return rc.Client.Set(ctx, rc.clusterPrefix(key), p, ttl).Err()
}

func (rc *RedisClusterCache) SetWithTagsTTL(ctx context.Context, key string, value interface{}, ttl time.Duration, tags []string) error {
	p, err := json.Marshal(value)
	if err != nil {
		return err
	}
	prefixedKey := rc.clusterPrefix(key)
	if len(tags) == 0 {
		return rc.Client.Set(ctx, prefixedKey, p, ttl).Err()
	}

	tagSetTTL := ttl
	if rc.TagSetTTLSecMax > 0 {
		maxTTL := time.Duration(rc.TagSetTTLSecMax) * time.Second
		if tagSetTTL == 0 || tagSetTTL > maxTTL {
			tagSetTTL = maxTTL
		}
	}

	pipe := rc.Client.TxPipeline()
	pipe.Set(ctx, prefixedKey, p, ttl)
	for _, t := range tags {
		if t == "" {
			continue
		}
		tagKey := rc.clusterPrefix(redisTagPrefix + t)
		pipe.SAdd(ctx, tagKey, prefixedKey)
		if tagSetTTL > 0 {
			pipe.Expire(ctx, tagKey, tagSetTTL)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (rc *RedisClusterCache) InvalidateByTags(ctx context.Context, tags []string) (int, error) {
	if len(tags) == 0 {
		return 0, nil
	}
	deleted := 0
	for _, t := range tags {
		if t == "" {
			continue
		}
		tagKey := rc.clusterPrefix(redisTagPrefix + t)
		var cursor uint64
		for {
			members, next, err := rc.Client.SScan(ctx, tagKey, cursor, "", tagScanBatch).Result()
			if err != nil {
				return deleted, err
			}
			for start := 0; start < len(members); start += unlinkBatch {
				end := start + unlinkBatch
				if end > len(members) {
					end = len(members)
				}
				batch := members[start:end]
				if len(batch) == 0 {
					continue
				}
				n, uerr := rc.Client.Unlink(ctx, batch...).Result()
				if uerr != nil {
					return deleted, uerr
				}
				deleted += int(n)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		if err := rc.Client.Unlink(ctx, tagKey).Err(); err != nil {
			return deleted, err
		}
	}
	return deleted, nil
}

func (rc *RedisClusterCache) AcquireLock(ctx context.Context, key string, ttl time.Duration, owner string) (bool, error) {
	if owner == "" {
		return false, errors.New("lock owner must not be empty")
	}
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return rc.Client.SetNX(ctx, rc.clusterPrefix(redisLockPrefix+key), owner, ttl).Result()
}

func (rc *RedisClusterCache) ReleaseLock(ctx context.Context, key string, owner string) error {
	if owner == "" {
		return errors.New("lock owner must not be empty")
	}
	_, err := releaseLockScript.Run(ctx, rc.Client, []string{rc.clusterPrefix(redisLockPrefix + key)}, owner).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	return nil
}

func (rc *RedisClusterCache) GetKeys(ctx context.Context, pattern string) ([]string, error) {
	prefixed := rc.clusterPrefix(pattern)
	var out []string
	err := rc.Client.ForEachMaster(ctx, func(ctx context.Context, master *redis.Client) error {
		keys, kerr := master.Keys(ctx, prefixed).Result()
		if kerr != nil {
			return kerr
		}
		out = append(out, keys...)
		return nil
	})
	return out, err
}

func (rc *RedisClusterCache) Delete(ctx context.Context, key string) error {
	return rc.Client.Del(ctx, rc.clusterPrefix(key)).Err()
}

func (rc *RedisClusterCache) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	if err := json.Unmarshal(*rj, &rc); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
