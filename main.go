package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"net/http"
	"os"
)

var POOL Pool

type Application struct {
	Alive bool
	IP    string
	Port  int
}

type Pool struct {
	Applications []Application `toml:applications`
	index        int
}

func (p *Pool) get_application() Application {
	n := len(p.Applications)
	app := p.Applications[p.index%n]

	p.index += 1

	return app
}

func universalHandler(w http.ResponseWriter, r *http.Request) {
	// This will receive any request to any path
	// I can then call my own requests to applications
	fmt.Printf("Received request: %s %s\n", r.Method, r.URL.Path)

	app := POOL.get_application()
	fmt.Println(app)

	w.WriteHeader(http.StatusOK)
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
