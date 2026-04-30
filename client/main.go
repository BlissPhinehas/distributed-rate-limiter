package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/BlissPhinehas/distributed-rate-limiter/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// flags
	addr       := flag.String("addr",      "localhost:50051", "gRPC server address")
	algo       := flag.String("algo",      "token",           "algorithm: token or sliding")
	clientID   := flag.String("client",    "test-client",     "client identifier")
	capacity   := flag.Int("capacity",     10,                "max tokens / max requests in window")
	rate       := flag.Int("rate",         2,                 "tokens per second (token bucket only)")
	windowMs   := flag.Int64("window",     5000,              "window size in ms (sliding window only)")
	requests   := flag.Int("requests",     20,                "total requests to send")
	concurrent := flag.Bool("concurrent",  false,             "send all requests concurrently (benchmark mode)")
	flag.Parse()

	// connect
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not connect to %s: %v", *addr, err)
	}
	defer conn.Close()

	client := pb.NewRateLimiterClient(conn)

	fmt.Printf("\n=== Rate Limiter CLI ===\n")
	fmt.Printf("Algorithm : %s\n", *algo)
	fmt.Printf("Client ID : %s\n", *clientID)
	fmt.Printf("Requests  : %d (concurrent=%v)\n\n", *requests, *concurrent)

	if *concurrent {
		runConcurrent(client, *algo, *clientID, int32(*capacity), int32(*rate), *windowMs, *requests)
	} else {
		runSequential(client, *algo, *clientID, int32(*capacity), int32(*rate), *windowMs, *requests)
	}
}

func check(client pb.RateLimiterClient, algo, clientID string, capacity, rate int32, windowMs int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := &pb.RateLimitRequest{
		ClientId: clientID,
		Capacity: capacity,
		Rate:     rate,
		WindowMs: windowMs,
	}

	var resp *pb.RateLimitResponse
	var err error

	if algo == "sliding" {
		resp, err = client.CheckSlidingWindow(ctx, req)
	} else {
		resp, err = client.CheckTokenBucket(ctx, req)
	}

	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}

	status := "✅ ALLOWED"
	if !resp.Allowed {
		status = fmt.Sprintf("❌ DENIED  (retry after %dms)", resp.RetryAfterMs)
	}
	fmt.Printf("  %s | remaining: %2d | algo: %s\n", status, resp.Remaining, resp.Algorithm)
}

func runSequential(client pb.RateLimiterClient, algo, clientID string, capacity, rate int32, windowMs int64, requests int) {
	for i := 1; i <= requests; i++ {
		fmt.Printf("[%2d] ", i)
		check(client, algo, clientID, capacity, rate, windowMs)
		time.Sleep(100 * time.Millisecond)
	}
}

func runConcurrent(client pb.RateLimiterClient, algo, clientID string, capacity, rate int32, windowMs int64, requests int) {
	var wg sync.WaitGroup
	start := time.Now()

	for i := 1; i <= requests; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fmt.Printf("[%2d] ", n)
			check(client, algo, clientID, capacity, rate, windowMs)
		}(i)
	}

	wg.Wait()
	fmt.Printf("\ncompleted %d concurrent requests in %v\n", requests, time.Since(start))
}