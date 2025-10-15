package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"io"
	"log"
	"net/http"
	"os"
)

var POOL Pool

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
}

func (p *Pool) GetApplication() Application {
	n := len(p.Applications)
	app := p.Applications[p.index%n]

	p.index += 1

	return app
}

func universalHandler(w http.ResponseWriter, r *http.Request) {
	// This will receive any request to any path
	// I can then call my own requests to applications
	fmt.Printf("Received request: %s %s\n", r.Method, r.URL.Path)

	app := POOL.GetApplication()
	fmt.Println("Using application:", app)

	if r.Method == "GET" {
		resource := app.Url() + r.URL.Path

		fmt.Printf("Calling \"%v\"\n", resource)

		resp, err := http.Get(resource)

		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading body:", err)
			return
		}

		w.WriteHeader(resp.StatusCode)
		w.Write(body)

		fmt.Fprintf(w, "Handled: %s", r.URL.Path)
		return
	}

	if r.Method == "POST" {
		resource := app.Url() + r.URL.Path
		fmt.Printf("Calling POST \"%v\"\n", resource)

		// Forward the POST request with body
		resp, err := http.Post(resource, r.Header.Get("Content-Type"), r.Body)
		if err != nil {
			fmt.Println("Error:", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Read response body
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
}

func main() {
	content, err := os.ReadFile("applications.toml")
	if err != nil {
		log.Fatal(err)
	}

	if _, err := toml.Decode(string(content), &POOL); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", universalHandler)

	fmt.Println("Load balancer listening on :8080")
	http.ListenAndServe(":8080", nil)
}
