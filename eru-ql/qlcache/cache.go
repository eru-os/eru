package qlcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
)

const (
	keyPrefix   = "eruql:qc:"
	populateTtl = 10 * time.Second // context timeout for the async write
)

type counters struct {
	hits   atomic.Int64
	misses atomic.Int64
	bypass atomic.Int64
	errors atomic.Int64
	writes atomic.Int64
}

var globalCounters counters

type Stats struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
	Bypass int64 `json:"bypass"`
	Errors int64 `json:"errors"`
	Writes int64 `json:"writes"`
}

func GetStats() Stats {
	return Stats{
		Hits:   globalCounters.hits.Load(),
		Misses: globalCounters.misses.Load(),
		Bypass: globalCounters.bypass.Load(),
		Errors: globalCounters.errors.Load(),
		Writes: globalCounters.writes.Load(),
	}
}

// NormalizeSQL trims and collapses whitespace. It does not lowercase because
// quoted identifiers in PostgreSQL are case-sensitive.
func NormalizeSQL(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))
	prevSpace := true
	for _, r := range sql {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// BuildKey constructs the canonical cache key for a query. projectId and
// dsAlias are folded into both the hash and the key prefix so two projects
// that happen to share a datasource alias (and the same SQL) never collide.
// When tenantId is non-empty the key is additionally tenant-scoped so the same
// query executed for different tenants (or at project level) never shares a
// cache entry. A project-level execution (tenantId == "") produces the same
// key as before, keeping existing entries valid.
func BuildKey(projectId, tenantId, dsAlias, finalSQL string) string {
	if tenantId == "" {
		h := sha256.Sum256([]byte(projectId + "::" + dsAlias + "::" + NormalizeSQL(finalSQL)))
		return keyPrefix + projectId + ":" + dsAlias + ":" + hex.EncodeToString(h[:])
	}
	h := sha256.Sum256([]byte(projectId + "::" + tenantId + "::" + dsAlias + "::" + NormalizeSQL(finalSQL)))
	return keyPrefix + projectId + ":" + tenantId + ":" + dsAlias + ":" + hex.EncodeToString(h[:])
}

// enabled returns the active cache store if caching is configured and enabled
// for this datasource. A nil return means the caller should bypass.
func enabled(ds *module_model.DataSource) bool {
	if ds == nil {
		return false
	}
	if !ds.QueryCacheConfig.Enabled {
		return false
	}
	if ds.GetQueryCache() == nil {
		return false
	}
	return true
}

// Serve checks the cache for a matching result. On hit it deserializes and
// returns the cached map. On miss/bypass/error it returns hit=false; the
// caller then runs the DB query as usual. Never panics, never errors out.
func Serve(ctx context.Context, ds *module_model.DataSource, tenantId string, finalSQL string) (hit bool, res map[string]interface{}, key string) {
	if !enabled(ds) {
		globalCounters.bypass.Add(1)
		return false, nil, ""
	}
	key = BuildKey(ds.ProjectId, tenantId, ds.DbAlias, finalSQL)
	return lookupRaw(ctx, ds, key)
}

// lookupRaw performs the cache GET given a precomputed key. It exists as a
// helper for ServeOrLoad's double-check pattern which already has the key in
// hand.
func lookupRaw(ctx context.Context, ds *module_model.DataSource, key string) (hit bool, res map[string]interface{}, outKey string) {
	if !breakerAllow() {
		globalCounters.bypass.Add(1)
		return false, nil, key
	}
	store := ds.GetQueryCache()
	raw, err := store.Get(ctx, key)
	if err != nil {
		if isMissErr(err) {
			breakerRecordSuccess()
			globalCounters.misses.Add(1)
			return false, nil, key
		}
		breakerRecordFailure()
		globalCounters.errors.Add(1)
		logs.WithContext(ctx).Debug("qlcache get error: " + err.Error())
		return false, nil, key
	}
	breakerRecordSuccess()
	res = map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		globalCounters.errors.Add(1)
		logs.WithContext(ctx).Debug("qlcache unmarshal error: " + err.Error())
		_ = store.Delete(ctx, key)
		return false, nil, key
	}
	globalCounters.hits.Add(1)
	return true, res, key
}

// Populate stores the result under key with the datasource's default TTL and
// the provided tags. Runs in a detached goroutine so the request path is
// never blocked by serialization or Redis I/O.
//
// Tags convention: callers should pass fully-qualified table names with the
// datasource alias prefixed, e.g. "primary::public.orders". That lets Phase 4
// mutation invalidation match reads to writes without cross-datasource bleed.
func Populate(ctx context.Context, ds *module_model.DataSource, key string, result map[string]interface{}, tags []string) {
	populateWithOverride(ctx, ds, key, result, tags, 0)
}

func populateWithOverride(ctx context.Context, ds *module_model.DataSource, key string, result map[string]interface{}, tags []string, ttlSecOverride int) {
	if !enabled(ds) || key == "" {
		return
	}
	maxBytes := ds.QueryCacheConfig.MaxValueBytes
	ttlSec := ds.QueryCacheConfig.DefaultTTLSec
	if ttlSecOverride > 0 {
		ttlSec = ttlSecOverride
	}
	store := ds.GetQueryCache()

	// Snapshot what we need; detach from the request context so a client
	// disconnect does not cancel the cache write.
	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), populateTtl)
		defer cancel()

		payload, err := json.Marshal(result)
		if err != nil {
			globalCounters.errors.Add(1)
			logs.WithContext(writeCtx).Debug("qlcache marshal error: " + err.Error())
			return
		}
		if maxBytes > 0 && len(payload) > maxBytes {
			// Too big to cache; increment a counter so this is visible.
			globalCounters.bypass.Add(1)
			logs.WithContext(writeCtx).Info("qlcache skipping oversize payload")
			return
		}
		var ttl time.Duration
		if ttlSec > 0 {
			ttl = time.Duration(ttlSec) * time.Second
		}
		if !breakerAllow() {
			globalCounters.bypass.Add(1)
			return
		}
		if err := store.SetWithTagsTTL(writeCtx, key, json.RawMessage(payload), ttl, tags); err != nil {
			breakerRecordFailure()
			globalCounters.errors.Add(1)
			logs.WithContext(writeCtx).Debug("qlcache set error: " + err.Error())
			return
		}
		breakerRecordSuccess()
		globalCounters.writes.Add(1)
	}()
}

// TableTag builds a canonical tag string scoped by projectId and datasource.
// Use DefaultSchemaName-prefixed table names so "orders" and "public.orders"
// don't produce different tags.
func TableTag(projectId, dsAlias, fqTable string) string {
	return projectId + "::" + dsAlias + "::" + NormalizeTableName(fqTable)
}

// NormalizeTableName folds unquoted identifiers to lower case the same way
// PostgreSQL resolves them, so a DML on "CRM_accounts" and a select on
// "crm_accounts" produce the same tag. Quoted segments keep their case because
// those are genuinely distinct tables.
func NormalizeTableName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if !strings.Contains(name, `"`) {
		return strings.ToLower(name)
	}
	segments := strings.Split(name, ".")
	for i, segment := range segments {
		if !strings.Contains(segment, `"`) {
			segments[i] = strings.ToLower(segment)
		}
	}
	return strings.Join(segments, ".")
}

func isMissErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "redis: nil") ||
		strings.EqualFold(msg, "nil")
}
