# Build stage
FROM golang:1.26.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/long cmd/long/main.go

# Run stage
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/long /app/long
COPY env.yaml /app/env.yaml
# Ensure we have CA certificates for TLS (common in Go apps)
RUN apk --no-cache add ca-certificates
EXPOSE 9001

CMD ["/app/long"]