package main

import (
    "context"
    "crypto/tls"
    "fmt"
    "log"
    "net"
    "os"

    "github.com/BlissPhinehas/distributed-rate-limiter/server/algorithm"
    pb "github.com/BlissPhinehas/distributed-rate-limiter/proto"
    "github.com/redis/go-redis/v9"
    "google.golang.org/grpc"
    "google.golang.org/grpc/reflection"
)

// server implements the RateLimiterServer interface generated from our proto
type server struct {
	pb.UnimplementedRateLimiterServer
	tokenBucket   *algorithm.TokenBucket
	slidingWindow *algorithm.SlidingWindow
}

func (s *server) CheckTokenBucket(ctx context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	allowed, remaining, retryAfterMs, err := s.tokenBucket.Allow(
		ctx,
		req.ClientId,
		req.Capacity,
		req.Rate,
	)
	if err != nil {
		return nil, fmt.Errorf("token bucket error: %w", err)
	}

	return &pb.RateLimitResponse{
		Allowed:        allowed,
		Remaining:      remaining,
		RetryAfterMs:   retryAfterMs,
		Algorithm:      "token_bucket",
	}, nil
}

func (s *server) CheckSlidingWindow(ctx context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	allowed, remaining, retryAfterMs, err := s.slidingWindow.Allow(
		ctx,
		req.ClientId,
		req.Capacity,
		req.WindowMs,
	)
	if err != nil {
		return nil, fmt.Errorf("sliding window error: %w", err)
	}

	return &pb.RateLimitResponse{
		Allowed:        allowed,
		Remaining:      remaining,
		RetryAfterMs:   retryAfterMs,
		Algorithm:      "sliding_window",
	}, nil
}

func main() {
	// Redis connection — reads from env so Docker and Cloud Run can inject it
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	goredisPassword := os.Getenv("REDIS_PASSWORD")

	opts := &redis.Options{
		Addr:     redisAddr,
		Password: goredisPassword,
	}

	if goredisPassword != "" {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		opts.TLSConfig = tlsCfg
	}

	rdb := redis.NewClient(opts)

	// Verify Redis is reachable before we start accepting traffic
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("could not connect to Redis at %s: %v", redisAddr, err)
	}
	log.Printf("connected to Redis at %s", redisAddr)

	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRateLimiterServer(grpcServer, &server{
		tokenBucket:   algorithm.NewTokenBucket(rdb),
		slidingWindow: algorithm.NewSlidingWindow(rdb),
	})

	// reflection lets tools like grpcurl introspect the API without the proto file
	reflection.Register(grpcServer)

	log.Printf("rate limiter gRPC server listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}