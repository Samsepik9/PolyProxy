// Command proxypool runs a local HTTP + SOCKS5 proxy that forwards traffic
// through a configurable pool of upstream proxies. A web dashboard shows live
// connections, traffic counters, and per-proxy health.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jeff/proxypool/internal/api"
	"github.com/jeff/proxypool/internal/config"
	"github.com/jeff/proxypool/internal/conntrack"
	"github.com/jeff/proxypool/internal/pool"
	"github.com/jeff/proxypool/internal/proxy"
)

const version = "0.1.0"

func main() {
	var (
		cfgPath string
		showVer bool
	)
	flag.StringVar(&cfgPath, "config", "", "path to config.yaml (default: "+config.DefaultConfigPath()+")")
	flag.BoolVar(&showVer, "version", false, "print version and exit")
	flag.Parse()

	if showVer {
		log.Printf("proxypool %s", version)
		return
	}

	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// Pool
	p, err := pool.New(&cfg.Pool)
	if err != nil {
		log.Fatalf("pool init: %v", err)
	}
	p.StartHealthCheck(cfg.Pool.HealthCheck, 30*time.Second)

	cm := conntrack.NewManager()

	// Proxy servers
	dialer := proxy.NewDialer(15 * time.Second)
	httpSrv := &proxy.HTTPServer{Pool: p, Dialer: dialer, Cm: cm}
	socksSrv := &proxy.SOCKS5Server{Pool: p, Dialer: dialer, Cm: cm}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if cfg.Server.HTTPListen != "" {
		go func() {
			log.Printf("[http]   listening on %s", cfg.Server.HTTPListen)
			if err := httpSrv.ListenAndServe(rootCtx, cfg.Server.HTTPListen); err != nil {
				log.Printf("[http]   server stopped: %v", err)
			}
		}()
	}
	if cfg.Server.SOCKS5Listen != "" {
		go func() {
			log.Printf("[socks5] listening on %s", cfg.Server.SOCKS5Listen)
			if err := socksSrv.ListenAndServe(rootCtx, cfg.Server.SOCKS5Listen); err != nil {
				log.Printf("[socks5] server stopped: %v", err)
			}
		}()
	}
	if cfg.Server.APIEnable && cfg.Server.APIListen != "" {
		apiSrv := &api.Server{Cm: cm, Pool: p}
		go func() {
			addr := cfg.Server.APIListen
			log.Printf("[api]    dashboard on http://%s", addr)
			srv := &http.Server{
				Addr:              addr,
				Handler:           apiSrv.Handler(),
				ReadHeaderTimeout: 5 * time.Second,
			}
			go func() {
				<-rootCtx.Done()
				shutCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
				defer c()
				_ = srv.Shutdown(shutCtx)
			}()
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[api]    server stopped: %v", err)
			}
		}()
	}

	log.Printf("proxypool started · strategy=%s · %d upstreams", p.Strategy(), len(p.List()))

	<-rootCtx.Done()
	log.Printf("shutdown signal received, closing connections…")
	cm.CloseAll()
	time.Sleep(200 * time.Millisecond)
}