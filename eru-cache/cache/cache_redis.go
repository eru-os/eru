package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/redis/go-redis/v9"
)

const (
	redisTagPrefix  = "__tag__:"
	redisLockPrefix = "__lock__:"
	tagScanBatch    = 500
	unlinkBatch     = 500
)

var releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end
`)

// RedisCache is a Redis-backed cache implementation.
type RedisCache struct {
	CacheStore
	RedisAddr       string `json:"redis_addr" eru:"required"`
	RedisPassword   string `json:"redis_password" eru:"required"`
	RedisUsername   string `json:"redis_username"`
	RedisDB         int    `json:"redis_db" eru:"required"`
	TLSEnabled      bool   `json:"tls_enabled"`
	TLSSkipVerify   bool   `json:"tls_skip_verify"`
	TLSServerName   string `json:"tls_server_name"`
	PoolSize        int    `json:"pool_size"`
	MinIdleConns    int    `json:"min_idle_conns"`
	ReadTimeoutMs   int    `json:"read_timeout_ms"`
	WriteTimeoutMs  int    `json:"write_timeout_ms"`
	MaxRetries      int    `json:"max_retries"`
	TagSetTTLSecMax int    `json:"tag_set_ttl_sec_max"`
	Client          *redis.Client
}

// NewRedisCache creates and configures a new Redis cache client.
func NewRedisCache() (rc *RedisCache, err error) {
	rc = &RedisCache{
		CacheStore: CacheStore{CacheStoreType: "REDIS"},
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		err = errors.New("REDIS_ADDR environment variable not set")
		return
	}
	redisPassword := os.Getenv("REDIS_PASSWORD") // Can be empty
	rc.RedisAddr = redisAddr
	rc.RedisPassword = redisPassword
	rc.RedisUsername = os.Getenv("REDIS_USERNAME")
	rc.RedisDB = 0

	rc.TLSEnabled = os.Getenv("REDIS_TLS") == "true"
	rc.TLSSkipVerify = os.Getenv("REDIS_TLS_SKIP_VERIFY") == "true"
	rc.TLSServerName = os.Getenv("REDIS_TLS_SERVER_NAME")

	rc.PoolSize = envInt("REDIS_POOL_SIZE", 20)
	rc.MinIdleConns = envInt("REDIS_MIN_IDLE_CONNS", 2)
	rc.ReadTimeoutMs = envInt("REDIS_READ_TIMEOUT_MS", 3000)
	rc.WriteTimeoutMs = envInt("REDIS_WRITE_TIMEOUT_MS", 3000)
	rc.MaxRetries = envInt("REDIS_MAX_RETRIES", 2)
	rc.TagSetTTLSecMax = envInt("REDIS_TAG_SET_TTL_SEC_MAX", 86400)

	err = rc.Init(context.Background())
	if err != nil {
		return
	}
	return
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (rc *RedisCache) Init(ctx context.Context) error {
	opts := &redis.Options{
		Addr:         rc.RedisAddr,
		Username:     rc.RedisUsername,
		Password:     rc.RedisPassword,
		DB:           rc.RedisDB,
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
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		err = logs.Err(ctx, err, "failed to connect to redis")
		return err
	}
	rc.Client = rdb
	return nil
}

func (rc *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return rc.Client.Get(ctx, key).Result()
}

func (rc *RedisCache) Set(ctx context.Context, key string, value interface{}) error {
	return rc.SetWithTTL(ctx, key, value, 0) // 0 TTL means persist forever
}

func (rc *RedisCache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	p, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return rc.Client.Set(ctx, key, p, ttl).Err()
}

func (rc *RedisCache) SetWithTagsTTL(ctx context.Context, key string, value interface{}, ttl time.Duration, tags []string) error {
	p, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return rc.Client.Set(ctx, key, p, ttl).Err()
	}

	tagSetTTL := ttl
	if rc.TagSetTTLSecMax > 0 {
		maxTTL := time.Duration(rc.TagSetTTLSecMax) * time.Second
		if tagSetTTL == 0 || tagSetTTL > maxTTL {
			tagSetTTL = maxTTL
		}
	}

	pipe := rc.Client.TxPipeline()
	pipe.Set(ctx, key, p, ttl)
	for _, t := range tags {
		if t == "" {
			continue
		}
		tagKey := redisTagPrefix + t
		pipe.SAdd(ctx, tagKey, key)
		if tagSetTTL > 0 {
			pipe.Expire(ctx, tagKey, tagSetTTL)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (rc *RedisCache) InvalidateByTags(ctx context.Context, tags []string) (int, error) {
	if len(tags) == 0 {
		return 0, nil
	}
	deleted := 0
	for _, t := range tags {
		if t == "" {
			continue
		}
		tagKey := redisTagPrefix + t
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

func (rc *RedisCache) AcquireLock(ctx context.Context, key string, ttl time.Duration, owner string) (bool, error) {
	if owner == "" {
		return false, errors.New("lock owner must not be empty")
	}
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return rc.Client.SetNX(ctx, redisLockPrefix+key, owner, ttl).Result()
}

func (rc *RedisCache) ReleaseLock(ctx context.Context, key string, owner string) error {
	if owner == "" {
		return errors.New("lock owner must not be empty")
	}
	_, err := releaseLockScript.Run(ctx, rc.Client, []string{redisLockPrefix + key}, owner).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	return nil
}

func (rc *RedisCache) GetKeys(ctx context.Context, pattern string) ([]string, error) {
	return rc.Client.Keys(ctx, pattern).Result()
}

func (rc *RedisCache) Delete(ctx context.Context, key string) error {
	return rc.Client.Del(ctx, key).Err()
}
func (rc *RedisCache) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &rc)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}
