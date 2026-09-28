FROM golang:1.24-alpine3.21

WORKDIR /build

# source files copied to build folder
COPY . .
WORKDIR eru-html-image

# Build eru-html-image
RUN GOOS=linux CGO_ENABLED=0 go build -a -ldflags '-s -w -extldflags "-static"' -o app .

FROM alpine:3.21

# chromium and the fonts it needs to rasterize the html output
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    font-noto \
    font-noto-cjk \
    tzdata \
 && (test -x /usr/bin/chromium || ln -s /usr/bin/chromium-browser /usr/bin/chromium)

ENV CHROMIUM_PATH=/usr/bin/chromium
ENV ERUHTMLIMAGEPORT=8089

WORKDIR /build/eru-html-image/app
COPY --from=0 /build/eru-html-image/app .
RUN mkdir -p /app/config
EXPOSE 8089
CMD ["./app", "run"]
