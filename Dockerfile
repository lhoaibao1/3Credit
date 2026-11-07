# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG GIT_SHA=dev
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath -ldflags="-s -w -X main.version=${GIT_SHA}" -o /out/3credit ./cmd/server

# ---- run ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -H -u 10001 app \
 && mkdir -p /data && chown app:app /data
ENV TZ=Asia/Ho_Chi_Minh \
    DATA_DIR=/data \
    ADDR=:8080 \
    APP_ENV=production
COPY --from=build /out/3credit /usr/local/bin/3credit
USER app
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/3credit"]
