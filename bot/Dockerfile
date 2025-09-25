FROM golang:1.24-bullseye as builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-s -w" -o app ./cmd/main.go

FROM alpine:3.18
WORKDIR /
COPY --from=builder /build/app .
ENTRYPOINT ["/app"]