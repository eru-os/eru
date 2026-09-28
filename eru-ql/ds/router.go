package ds

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync/atomic"
	"syscall"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-ql/module_model"
	mysqldrv "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type RouterHint struct {
	UseWriter bool
}

type useWriterCtxKey struct{}

// WithUseWriter returns a context that forces SELECTs to the main writer.
func WithUseWriter(ctx context.Context, useWriter bool) context.Context {
	if !useWriter {
		return ctx
	}
	return context.WithValue(ctx, useWriterCtxKey{}, true)
}

func hintFromCtx(ctx context.Context) RouterHint {
	if v, ok := ctx.Value(useWriterCtxKey{}).(bool); ok && v {
		return RouterHint{UseWriter: true}
	}
	return RouterHint{}
}

const (
	StrategyRoundRobin = "round_robin"
	StrategyWeighted   = "weighted"
	StrategyRandom     = "random"
	StrategyMainOnly   = "main_only"
)

func WriteCon(ctx context.Context, ds *module_model.DataSource) (*sqlx.DB, error) {
	if ds == nil || ds.Con == nil {
		return nil, errors.New("write connection not available")
	}
	return ds.Con, nil
}

// ReadCon picks a connection for a SELECT per ds.ReadPolicy.
// The returned *ReadDbConfig is non-nil only when a replica was chosen — pass it to
// MarkReadFailed on query error to flip that replica to unhealthy.
func ReadCon(ctx context.Context, ds *module_model.DataSource, hint RouterHint) (*sqlx.DB, *module_model.ReadDbConfig, error) {
	if ds == nil {
		return nil, nil, errors.New("nil datasource")
	}
	if !hint.UseWriter {
		hint = hintFromCtx(ctx)
	}
	policy := ds.ReadPolicy
	if hint.UseWriter || policy.Disabled || len(ds.ReadDbConfigs) == 0 || policy.Strategy == "" || policy.Strategy == StrategyMainOnly {
		if ds.Con == nil {
			return nil, nil, errors.New("main connection not available")
		}
		return ds.Con, nil, nil
	}

	type candidate struct {
		db      *sqlx.DB
		replica *module_model.ReadDbConfig
		weight  int
	}

	var sm SqlMakerI
	var candidates []candidate
	for i, replica := range ds.ReadDbConfigs {
		if replica.Disabled {
			continue
		}
		if !replica.ConStatus || replica.Con == nil {
			if sm == nil {
				sm = GetSqlMaker(ds.DbName)
			}
			if sm == nil {
				continue
			}
			if rerr := sm.ConnectReadReplica(ctx, ds, i); rerr != nil {
				logs.WithContext(ctx).Debug("read replica reconnect skipped: " + rerr.Error())
				continue
			}
		}
		w := replica.Weight
		if w <= 0 {
			w = 1
		}
		candidates = append(candidates, candidate{db: replica.Con, replica: replica, weight: w})
	}

	if policy.IncludeMainInReads && ds.Con != nil {
		w := policy.MainWeight
		if w <= 0 {
			w = 1
		}
		candidates = append(candidates, candidate{db: ds.Con, replica: nil, weight: w})
	}

	if len(candidates) == 0 {
		if policy.FailoverToMain && ds.Con != nil {
			return ds.Con, nil, nil
		}
		return nil, nil, errors.New("no read connections available")
	}

	var pick candidate
	switch policy.Strategy {
	case StrategyRandom:
		pick = candidates[rand.Intn(len(candidates))]
	case StrategyWeighted:
		total := 0
		for _, c := range candidates {
			total += c.weight
		}
		seq := atomic.AddUint64(&ds.ReadCounter, 1) - 1
		idx := int(seq % uint64(total))
		cum := 0
		pick = candidates[len(candidates)-1]
		for _, c := range candidates {
			cum += c.weight
			if idx < cum {
				pick = c
				break
			}
		}
	default: // StrategyRoundRobin and any unknown value
		seq := atomic.AddUint64(&ds.ReadCounter, 1) - 1
		pick = candidates[int(seq%uint64(len(candidates)))]
	}
	if pick.replica != nil {
		logs.WithContext(ctx).Info(fmt.Sprint("replica ", pick.replica.Name, " used"))
	} else {
		logs.WithContext(ctx).Info("main connection used")
	}
	return pick.db, pick.replica, nil
}

// MarkReadFailed flips a replica to unhealthy only when err is connection-class, so subsequent
// picks skip it (and trigger a reconnect attempt). SQL/permission/constraint errors leave the
// pool intact. No-op when replica is nil (main writer) or err is not connection-related.
func MarkReadFailed(ctx context.Context, replica *module_model.ReadDbConfig, err error) {
	if replica == nil || !isConnError(err) {
		return
	}
	logs.WithContext(ctx).Warn(fmt.Sprint("read replica ", replica.Name, " marked unhealthy: ", err.Error()))
	replica.ConStatus = false
	if replica.Con != nil {
		_ = replica.Con.Close()
		replica.Con = nil
	}
}

// isConnError reports whether err is a connection-class failure that should drop the pool.
// Returns false for SQL syntax, permission, constraint, and other query-level errors.
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		c := string(pqErr.Code)
		// SQLSTATE class 08 = connection exception; 57P01/02/03 = admin shutdown / crash / cannot connect now.
		if strings.HasPrefix(c, "08") || c == "57P01" || c == "57P02" || c == "57P03" {
			return true
		}
	}
	var myErr *mysqldrv.MySQLError
	if errors.As(err, &myErr) {
		// 2006 server has gone away, 2013 lost connection during query.
		if myErr.Number == 2006 || myErr.Number == 2013 {
			return true
		}
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "i/o timeout")
}
