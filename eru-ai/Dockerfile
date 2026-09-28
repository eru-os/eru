FROM golang:1.24.5-alpine3.22

WORKDIR /build

# source files copied to build folder
COPY . .
WORKDIR eru-ai

# Build eru-files
RUN GOOS=linux CGO_ENABLED=0 go build -a -ldflags '-s -w -extldflags "-static"' -o app .

FROM alpine:3.22.0

WORKDIR /build/eru-ai/app
COPY --from=0 /build/eru-ai/app .
RUN mkdir -p /app/config
CMD ["./app", "run"]

