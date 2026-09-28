FROM golang:1.24.5-alpine3.22

WORKDIR /build

# source files copied to build folder
COPY . .
WORKDIR eru-functions

# Build eru-functions with CGO disabled
# Note: This build will not include Kafka functionality due to CGO requirements
# Use DB event type for async processing when Kafka is not available
RUN GOOS=linux CGO_ENABLED=0 go build -a -ldflags '-s -w -extldflags "-static"' -o app .

FROM alpine:3.22.0

WORKDIR /build/eru-functions/app
COPY --from=0 /build/eru-functions/app .
RUN mkdir -p /app/config
CMD ["./app", "run"]

