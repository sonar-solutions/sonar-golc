package utils

import (
	"net/http"
	"time"
)

// HTTPClient is a shared client with connection pooling enabled.
// http.Client is safe for concurrent use by multiple goroutines.
var HTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}
