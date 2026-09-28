package qlcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
	"golang.org/x/sync/singleflight"
)

// Loader is invoked on cache miss to produce a fresh result and the list of
// unqualified table names that the query touched. If tables is empty the
// entry is still cached under the cache key, but no tag-based invalidation
// will ever touch it (TTL remains the only invalidator).
type Loader func(ctx context.Context) (result map[string]interface{}, tables []string, err error)

// Options controls per-call overrides of the datasource's cache policy.
type Options struct {
	TTLSec    int    // >0 overrides QueryCacheConfig.DefaultTTLSec
	SkipCache bool   // entirely bypass cache for this call
	LockHot   bool   // override: if true, acquire distributed lock on miss
	QueryName string // myquery name for logging; falls back to sql when empty
}

const (
	lockAcquireTTL  = 5 * time.Second
	lockRetryBudget = 3
	lockRetrySleep  = 50 * time.Millisecond
)

var flightGroup singleflight.Group

// ServeOrLoad is the single entrypoint callers should use to run a cached
// read. It handles: cache bypass when disabled/skipped, cache serve on hit,
// in-pod singleflight dedup on miss, optional distributed lock for hot
// queries, async populate on success with volatile-table filtering, and
// per-call TTL override.
func ServeOrLoad(
	ctx context.Context,
	ds *module_model.DataSource,
	tenantId string,
	sql string,
	schemaPrefix string,
	loader Loader,
	opts Options,
) (map[string]interface{}, error) {

	if opts.SkipCache || !enabled(ds) {
		res, _, err := loader(ctx)
		if err == nil {
			globalCounters.bypass.Add(1)
		}
		return res, err
	}

	key := BuildKey(ds.ProjectId, tenantId, ds.DbAlias, sql)
	desc := opts.QueryName
	if desc == "" {
		desc = sql
	}

	// Fast path: direct cache hit.
	if hit, res, _ := lookupRaw(ctx, ds, key); hit {
		logs.WithContext(ctx).Info(fmt.Sprintf("cache hit for query %s", desc))
		return res, nil
	}

	// Miss: coalesce concurrent callers for the same key.
	v, err, _ := flightGroup.Do(key, func() (interface{}, error) {
		// Re-check the cache — another flight winner may have populated.
		if hit, res, _ := lookupRaw(ctx, ds, key); hit {
			logs.WithContext(ctx).Info(fmt.Sprintf("cache hit for query %s", desc))
			return res, nil
		}

		wantLock := opts.LockHot || ds.QueryCacheConfig.LockHotQueries
		var lockOwner string
		if wantLock {
			lockOwner = newLockOwner()
			if acquired, _ := ds.GetQueryCache().AcquireLock(ctx, key, lockAcquireTTL, lockOwner); !acquired {
				// Someone else is loading. Give them a bounded window to
				// populate; if nothing shows up, run the loader ourselves
				// rather than blocking the request indefinitely.
				for i := 0; i < lockRetryBudget; i++ {
					time.Sleep(lockRetrySleep)
					if hit, res, _ := lookupRaw(ctx, ds, key); hit {
						return res, nil
					}
				}
				lockOwner = "" // signal that we don't own anything to release
			}
		}

		res, tables, err := loader(ctx)
		if err == nil && res != nil {
			qualified := QualifyTables(tables, schemaPrefix)
			if !hasVolatileTable(qualified, ds.QueryCacheConfig.VolatileTables) {
				populateWithOverride(ctx, ds, key, res, tagsFromQualified(ds.ProjectId, ds.DbAlias, qualified), opts.TTLSec)
			} else {
				logs.WithContext(ctx).Debug("qlcache skipping populate for volatile tables")
				globalCounters.bypass.Add(1)
			}
		}

		if lockOwner != "" {
			_ = ds.GetQueryCache().ReleaseLock(ctx, key, lockOwner)
		}
		return res, err
	})
	if err != nil {
		return nil, err
	}
	res, _ := v.(map[string]interface{})
	return res, nil
}

func tagsFromQualified(projectId, dsAlias string, qualified []string) []string {
	if len(qualified) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(qualified))
	tags := make([]string, 0, len(qualified))
	for _, t := range qualified {
		tag := TableTag(projectId, dsAlias, t)
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

func hasVolatileTable(tables []string, volatile []string) bool {
	if len(volatile) == 0 || len(tables) == 0 {
		return false
	}
	vset := make(map[string]struct{}, len(volatile))
	for _, v := range volatile {
		vset[NormalizeTableName(v)] = struct{}{}
	}
	for _, t := range tables {
		if _, ok := vset[t]; ok {
			return true
		}
	}
	return false
}

func newLockOwner() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
