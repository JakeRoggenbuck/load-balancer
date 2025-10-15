package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/BurntSushi/toml"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var POOL Pool
var cache *LRUCache

type Application struct {
	Alive bool
	IP    string
	Port  string
	TLS   bool
}

func (a *Application) Url() string {
	var proto string
	if a.TLS {
		proto = "https://"
	} else {
		proto = "http://"
	}
	if a.Port == "0" {
		return proto + a.IP
	}
	return proto + a.IP + ":" + a.Port
}

type Pool struct {
	Applications []Application
	index        int
	Cache        bool // Enable/disable caching from config
	CacheSize    int  // Cache capacity from config
}

func (p *Pool) GetApplication() Application {
	n := len(p.Applications)
	app := p.Applications[p.index%n]
	p.index += 1
	return app
}

// CachedResponse stores the response data with expiry
type CachedResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	ExpiresAt  time.Time
}

// IsExpired checks if the cached response has expired
func (c *CachedResponse) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// hashKey creates a cache key from the request path
func hashKey(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

// parseCacheControl extracts cache directives from Cache-Control header
func parseCacheControl(header string) (maxAge int, noCache bool, noStore bool) {
	directives := strings.Split(header, ",")
	for _, directive := range directives {
		directive = strings.TrimSpace(strings.ToLower(directive))

		if directive == "no-cache" {
			noCache = true
		} else if directive == "no-store" {
			noStore = true
		} else if strings.HasPrefix(directive, "max-age=") {
			ageStr := strings.TrimPrefix(directive, "max-age=")
			if age, err := strconv.Atoi(ageStr); err == nil {
				maxAge = age
			}
		}
	}
	return
}

// shouldCache determines if a request should use caching based on headers
func shouldCache(r *http.Request) (bool, int) {
	if !POOL.Cache {
		return false, 0
	}

	// Check client's Cache-Control header
	cacheControl := r.Header.Get("Cache-Control")
	if cacheControl != "" {
		maxAge, noCache, noStore := parseCacheControl(cacheControl)

		// no-store means don't cache at all
		if noStore || noCache {
			return false, 0
		}

		// Return max-age if specified
		if maxAge > 0 {
			return true, maxAge
		}
	}

	// Check for Pragma: no-cache (legacy HTTP/1.0)
	if r.Header.Get("Pragma") == "no-cache" {
		return false, 0
	}

	// Default: cache with no expiry (or you can set a default like 300 seconds)
	return true, 0
}

func universalHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request: %s %s\n", r.Method, r.URL.Path)

	if r.Method == "GET" {
		useCache, maxAge := shouldCache(r)
		cacheKey := hashKey(r.URL.Path)

		// Check cache if caching is allowed
		if useCache {
			if cachedData := cache.Get(cacheKey); cachedData != nil {
				cached := cachedData.(*CachedResponse)

				// Check if cached response has expired
				if !cached.IsExpired() {
					fmt.Println("Cache HIT for:", r.URL.Path)

					// Write cached headers
					for key, values := range cached.Headers {
						for _, value := range values {
							w.Header().Add(key, value)
						}
					}
					w.Header().Set("X-Cache", "HIT")
					w.Header().Set("Age", fmt.Sprintf("%d", int(time.Since(cached.ExpiresAt.Add(-time.Duration(maxAge)*time.Second)).Seconds())))
					w.WriteHeader(cached.StatusCode)
					w.Write(cached.Body)
					return
				} else {
					fmt.Println("Cache EXPIRED for:", r.URL.Path)
					// Remove expired entry
					cache.Remove(cacheKey)
				}
			}
			fmt.Println("Cache MISS for:", r.URL.Path)
		} else {
			fmt.Println("Cache BYPASSED for:", r.URL.Path)
		}

		// Cache miss, expired, or caching disabled - fetch from backend
		app := POOL.GetApplication()
		fmt.Println("Using application:", app)
		resource := app.Url() + r.URL.Path
		fmt.Printf("Calling \"%v\"\n", resource)

		resp, err := http.Get(resource)
		if err != nil {
			fmt.Println("Error:", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading body:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Check backend's Cache-Control header for max-age
		backendCacheControl := resp.Header.Get("Cache-Control")
		if backendCacheControl != "" {
			backendMaxAge, backendNoCache, backendNoStore := parseCacheControl(backendCacheControl)
			if backendNoStore || backendNoCache {
				useCache = false
			} else if backendMaxAge > 0 && maxAge == 0 {
				maxAge = backendMaxAge
			}
		}

		// Store in cache if enabled and response is successful
		if useCache && resp.StatusCode == http.StatusOK {
			var expiresAt time.Time
			if maxAge > 0 {
				expiresAt = time.Now().Add(time.Duration(maxAge) * time.Second)
				fmt.Printf("Caching response for %d seconds\n", maxAge)
			} else {
				expiresAt = time.Now().Add(time.Hour * 24 * 365) // Far future if no expiry
				fmt.Println("Caching response indefinitely")
			}

			cachedResp := &CachedResponse{
				StatusCode: resp.StatusCode,
				Body:       body,
				Headers:    resp.Header.Clone(),
				ExpiresAt:  expiresAt,
			}
			cache.Put(cacheKey, cachedResp)
			fmt.Println("Cached response for:", r.URL.Path)
		}

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.Header().Set("X-Cache", "MISS")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	if r.Method == "POST" {
		app := POOL.GetApplication()
		fmt.Println("Using application:", app)
		resource := app.Url() + r.URL.Path
		fmt.Printf("Calling POST \"%v\"\n", resource)

		resp, err := http.Post(resource, r.Header.Get("Content-Type"), r.Body)
		if err != nil {
			fmt.Println("Error:", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading body:", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	// Method not supported
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func main() {
	content, err := os.ReadFile("applications.toml")
	if err != nil {
		log.Fatal(err)
	}

	if _, err := toml.Decode(string(content), &POOL); err != nil {
		log.Fatal(err)
	}

	// Initialize cache
	cacheSize := POOL.CacheSize
	if cacheSize == 0 {
		cacheSize = 100 // Default cache size
	}
	cache = NewLRUCache(cacheSize)

	if POOL.Cache {
		fmt.Printf("LRU Cache enabled (capacity: %d)\n", cacheSize)
	} else {
		fmt.Println("Cache disabled")
	}

	http.HandleFunc("/", universalHandler)
	fmt.Println("Load balancer listening on :8080")
	http.ListenAndServe(":8080", nil)
}
