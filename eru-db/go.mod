module github.com/eru-os/eru/eru-db

go 1.24

require (
	github.com/eru-os/eru/eru-logs v0.0.0-00010101000000-000000000000
	github.com/jmoiron/sqlx v1.3.5
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/lib/pq v1.10.4 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
)

replace github.com/eru-os/eru/eru-logs => ../eru-logs
