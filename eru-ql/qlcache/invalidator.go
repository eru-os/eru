package qlcache

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eru-os/eru/eru-cache/cache"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
)

const (
	invalidateQueueSize = 1024
	invalidateTimeout   = 10 * time.Second
)

type invalidateMsg struct {
	store cache.CacheStoreI
	tags  []string
}

var (
	invalidateQueue   chan invalidateMsg
	invalidateOnce    sync.Once
	invalidateStarted atomic.Bool
	invalidateDropCnt atomic.Int64
)

// StartInvalidator spins up the async invalidation workers. Safe to call
// multiple times (the first call wins). workers<=0 falls back to env var
// ERUQL_CACHE_INVALIDATE_WORKERS or 2.
func StartInvalidator(ctx context.Context, workers int) {
	invalidateOnce.Do(func() {
		if workers <= 0 {
			if v := os.Getenv("ERUQL_CACHE_INVALIDATE_WORKERS"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					workers = n
				}
			}
		}
		if workers <= 0 {
			workers = 2
		}
		invalidateQueue = make(chan invalidateMsg, invalidateQueueSize)
		for i := 0; i < workers; i++ {
			go invalidateWorker()
		}
		invalidateStarted.Store(true)
		logs.WithContext(ctx).Info("qlcache invalidator started")
	})
}

func invalidateWorker() {
	for msg := range invalidateQueue {
		ctx, cancel := context.WithTimeout(context.Background(), invalidateTimeout)
		if _, err := msg.store.InvalidateByTags(ctx, msg.tags); err != nil {
			globalCounters.errors.Add(1)
			logs.WithContext(ctx).Debug("qlcache invalidate error: " + err.Error())
		}
		cancel()
	}
}

// EnqueueInvalidate drops an invalidation message onto the async queue.
// Non-blocking: if the queue is full the message is dropped and a counter
// incremented; TTL is the backstop, and the subsequent request's fresh read
// will repopulate.
func EnqueueInvalidate(ctx context.Context, ds *module_model.DataSource, tables []string) {
	if !enabled(ds) || len(tables) == 0 {
		return
	}
	if !invalidateStarted.Load() {
		// start lazily with defaults so callers don't have to remember to init
		StartInvalidator(ctx, 0)
	}
	store := ds.GetQueryCache()
	tags := buildTagList(ds.ProjectId, ds.DbAlias, tables)
	if len(tags) == 0 {
		return
	}
	select {
	case invalidateQueue <- invalidateMsg{store: store, tags: tags}:
	default:
		invalidateDropCnt.Add(1)
		logs.WithContext(ctx).Info("qlcache invalidate queue full; message dropped")
	}
}

// InvalidateBlocking is a synchronous variant used by the admin endpoints
// where the operator wants a confirmation. It bypasses the queue.
func InvalidateBlocking(ctx context.Context, ds *module_model.DataSource, tables []string) (deleted int, err error) {
	if !enabled(ds) || len(tables) == 0 {
		return 0, nil
	}
	store := ds.GetQueryCache()
	tags := buildTagList(ds.ProjectId, ds.DbAlias, tables)
	if len(tags) == 0 {
		return 0, nil
	}
	return store.InvalidateByTags(ctx, tags)
}

func buildTagList(projectId, dsAlias string, tables []string) []string {
	tags := make([]string, 0, len(tables))
	seen := make(map[string]struct{}, len(tables))
	for _, t := range tables {
		if t == "" {
			continue
		}
		tag := TableTag(projectId, dsAlias, t)
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

// DropCount exposes the dropped-message counter so admin stats can report on it.
func DropCount() int64 { return invalidateDropCnt.Load() }
