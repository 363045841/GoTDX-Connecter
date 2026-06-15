package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"KlineChartQuantGo/internal/api"
	"KlineChartQuantGo/internal/client"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	client.Get()
	router := api.NewRouter()
	addr := fmt.Sprintf(":%s", port)

	log.Printf("starting gotdx server on http://127.0.0.1%s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
