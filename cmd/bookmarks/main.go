package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielfrg/bookmarks/internal/capture"
	"github.com/danielfrg/bookmarks/internal/fetch"
	"github.com/danielfrg/bookmarks/internal/httpapi"
	"github.com/danielfrg/bookmarks/internal/jobs"
	"github.com/danielfrg/bookmarks/internal/service"
	"github.com/danielfrg/bookmarks/internal/store"
	"github.com/danielfrg/bookmarks/internal/web"
)

// noopQueue is a placeholder until the background job queue is wired in.
type noopQueue struct{}

func (noopQueue) Enqueue(context.Context, jobs.Kind, int64) error { return nil }

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
	fc := fetch.NewClient(fetch.Options{AllowPrivate: settings.AllowPrivateFetch})
	svc := service.New(db, fc, capture.NewBasic(fc), noopQueue{}, service.Options{
		DataDir: settings.DataDir, FaviconProvider: settings.FaviconProvider, MaxUploadBytes: settings.MaxUploadBytes,
	})
	handler := httpapi.NewHandler(assets, httpapi.Deps{
		Ping: db.Ping, Service: svc, APIToken: settings.APIToken, MCPAllowedHosts: settings.MCPAllowedHosts,
		MaxUploadBytes: settings.MaxUploadBytes, DataDir: settings.DataDir,
	})
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
