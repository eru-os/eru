module github.com/eru-os/eru/eru-utils

go 1.24

require (
	github.com/eru-os/eru/eru-logs v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-models v0.0.0-00010101000000-000000000000
	github.com/google/go-cmp v0.7.0
	github.com/jmoiron/sqlx v1.3.5
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0
)

require (
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lib/pq v1.10.4 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
)

replace (
	github.com/eru-os/eru/eru-logs => ../eru-logs
	github.com/eru-os/eru/eru-models => ../eru-models
)
