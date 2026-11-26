package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/httpapi"
	"github.com/sourcebox/sourcebox/internal/legacy"
	"github.com/sourcebox/sourcebox/internal/refresh"
	"github.com/sourcebox/sourcebox/internal/store"
)

func main() {
	printAdminPath := flag.Bool("print-admin-path", false, "print the dashboard URL and exit")
	flag.Parse()

	cfg := config.Load()

	db, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	legacy.ImportConfig(db, cfg)

	handler := httpapi.New(cfg, db, refresh.NewFactory(cfg))

	if *printAdminPath {
		fmt.Println(handler.AdminURL())
		return
	}

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           httpapi.Recover(handler),
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		log.Printf("SourceBox %s listening on http://%s", httpapi.Version, cfg.Addr())
		log.Printf("source endpoint: %s", handler.SourceURL())
		log.Printf("admin dashboard:  %s", handler.AdminURL())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

