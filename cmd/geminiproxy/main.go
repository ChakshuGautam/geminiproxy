// cmd/geminiproxy/main.go
package main

import (
	"log"
	"net/http"

	"geminiproxy"
)

func main() {
	km, err := geminiproxy.NewKeyManager(geminiproxy.DefaultKeysFile)
	if err != nil {
		log.Fatalf("ERROR: %v", err)
	}

	proxy := geminiproxy.NewProxyServer(km, geminiproxy.DefaultPort)
	log.Printf("Starting proxy server on %s...", proxy.ProxyURL())

	go func() {
		if err := proxy.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ERROR: Could not start proxy: %v", err)
		}
	}()

	// block forever
	select {}
}
