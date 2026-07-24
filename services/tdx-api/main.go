package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"KlineChartQuantGo/services/tdx-api/internal/api"
	"KlineChartQuantGo/services/tdx-api/internal/client"
	"github.com/gin-gonic/gin"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	gin.SetMode(gin.ReleaseMode)
	if client.Get() == nil {
		log.Fatal("unable to initialize gotdx client")
	}
	heartbeatContext, stopHeartbeat := context.WithCancel(context.Background())
	defer stopHeartbeat()
	if err := client.StartHeartbeat(heartbeatContext, client.DefaultHeartbeatInterval, client.DefaultHeartbeatFailureThreshold); err != nil {
		log.Fatalf("unable to start gotdx heartbeat: %v", err)
	}
	router := api.NewRouter()
	addr := fmt.Sprintf(":%s", port)

	log.Printf("starting gotdx server on http://127.0.0.1%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
