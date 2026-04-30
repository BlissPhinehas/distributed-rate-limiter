# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o ratelimiter ./server

# Run stage — minimal image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/ratelimiter .

EXPOSE 50051

CMD ["./ratelimiter"]