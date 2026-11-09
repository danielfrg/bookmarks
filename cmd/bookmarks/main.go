package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielfrg/bookmarks/internal/httpapi"
	"github.com/danielfrg/bookmarks/internal/store"
	"github.com/danielfrg/bookmarks/internal/web"
)

func run(ctx context.Context) error {
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	db, err := store.Open(settings.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	assets, err := web.Assets()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", settings.Addr)
	if err != nil {
		return err
	}
	log.Printf("Bookmarks listening at http://%s (database: %s)", listener.Addr(), settings.DatabasePath)
	handler := httpapi.NewHandler(assets, httpapi.Deps{Ping: db.Ping})
	return serve(ctx, listener, handler, 5*time.Second)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
