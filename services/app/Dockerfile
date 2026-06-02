FROM golang:1.24.2-alpine3.21 AS builder
WORKDIR /builder
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd
    
FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/app /app/app
COPY --from=builder /builder/internal/adapter/postgres/migrations /app/internal/adapter/postgres/migrations
COPY --from=builder /builder/configs /app/configs
ENTRYPOINT ["/app/app"]
