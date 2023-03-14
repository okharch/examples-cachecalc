package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	// Parse command-line flags
	var remoteURL string
	var minTTL, maxTTL time.Duration
	flag.StringVar(&remoteURL, "url", "https://httpbin.org/get", "URL of the remote server to proxy to")
	flag.DurationVar(&minTTL, "min_ttl", time.Hour, "Minimum TTL for cached responses")
	flag.DurationVar(&maxTTL, "max_ttl", time.Hour*24, "Maximum TTL for cached responses")
	flag.Parse()

	// Serialize the HTTP request
	req, err := http.NewRequest("GET", remoteURL, nil)
	if err != nil {
		// Handle error
		fmt.Println("Error:", err)
		return
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err = enc.Encode(req)
	if err != nil {
		// Handle error
		fmt.Println("Error:", err)
		return
	}

	// Send the serialized request to the proxy server with request parameters
	url := fmt.Sprintf("http://localhost:8080/proxy?min_ttl=%s&max_ttl=%s", minTTL.String(), maxTTL.String())
	resp, err := http.Post(url, "application/octet-stream", &buf)
	if err != nil {
		// Handle error
		fmt.Println("Error:", err)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Read the serialized JSON response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// Handle error
		fmt.Println("Error:", err)
		return
	}

	// Deserialize the JSON response
	var respData interface{}
	err = json.Unmarshal(body, &respData)
	if err != nil {
		// Handle error
		fmt.Println("Error:", err)
		return
	}

	// Print the response data
	fmt.Println(respData)
}
