# eru

eru is a set of Go microservices for building data-driven applications and AI agents: a query
engine, a function and workflow runner, file storage, authentication with a built-in OAuth 2.0
server, an API gateway, and an agent runtime that uses the other services as its tools.

## Services

| Service | Default port | Port variable | What it does |
|---|---|---|---|
| `eru-ai` | 8088 | `ERUAIPORT` | Agent runtime: reasoning, reflex and orchestrator agents, tools, conversation memory |
| `eru-auth` | 8085 | `ERUAUTHPORT` | Users, sessions and an OAuth 2.0 authorization server |
| `eru-functions` | 8083 | `ERUFUNCTIONSPORT` | Function groups and workflows, scheduling, events |
| `eru-ql` | 8087 | `ERUQLPORT` | SQL and GraphQL queries over configured data sources |
| `eru-files` | 8082 | `ERUFILESPORT` | File storage on AWS S3, Azure Blob, GCP Storage, Google Drive and OneDrive |
| `eru-gateway` | 8086 | `ERUGATEWAYPORT` | API gateway and request routing |
| `eru-rules` | 8084 | `ERURULESPORT` | Data type definitions for rules (early stage) |
| `eru-html-image` | 8089 | `ERUHTMLIMAGEPORT` | Renders HTML to images |

Shared libraries used by the services: `eru-server`, `eru-store`, `eru-logs`, `eru-crypto`,
`eru-utils`, `eru-models`, `eru-db`, `eru-cache`, `eru-events`, `eru-repos`, `eru-scheduler`,
`eru-secret-manager`, `eru-security-rule`, `eru-templates`, `eru-read-write`, `eru-embeddings`
and `eru-vectorstore`.

`eru-ai` works with Anthropic, OpenAI, Google Gemini and AWS Bedrock models. Embeddings come from
Anthropic, OpenAI, AWS Bedrock or Hugging Face, and vector stores from pgvector, Pinecone, ChromaDB
or S3 Vectors.

## Building

Each directory is its own Go module (Go 1.24). The `go.work` file at the root ties them together,
so a service builds against the libraries in this checkout.

```bash
cd eru-functions
go build -o app .
```

Docker images are built from the repository root, because a service needs the shared libraries
next to it:

```bash
docker build -f eru-functions/Dockerfile -t eru-functions .
```

## Running

```bash
cd eru-functions
go run .
```

Configuration comes from environment variables:

| Variable | Purpose |
|---|---|
| `STORE_TYPE` | `STANDALONE` (default, file based) or `POSTGRES` |
| `STORE_DB_PATH` | Postgres connection string when `STORE_TYPE=POSTGRES`; may contain `ENV_STORE_DB_USER` and `ENV_STORE_DB_PASSWORD` placeholders |
| `STORE_DB_USER`, `STORE_DB_PASSWORD` | Values substituted into `STORE_DB_PATH` |
| `ERU<SERVICE>PORT` | Port a service listens on (see the table above) |
| `ALLOWED_ORIGINS` | CORS origins |
| `LOG_LEVEL` | Log level |
| `TRACE_URL` | OpenTelemetry collector for distributed tracing; tracing is off when unset |

## Authentication

`eru-auth` includes an OAuth 2.0 authorization server with dynamic client registration, PKCE and
discovery metadata. Set an auth's `oauth_server.backend` to `ERU` (the default in this repository)
to have eru-auth issue and sign tokens itself.

The direct login, registration, token refresh and logout APIs rely on an external token backend,
which this repository does not include; without one they return an error.

## Extending

Tools, agent types and auth providers register themselves from `init()` through
`tools.RegisterTool`, `agents.RegisterAgentType`, `agentspec.RegisterType` and
`auth.RegisterAuth`. To add one, write a package that registers in `init()` and import it from the
service's `main.go`.

## License

Apache License 2.0 - see [LICENSE](LICENSE).
