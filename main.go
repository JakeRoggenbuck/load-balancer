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

	return proto + a.IP + ":" + a.Port
}

type Pool struct {
	Applications []Application `toml:applications`
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
	} else {
		w.WriteHeader(http.StatusOK)
	}

	fmt.Fprintf(w, "Handled: %s", r.URL.Path)
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
