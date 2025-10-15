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

// CachedResponse stores the response data
type CachedResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// hashKey creates a cache key from the request path
func hashKey(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

func universalHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request: %s %s\n", r.Method, r.URL.Path)

	if r.Method == "GET" {
		// Check cache first if caching is enabled
		if POOL.Cache {
			cacheKey := hashKey(r.URL.Path)
			if cachedData := cache.Get(cacheKey); cachedData != nil {
				cached := cachedData.(*CachedResponse)
				fmt.Println("Cache HIT for:", r.URL.Path)

				// Write cached headers
				for key, values := range cached.Headers {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.Header().Add("X-Cache", "HIT")
				w.WriteHeader(cached.StatusCode)
				w.Write(cached.Body)
				return
			}
			fmt.Println("Cache MISS for:", r.URL.Path)
		}

		// Cache miss or caching disabled - fetch from backend
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

		// Store in cache if enabled and response is successful
		if POOL.Cache && resp.StatusCode == http.StatusOK {
			cacheKey := hashKey(r.URL.Path)
			cachedResp := &CachedResponse{
				StatusCode: resp.StatusCode,
				Body:       body,
				Headers:    resp.Header.Clone(),
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
		w.Header().Add("X-Cache", "MISS")
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
