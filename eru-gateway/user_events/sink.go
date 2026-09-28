package user_events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/eru-os/eru/eru-store/store"
	"github.com/jmoiron/sqlx"
)

type Sink interface {
	Write(ctx context.Context, events []UserEvent) (err error)
	Name() string
}

const eventColumnCount = 12

type dbSink struct {
	store     store.StoreI
	tableName string
}

func newDbSink(ctx context.Context, s store.StoreI, tableName string, autoCreate bool) (sink Sink, err error) {
	if s == nil {
		err = errors.New("user event logging requires STORE_TYPE=POSTGRES - no config store available")
		return
	}
	d := &dbSink{store: s, tableName: tableName}
	if _, err = d.conn(); err != nil {
		return nil, err
	}
	if autoCreate {
		if err = d.ensureTable(ctx); err != nil {
			return
		}
	}
	return d, nil
}

func (d *dbSink) Name() string {
	return fmt.Sprint("db:", d.tableName)
}

func (d *dbSink) conn() (db *sqlx.DB, err error) {
	db = d.store.GetConn()
	if db != nil {
		return
	}
	if err = d.store.CreateConn(); err != nil {
		return nil, fmt.Errorf("user event logging could not open a database connection : %w", err)
	}
	db = d.store.GetConn()
	if db == nil {
		return nil, errors.New("user event logging requires STORE_TYPE=POSTGRES - no database connection available")
	}
	return
}

func (d *dbSink) ensureTable(ctx context.Context) (err error) {
	db, err := d.conn()
	if err != nil {
		return
	}
	ddl := fmt.Sprintf(`create table if not exists %s (
		id bigserial primary key,
		request_id text,
		trace_id text,
		user_id text,
		host text,
		path text,
		method text,
		status int,
		duration_ms int,
		target_host text,
		client_ip text,
		headers jsonb,
		request_time timestamptz not null default now()
	)`, d.tableName)
	if _, err = db.ExecContext(ctx, ddl); err != nil {
		return
	}
	indexName := strings.ReplaceAll(d.tableName, ".", "_")
	if _, err = db.ExecContext(ctx, fmt.Sprintf(
		"create index if not exists %s_request_time_brin on %s using brin (request_time)", indexName, d.tableName)); err != nil {
		return
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"create index if not exists %s_user_id_time on %s (user_id, request_time desc)", indexName, d.tableName))
	return
}

func (d *dbSink) Write(ctx context.Context, events []UserEvent) (err error) {
	if len(events) == 0 {
		return
	}
	db, err := d.conn()
	if err != nil {
		return
	}
	query, vals := buildInsert(d.tableName, events)
	_, err = db.ExecContext(ctx, query, vals...)
	return
}

func buildInsert(tableName string, events []UserEvent) (query string, vals []interface{}) {
	placeholders := make([]string, 0, len(events))
	vals = make([]interface{}, 0, len(events)*eventColumnCount)
	for i, e := range events {
		base := i * eventColumnCount
		placeholders = append(placeholders, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d::jsonb,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12))

		headers := "{}"
		if len(e.Headers) > 0 {
			if headerBytes, jsonErr := json.Marshal(e.Headers); jsonErr == nil {
				headers = string(headerBytes)
			}
		}
		vals = append(vals, nullable(e.RequestId), nullable(e.TraceId), nullable(e.UserId), nullable(e.Host),
			nullable(e.Path), nullable(e.Method), e.Status, e.DurationMs, nullable(e.TargetHost),
			nullable(e.ClientIp), headers, e.RequestTime)
	}

	query = fmt.Sprintf(`insert into %s (request_id, trace_id, user_id, host, path, method, status, duration_ms, target_host, client_ip, headers, request_time) values %s`,
		tableName, strings.Join(placeholders, ","))
	return
}

func nullable(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
