package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"os"
)

type Application struct {
	Alive bool
	IP    string
	Port  int
}

type Pool struct {
	Applications []Application `toml:applications`
}

func main() {
	content, err := os.ReadFile("applications.toml")
	if err != nil {
		log.Fatal(err)
	}

	var pool Pool
	if _, err := toml.Decode(string(content), &pool); err != nil {
		log.Fatal(err)
	}

	fmt.Println(pool)
}
