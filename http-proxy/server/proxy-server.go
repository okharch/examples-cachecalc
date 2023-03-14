package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"github.com/okharch/cachecalc"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func CalculateSHA256(data []byte) ([]byte, error) {
	hash := sha256.New()
	_, err := hash.Write(data)
	if err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

func main() {
	// redefine cachecalc.DefaultCCs to use REDIS_URI for persistance
	ctx, cancel := context.WithCancel(context.Background())
	redisCache, err := cachecalc.NewRedisCache(ctx)
	if err != nil {
		log.Fatalf("Can't connect to redis cache: %s", err)
	}
	cachecalc.DefaultCCs = cachecalc.NewCachedCalculations(4, redisCache)
	// Set up an HTTP endpoint to receive the encoded request
	http.HandleFunc("/proxy", func(w http.ResponseWriter, r *http.Request) {
		// Get the values of the min_ttl and max_ttl query parameters
		minTTL := time.Hour // default minimum TTL is 1 hour
		if minTTLStr := r.URL.Query().Get("min_ttl"); minTTLStr != "" {
			if duration, err := time.ParseDuration(minTTLStr); err == nil {
				minTTL = duration
			}
		}
		maxTTL := time.Hour * 24 // default maximum TTL is 1 day
		if maxTTLStr := r.URL.Query().Get("max_ttl"); maxTTLStr != "" {
			if duration, err := time.ParseDuration(maxTTLStr); err == nil {
				maxTTL = duration
			}
		} // Clone the request body
		body, _ := io.ReadAll(r.Body)
		key, _ := CalculateSHA256(body)
		log.Printf("hash of the request: %x", key)
		forwardRequest := func(ctx context.Context) (result []byte, err error) {
			// Deserialize the encoded request using the gob package
			reader := bytes.NewReader(body)
			dec := gob.NewDecoder(reader)
			var req http.Request
			err = dec.Decode(&req)
			if err != nil {
				return
			}

			// Complete the request
			log.Printf("%+v", req)
			resp, err := http.DefaultClient.Do(&req)
			if err != nil {
				return
			}
			defer func() {
				_ = resp.Body.Close()
			}()

			// Serialize the JSON response
			body, err = io.ReadAll(resp.Body)
			if err != nil {
				return
			}
			respData := map[string]interface{}{
				"status":     resp.Status,
				"body":       string(body),
				"statusCode": resp.StatusCode,
				"refreshed":  time.Now(),
			}
			result, err = json.Marshal(respData)
			return
		}
		cKey := "request." + string(key)
		respBytes, err := cachecalc.GetCachedCalc(ctx, cKey, minTTL, maxTTL, true, forwardRequest)
		if err != nil {
			respBytes = []byte(fmt.Sprintf("{error: %q}", err))
		}
		// Send the serialized JSON response back to the client
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(respBytes)
	})

	// Start the server on port 8080
	srv := &http.Server{Addr: ":8080"}
	go func() {
		log.Printf("listening to :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for stop signal to gracefully shut down the server
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint
	log.Println("Shutting down server...")
	cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server shutdown complete.")
}
