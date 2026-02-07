FROM golang:1.24.2 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/app ./cmd
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /out/app /app/app
COPY --from=builder /src/configs /app/configs
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/app"]
