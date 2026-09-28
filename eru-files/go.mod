module github.com/eru-os/eru/eru-files

go 1.24

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.17.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.8.2
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.6.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.0
	github.com/aws/aws-sdk-go-v2 v1.41.4
	github.com/aws/aws-sdk-go-v2/config v1.31.0
	github.com/aws/aws-sdk-go-v2/credentials v1.18.4
	github.com/aws/aws-sdk-go-v2/service/s3 v1.97.1
	github.com/aws/smithy-go v1.24.2
	github.com/eru-os/eru/eru-crypto v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-events v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-logs v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-read-write v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-repos v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-secret-manager v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-server v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-store v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-utils v0.0.0-00010101000000-000000000000
	github.com/google/go-cmp v0.7.0
	github.com/gorilla/mux v1.8.0
	github.com/segmentio/ksuid v1.0.3
	github.com/tidwall/gjson v1.18.0
)

require (
	cloud.google.com/go v0.121.3 // indirect
	cloud.google.com/go/auth v0.16.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	cloud.google.com/go/iam v1.5.2 // indirect
	cloud.google.com/go/kms v1.22.0 // indirect
	cloud.google.com/go/pubsub v1.49.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.3.3 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.7 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.31.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.28.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.36.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.32.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.28.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.37.0 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/eru-os/eru/eru-cache v0.0.0-00010101000000-000000000000 // indirect
	github.com/eru-os/eru/eru-db v0.0.0-00010101000000-000000000000 // indirect
	github.com/eru-os/eru/eru-models v0.0.0-00010101000000-000000000000 // indirect
	github.com/eru-os/eru/eru-scheduler v0.0.0-00010101000000-000000000000 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.2 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/redis/go-redis/v9 v9.11.0 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tiendc/go-deepcopy v1.6.0 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/excelize/v2 v2.9.1 // indirect
	github.com/xuri/nfp v0.0.1 // indirect
	go.etcd.io/etcd/api/v3 v3.6.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.4 // indirect
	go.etcd.io/etcd/client/v3 v3.6.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.21.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.40.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/api v0.239.0 // indirect
	google.golang.org/genproto v0.0.0-20250505200425-f936aa4a68b2 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250721164621-a45f3dfb1074 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250728155136-f173205681a0 // indirect
	google.golang.org/grpc v1.74.2 // indirect
	google.golang.org/protobuf v1.36.7 // indirect
)

require (
	github.com/gabriel-vasile/mimetype v1.4.11
	github.com/gobwas/glob v0.2.3
	github.com/jmoiron/sqlx v1.3.5 // indirect
	github.com/lib/pq v1.10.4 // indirect
	github.com/rs/cors v1.7.0 // indirect
	golang.org/x/net v0.42.0 // indirect
)

replace (
	github.com/eru-os/eru/eru-cache => ../eru-cache
	github.com/eru-os/eru/eru-crypto => ../eru-crypto
	github.com/eru-os/eru/eru-db => ../eru-db
	github.com/eru-os/eru/eru-events => ../eru-events
	github.com/eru-os/eru/eru-logs => ../eru-logs
	github.com/eru-os/eru/eru-models => ../eru-models
	github.com/eru-os/eru/eru-read-write => ../eru-read-write
	github.com/eru-os/eru/eru-repos => ../eru-repos
	github.com/eru-os/eru/eru-scheduler => ../eru-scheduler
	github.com/eru-os/eru/eru-secret-manager => ../eru-secret-manager
	github.com/eru-os/eru/eru-server => ../eru-server
	github.com/eru-os/eru/eru-store => ../eru-store
	github.com/eru-os/eru/eru-utils => ../eru-utils
)
