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
	Cache        bool
	CacheSize    int
}

func (p *Pool) GetApplication() Application {
	n := len(p.Applications)
	app := p.Applications[p.index%n]
	p.index += 1
	return app
}

type CachedResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	ExpiresAt  time.Time
}

func (c *CachedResponse) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

func hashKey(path string) string {
	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}

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

func shouldCache(r *http.Request) (bool, int) {
	if !POOL.Cache {
		return false, 0
	}

	cacheControl := r.Header.Get("Cache-Control")
	if cacheControl != "" {
		maxAge, noCache, noStore := parseCacheControl(cacheControl)
		if noStore || noCache {
			return false, 0
		}
		if maxAge > 0 {
			return true, maxAge
		}
	}

	if r.Header.Get("Pragma") == "no-cache" {
		return false, 0
	}

	return true, 0
}

func getCachedResponse(cacheKey string, maxAge int) (*CachedResponse, bool) {
	cachedData := cache.Get(cacheKey)
	if cachedData == nil {
		return nil, false
	}

	cached := cachedData.(*CachedResponse)
	if cached.IsExpired() {
		fmt.Println("Cache EXPIRED")
		cache.Remove(cacheKey)
		return nil, false
	}

	return cached, true
}

func writeCachedResponse(w http.ResponseWriter, cached *CachedResponse, maxAge int) {
	for key, values := range cached.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("X-Cache", "HIT")
	age := int(time.Since(cached.ExpiresAt.Add(-time.Duration(maxAge) * time.Second)).Seconds())
	w.Header().Set("Age", fmt.Sprintf("%d", age))
	w.WriteHeader(cached.StatusCode)
	w.Write(cached.Body)
}

func fetchFromBackend(resource string) (*http.Response, []byte, error) {
	resp, err := http.Get(resource)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, body, nil
}

func determineMaxAge(backendResp *http.Response, clientMaxAge int) (int, bool) {
	backendCacheControl := backendResp.Header.Get("Cache-Control")
	if backendCacheControl == "" {
		return clientMaxAge, true
	}

	backendMaxAge, backendNoCache, backendNoStore := parseCacheControl(backendCacheControl)
	if backendNoStore || backendNoCache {
		return 0, false
	}

	if backendMaxAge > 0 && clientMaxAge == 0 {
		return backendMaxAge, true
	}

	return clientMaxAge, true
}

func cacheResponse(cacheKey string, statusCode int, body []byte, headers http.Header, maxAge int) {
	var expiresAt time.Time
	if maxAge > 0 {
		expiresAt = time.Now().Add(time.Duration(maxAge) * time.Second)
		fmt.Printf("Caching response for %d seconds\n", maxAge)
	} else {
		expiresAt = time.Now().Add(time.Hour * 24 * 365)
		fmt.Println("Caching response indefinitely")
	}

	cachedResp := &CachedResponse{
		StatusCode: statusCode,
		Body:       body,
		Headers:    headers.Clone(),
		ExpiresAt:  expiresAt,
	}
	cache.Put(cacheKey, cachedResp)
}

func writeResponse(w http.ResponseWriter, statusCode int, body []byte, headers http.Header, cacheStatus string) {
	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("X-Cache", cacheStatus)
	w.WriteHeader(statusCode)
	w.Write(body)
}

func handleGetRequest(w http.ResponseWriter, r *http.Request) {
	useCache, maxAge := shouldCache(r)
	cacheKey := hashKey(r.URL.Path)

	// Try to serve from cache
	if useCache {
		if cached, found := getCachedResponse(cacheKey, maxAge); found {
			fmt.Println("Cache HIT for:", r.URL.Path)
			writeCachedResponse(w, cached, maxAge)
			return
		}
		fmt.Println("Cache MISS for:", r.URL.Path)
	} else {
		fmt.Println("Cache BYPASSED for:", r.URL.Path)
	}

	// Fetch from backend
	app := POOL.GetApplication()
	fmt.Println("Using application:", app)
	resource := app.Url() + r.URL.Path
	fmt.Printf("Calling \"%v\"\n", resource)

	resp, body, err := fetchFromBackend(resource)
	if err != nil {
		fmt.Println("Error:", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	// Determine final max-age considering backend headers
	finalMaxAge, shouldCacheResponse := determineMaxAge(resp, maxAge)
	useCache = useCache && shouldCacheResponse

	// Cache the response if applicable
	if useCache && resp.StatusCode == http.StatusOK {
		cacheResponse(cacheKey, resp.StatusCode, body, resp.Header, finalMaxAge)
		fmt.Println("Cached response for:", r.URL.Path)
	}

	writeResponse(w, resp.StatusCode, body, resp.Header, "MISS")
}

func handlePostRequest(w http.ResponseWriter, r *http.Request) {
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

	writeResponse(w, resp.StatusCode, body, resp.Header, "N/A")
}

func universalHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request: %s %s\n", r.Method, r.URL.Path)

	switch r.Method {
	case "GET":
		handleGetRequest(w, r)
	case "POST":
		handlePostRequest(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func loadConfig() error {
	content, err := os.ReadFile("applications.toml")
	if err != nil {
		return err
	}

	if _, err := toml.Decode(string(content), &POOL); err != nil {
		return err
	}

	return nil
}

func initializeCache() {
	cacheSize := POOL.CacheSize
	if cacheSize == 0 {
		cacheSize = 100
	}
	cache = NewLRUCache(cacheSize)

	if POOL.Cache {
		fmt.Printf("LRU Cache enabled (capacity: %d)\n", cacheSize)
	} else {
		fmt.Println("Cache disabled")
	}
}

func main() {
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}

	initializeCache()

	http.HandleFunc("/", universalHandler)
	fmt.Println("Load balancer listening on :8080")
	http.ListenAndServe(":8080", nil)
}
