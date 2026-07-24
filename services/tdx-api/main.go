package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	heartbeatContext, stopHeartbeat := context.WithCancel(context.Background())
	heartbeatDone, err := client.StartHeartbeat(heartbeatContext, client.DefaultHeartbeatInterval, client.DefaultHeartbeatFailureThreshold)
	if err != nil {
		log.Fatalf("unable to start gotdx heartbeat: %v", err)
	}
	router := api.NewRouter()
	addr := fmt.Sprintf(":%s", port)
	server := &http.Server{Addr: addr, Handler: router}
	serverErrors := make(chan error, 1)

	log.Printf("starting gotdx server on http://127.0.0.1%s", addr)
	go func() { serverErrors <- server.ListenAndServe() }()
	go func() {
		if err := client.Start(); err != nil {
			log.Printf("gotdx started in degraded mode: %v", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Printf("received %s, shutting down", sig)
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(shutdownContext) }()
	stopHeartbeat()
	select {
	case <-heartbeatDone:
	case <-shutdownContext.Done():
		log.Printf("heartbeat shutdown timed out: %v", shutdownContext.Err())
	}
	if err := <-shutdownDone; err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
	if err := client.CloseContext(shutdownContext); err != nil {
		log.Printf("gotdx close failed: %v", err)
	}
}
