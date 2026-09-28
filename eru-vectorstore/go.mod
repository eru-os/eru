module github.com/eru-os/eru/eru-vectorstore

go 1.24

require (
	github.com/amikos-tech/chroma-go v0.1.4
	github.com/aws/aws-sdk-go-v2 v1.41.4
	github.com/aws/aws-sdk-go-v2/config v1.31.0
	github.com/aws/aws-sdk-go-v2/credentials v1.18.4
	github.com/aws/aws-sdk-go-v2/service/s3vectors v1.4.0
	github.com/aws/smithy-go v1.24.2
	github.com/eru-os/eru/eru-ai v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-logs v0.0.0-00010101000000-000000000000
	github.com/eru-os/eru/eru-utils v0.0.0-00010101000000-000000000000
	github.com/jmoiron/sqlx v1.3.5
)

require (
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/anthropics/anthropic-sdk-go v1.35.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.7 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.28.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.37.0 // indirect
	github.com/eru-os/eru/eru-db v0.0.0-00010101000000-000000000000 // indirect
	github.com/eru-os/eru/eru-models v0.0.0-00010101000000-000000000000 // indirect
	github.com/eru-os/eru/eru-scheduler v0.0.0-00010101000000-000000000000 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace (
	github.com/eru-os/eru/eru-ai => ../eru-ai
	github.com/eru-os/eru/eru-db => ../eru-db
	github.com/eru-os/eru/eru-logs => ../eru-logs
	github.com/eru-os/eru/eru-models => ../eru-models
	github.com/eru-os/eru/eru-scheduler => ../eru-scheduler
	github.com/eru-os/eru/eru-utils => ../eru-utils
)
