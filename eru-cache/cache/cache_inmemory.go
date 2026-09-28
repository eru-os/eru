package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
)

// InMemoryCache is an in-memory cache implementation.
type InMemoryCache struct {
	CacheStore
	// CacheValues are the cache's live contents - conversations, query results,
	// whatever callers put in it. They are state, never configuration, so they
	// are never serialised: the store is embedded in configs (an agent's
	// chat_memory, a datasource's query_cache) that are marshalled to be saved,
	// cloned and listed. Tagged for JSON, one shared store's contents were
	// written into every config that used it - other tenants' conversations
	// included - persisted to the config database on every save, and loaded
	// back at startup. json.Marshal also read this map without the lock.
	CacheValues map[string]CacheValue `json:"-"`
	mu          sync.Mutex
	tags        map[string]map[string]struct{} // tag -> set of keys
	keyTags     map[string]map[string]struct{} // key -> set of tags
	locks       map[string]inMemLock
}

// CacheValue holds the data for an in-memory cache item.
type CacheValue struct {
	Key      string      `json:"key"`
	Value    interface{} `json:"value"`
	ExpireAt time.Time   `json:"expire_at,omitempty"`
}

type inMemLock struct {
	owner    string
	expireAt time.Time
}

// NewInMemoryCache creates a new in-memory cache.
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		CacheStore:  CacheStore{CacheStoreType: "INMEMORY"},
		CacheValues: make(map[string]CacheValue),
		tags:        make(map[string]map[string]struct{}),
		keyTags:     make(map[string]map[string]struct{}),
		locks:       make(map[string]inMemLock),
	}
}

func (imc *InMemoryCache) ensureInit() {
	if imc.CacheValues == nil {
		imc.CacheValues = make(map[string]CacheValue)
	}
	if imc.tags == nil {
		imc.tags = make(map[string]map[string]struct{})
	}
	if imc.keyTags == nil {
		imc.keyTags = make(map[string]map[string]struct{})
	}
	if imc.locks == nil {
		imc.locks = make(map[string]inMemLock)
	}
}

func (imc *InMemoryCache) Get(ctx context.Context, key string) (string, error) {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()

	cv, ok := imc.CacheValues[key]
	if !ok {
		return "", fmt.Errorf("cache key %s not found", key)
	}
	if !cv.ExpireAt.IsZero() && time.Now().After(cv.ExpireAt) {
		imc.deleteLocked(key)
		return "", fmt.Errorf("cache key %s not found", key)
	}
	logs.WithContext(ctx).Info(fmt.Sprintf("cache key %s found", key))
	if str, ok := cv.Value.(string); ok {
		return str, nil
	}
	res, err := json.Marshal(cv.Value)
	if err != nil {
		return "", err
	}
	return string(res), nil
}

func (imc *InMemoryCache) Set(ctx context.Context, key string, value interface{}) error {
	return imc.SetWithTTL(ctx, key, value, 0)
}

func (imc *InMemoryCache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()
	imc.setLocked(key, value, ttl, nil)
	return nil
}

func (imc *InMemoryCache) SetWithTagsTTL(ctx context.Context, key string, value interface{}, ttl time.Duration, tags []string) error {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()
	imc.setLocked(key, value, ttl, tags)
	return nil
}

func (imc *InMemoryCache) setLocked(key string, value interface{}, ttl time.Duration, tags []string) {
	cv := CacheValue{Key: key, Value: value}
	if ttl > 0 {
		cv.ExpireAt = time.Now().Add(ttl)
	}
	imc.CacheValues[key] = cv

	// clear old tag associations for this key
	if oldTags, ok := imc.keyTags[key]; ok {
		for t := range oldTags {
			if set, sOk := imc.tags[t]; sOk {
				delete(set, key)
				if len(set) == 0 {
					delete(imc.tags, t)
				}
			}
		}
		delete(imc.keyTags, key)
	}

	if len(tags) == 0 {
		return
	}
	kt := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		if t == "" {
			continue
		}
		if _, ok := imc.tags[t]; !ok {
			imc.tags[t] = make(map[string]struct{})
		}
		imc.tags[t][key] = struct{}{}
		kt[t] = struct{}{}
	}
	imc.keyTags[key] = kt
}

func (imc *InMemoryCache) InvalidateByTags(ctx context.Context, tags []string) (int, error) {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()

	seen := make(map[string]struct{})
	for _, t := range tags {
		if t == "" {
			continue
		}
		set, ok := imc.tags[t]
		if !ok {
			continue
		}
		for k := range set {
			seen[k] = struct{}{}
		}
	}
	count := 0
	for k := range seen {
		if _, ok := imc.CacheValues[k]; ok {
			count++
		}
		imc.deleteLocked(k)
	}
	return count, nil
}

func (imc *InMemoryCache) AcquireLock(ctx context.Context, key string, ttl time.Duration, owner string) (bool, error) {
	if owner == "" {
		return false, fmt.Errorf("lock owner must not be empty")
	}
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()

	now := time.Now()
	if existing, ok := imc.locks[key]; ok && now.Before(existing.expireAt) {
		return false, nil
	}
	imc.locks[key] = inMemLock{owner: owner, expireAt: now.Add(ttl)}
	return true, nil
}

func (imc *InMemoryCache) ReleaseLock(ctx context.Context, key string, owner string) error {
	if owner == "" {
		return fmt.Errorf("lock owner must not be empty")
	}
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()

	if existing, ok := imc.locks[key]; ok && existing.owner == owner {
		delete(imc.locks, key)
	}
	return nil
}

func (imc *InMemoryCache) GetKeys(ctx context.Context, pattern string) ([]string, error) {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()

	now := time.Now()
	var keys []string
	for k, cv := range imc.CacheValues {
		if !cv.ExpireAt.IsZero() && now.After(cv.ExpireAt) {
			continue
		}
		if pattern == "" || pattern == "*" {
			keys = append(keys, k)
			continue
		}
		ok, err := path.Match(pattern, k)
		if err == nil && ok {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// PurgeExpired drops every entry whose ttl has passed and reports how many went.
//
// Expiry elsewhere in this store is lazy: a read of a stale key deletes it. That
// is enough for a key someone comes back to and does nothing at all for one they
// abandon - and an abandoned entry is precisely the one still holding memory.
// GetKeys hides expired entries rather than removing them, so a caller cannot
// reclaim them by walking the keys either.
func (imc *InMemoryCache) PurgeExpired(ctx context.Context) int {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()

	now := time.Now()
	expired := make([]string, 0)
	for k, cv := range imc.CacheValues {
		if !cv.ExpireAt.IsZero() && now.After(cv.ExpireAt) {
			expired = append(expired, k)
		}
	}
	for _, k := range expired {
		imc.deleteLocked(k)
	}
	if len(expired) > 0 {
		logs.WithContext(ctx).Info(fmt.Sprintf("purged %d expired cache entr(ies)", len(expired)))
	}
	return len(expired)
}

func (imc *InMemoryCache) Delete(ctx context.Context, key string) error {
	imc.mu.Lock()
	defer imc.mu.Unlock()
	imc.ensureInit()
	imc.deleteLocked(key)
	return nil
}

func (imc *InMemoryCache) deleteLocked(key string) {
	delete(imc.CacheValues, key)
	if ts, ok := imc.keyTags[key]; ok {
		for t := range ts {
			if set, sOk := imc.tags[t]; sOk {
				delete(set, key)
				if len(set) == 0 {
					delete(imc.tags, t)
				}
			}
		}
		delete(imc.keyTags, key)
	}
}

func (imc *InMemoryCache) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &imc)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	imc.ensureInit()
	return nil
}
