# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web-build /web/dist ./web/dist
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN set -eux; \
    GOOS="${TARGETOS:-$(go env GOOS)}"; \
    GOARCH="${TARGETARCH:-$(go env GOARCH)}"; \
    GOARM=""; \
    if [ "$GOARCH" = "arm" ]; then \
      if [ -n "${TARGETVARIANT:-}" ]; then GOARM="${TARGETVARIANT#v}"; else GOARM="$(go env GOARM)"; fi; \
    fi; \
    export CGO_ENABLED=0 GOOS GOARCH; \
    if [ -n "$GOARM" ]; then export GOARM; fi; \
    go build -o /out/rhythmiq ./cmd/server

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app && apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go-build /out/rhythmiq /app/rhythmiq
COPY --from=web-build /web/dist /app/web/dist
RUN mkdir -p /data && chown -R app:app /app /data
USER app
EXPOSE 8080
ENV RHYTHMIQ_ADDR=:8080
ENV RHYTHMIQ_DB_PATH=/data/rhythmiq.db
ENTRYPOINT ["/app/rhythmiq"]
