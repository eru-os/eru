module github.com/eru-os/eru/eru-rules

go 1.24

require (
	github.com/eru-os/eru/eru-logs v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-server v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-store v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-utils v0.0.0-00010101000000-000000000000
	github.com/gorilla/mux v1.8.0
)

require (
	github.com/jmoiron/sqlx v1.3.4 // indirect
	github.com/lib/pq v1.2.0 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/segmentio/ksuid v1.0.3 // indirect
)

replace (
	github.com/eru-os/eru/eru-logs => ../eru-logs
	github.com/eru-os/eru/eru-repos => ../eru-repos
	github.com/eru-os/eru/eru-server => ../eru-server
	github.com/eru-os/eru/eru-store => ../eru-store
	github.com/eru-os/eru/eru-utils => ../eru-utils
)
