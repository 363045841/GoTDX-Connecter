package main

import (
	"fmt"
	"log"
	"os"

	"BianceAPI/internal/binance"
	"BianceAPI/internal/handler"
	"github.com/gin-gonic/gin"
)

func main() {
	// 默认代理 127.0.0.1:6666，可通过环境变量覆盖
	if os.Getenv("HTTP_PROXY") == "" {
		proxy := "http://127.0.0.1:6666"
		os.Setenv("HTTP_PROXY", proxy)
		os.Setenv("HTTPS_PROXY", proxy)
	}

	// 默认 8081，可通过 PORT 环境变量覆盖
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// 默认 btcusdt,ethusdt，可通过 SYMBOLS 覆盖
	symbols := os.Getenv("SYMBOLS")
	if symbols == "" {
		symbols = "btcusdt,ethusdt"
	}

	gin.SetMode(gin.ReleaseMode)

	// 启动 WebSocket 后台协程
	bc := binance.NewClient(symbols)
	go bc.Start()

	// 深度事件流 hub
	dh := binance.NewDepthHub()

	// 启动 HTTP 服务
	router := handler.NewRouter(bc, dh)
	addr := fmt.Sprintf(":%s", port)

	log.Printf("starting binance API server on http://127.0.0.1%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
