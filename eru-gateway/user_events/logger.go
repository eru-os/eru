package user_events

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-store/store"
)

const (
	defaultTableName      = "erugateway_user_events"
	defaultBatchSize      = 200
	defaultFlushInterval  = 250 * time.Millisecond
	defaultBufferSize     = 2000
	defaultBufferBytes    = 4 * 1024 * 1024
	defaultAllowedHeaders = "user-agent,content-type,content-length,referer,origin,accept,accept-language,x-forwarded-for,x-real-ip,x-request-id,x-correlation-id"
	defaultExcludedPaths  = "/hello,/state,/echo,/env,/registry,/health,/healthz,/ready,/live"
	flushTimeout          = 10 * time.Second
	closeTimeout          = 15 * time.Second
	flushRetryDelay       = 200 * time.Millisecond
)

type Config struct {
	BatchSize      int
	FlushInterval  time.Duration
	BufferSize     int
	MaxBufferBytes int64
	AllowedHeaders []string
	ExcludedPaths  []string
}

func ConfigFromEnv() Config {
	return Config{
		BatchSize:      envInt("USER_EVENT_BATCH_SIZE", defaultBatchSize),
		FlushInterval:  envDuration("USER_EVENT_FLUSH_INTERVAL", defaultFlushInterval),
		BufferSize:     envInt("USER_EVENT_BUFFER_SIZE", defaultBufferSize),
		MaxBufferBytes: int64(envInt("USER_EVENT_BUFFER_BYTES", defaultBufferBytes)),
		AllowedHeaders: envList("USER_EVENT_HEADERS", defaultAllowedHeaders),
		ExcludedPaths:  envList("USER_EVENT_EXCLUDE_PATHS", defaultExcludedPaths),
	}
}

type Logger struct {
	sink           Sink
	events         chan UserEvent
	done           chan struct{}
	wg             sync.WaitGroup
	closeOnce      sync.Once
	batchSize      int
	flushInterval  time.Duration
	maxBufferBytes int64
	bufferedBytes  atomic.Int64
	dropped        atomic.Uint64
	written        atomic.Uint64
	failed         atomic.Uint64
	allowedHeaders []string
	excludedPaths  []string
}

func New(ctx context.Context, s store.StoreI) (logger *Logger, err error) {
	if !envBool("USER_EVENT_LOG", false) {
		logs.WithContext(ctx).Info("user event logging is disabled - set USER_EVENT_LOG=true to enable")
		return nil, nil
	}

	tableName := envString("USER_EVENT_TABLE", defaultTableName)
	sink, err := newDbSink(ctx, s, tableName, envBool("USER_EVENT_AUTO_CREATE", true))
	if err != nil {
		logs.WithContext(ctx).Error(fmt.Sprint("failed to initialise user event sink : ", err.Error()))
		return nil, err
	}

	logger = newLogger(sink, ConfigFromEnv())

	logs.WithContext(ctx).Info(fmt.Sprint("user event logging enabled - sink=", sink.Name(),
		" batch_size=", logger.batchSize, " flush_interval=", logger.flushInterval,
		" buffer_size=", cap(logger.events), " buffer_bytes=", logger.maxBufferBytes))
	return logger, nil
}

func newLogger(sink Sink, cfg Config) *Logger {
	logger := &Logger{
		sink:           sink,
		events:         make(chan UserEvent, cfg.BufferSize),
		done:           make(chan struct{}),
		batchSize:      cfg.BatchSize,
		flushInterval:  cfg.FlushInterval,
		maxBufferBytes: cfg.MaxBufferBytes,
		allowedHeaders: cfg.AllowedHeaders,
		excludedPaths:  cfg.ExcludedPaths,
	}
	logger.wg.Add(1)
	go logger.run()
	return logger
}

func (l *Logger) Enabled() bool {
	return l != nil
}

func (l *Logger) ShouldLog(r *http.Request) bool {
	if l == nil {
		return false
	}
	path := r.URL.Path
	for _, excluded := range l.excludedPaths {
		if excluded != "" && strings.HasPrefix(path, excluded) {
			return false
		}
	}
	return true
}

func (l *Logger) NewEvent(r *http.Request, w http.ResponseWriter, at time.Time) UserEvent {
	if l == nil {
		return UserEvent{}
	}
	return NewEvent(r, w, at, l.allowedHeaders)
}

func (l *Logger) Log(e UserEvent) {
	if l == nil {
		return
	}
	if l.bufferedBytes.Load()+int64(e.sizeBytes) > l.maxBufferBytes {
		l.dropped.Add(1)
		return
	}
	select {
	case l.events <- e:
		l.bufferedBytes.Add(int64(e.sizeBytes))
	default:
		l.dropped.Add(1)
	}
}

func (l *Logger) run() {
	defer l.wg.Done()
	batch := make([]UserEvent, 0, l.batchSize)

	defer func() {
		if r := recover(); r != nil {
			logs.Logger.Error(fmt.Sprint("user event flusher panic: ", r, " : ", string(debug.Stack())))
			l.writeBatch(batch)
		}
	}()

	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case e := <-l.events:
			l.bufferedBytes.Add(int64(-e.sizeBytes))
			batch = append(batch, e)
			if len(batch) >= l.batchSize {
				l.writeBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				l.writeBatch(batch)
				batch = batch[:0]
			}
			l.reportDropped()
		case <-l.done:
			batch = l.drain(batch)
			l.writeBatch(batch)
			l.reportDropped()
			return
		}
	}
}

func (l *Logger) drain(batch []UserEvent) []UserEvent {
	for {
		select {
		case e := <-l.events:
			l.bufferedBytes.Add(int64(-e.sizeBytes))
			batch = append(batch, e)
			if len(batch) >= l.batchSize {
				l.writeBatch(batch)
				batch = batch[:0]
			}
		default:
			return batch
		}
	}
}

func (l *Logger) writeBatch(batch []UserEvent) {
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()

	err := l.sink.Write(ctx, batch)
	if err != nil {
		logs.Logger.Error(fmt.Sprint("user event flush failed, retrying : ", err.Error()))
		time.Sleep(flushRetryDelay)
		err = l.sink.Write(ctx, batch)
	}
	if err != nil {
		l.failed.Add(uint64(len(batch)))
		logs.Logger.Error(fmt.Sprint("user event flush failed, dropping ", len(batch), " events : ", err.Error()))
		return
	}
	l.written.Add(uint64(len(batch)))
}

func (l *Logger) reportDropped() {
	if dropped := l.dropped.Swap(0); dropped > 0 {
		logs.Logger.Warn(fmt.Sprint("user event buffer full - dropped ", dropped, " events"))
	}
}

func (l *Logger) Close(ctx context.Context) {
	if l == nil {
		return
	}
	l.closeOnce.Do(func() {
		close(l.done)
		finished := make(chan struct{})
		go func() {
			l.wg.Wait()
			close(finished)
		}()
		select {
		case <-finished:
			logs.Logger.Info(fmt.Sprint("user event logger stopped - written=", l.written.Load(), " failed=", l.failed.Load()))
		case <-time.After(closeTimeout):
			logs.Logger.Error("user event logger shutdown timed out - buffered events may be lost")
		}
	})
}

func envString(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envList(key string, fallback string) []string {
	raw := envString(key, fallback)
	parts := strings.Split(raw, ",")
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
}
