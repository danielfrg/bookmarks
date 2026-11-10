package main

import (
	"context"
	"errors"
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

const queueShutdownTimeout = 10 * time.Second

// run starts the server and blocks until ctx is cancelled. ready, when not nil,
// is called with the listen address once the server accepts connections.
func run(ctx context.Context, ready func(addr string)) error {
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
	defer listener.Close()

	fc := fetch.NewClient(fetch.Options{AllowPrivate: settings.AllowPrivateFetch})
	svc := service.New(db, fc, capture.NewBasic(fc), nil, service.Options{
		DataDir: settings.DataDir, FaviconProvider: settings.FaviconProvider, MaxUploadBytes: settings.MaxUploadBytes,
	})
	queue := jobs.New(db, svc.JobHandlers(), jobs.Options{})
	svc.SetEnqueuer(queue)
	queueCtx, cancelQueue := context.WithCancel(context.Background())
	queueDone := make(chan struct{})
	go func() {
		defer close(queueDone)
		if err := queue.Run(queueCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("job queue: %v", err)
		}
	}()
	handler := httpapi.NewHandler(assets, httpapi.Deps{
		Ping: db.Ping, Service: svc, MCPAllowedHosts: settings.MCPAllowedHosts,
		MaxUploadBytes: settings.MaxUploadBytes, DataDir: settings.DataDir,
	})

	log.Printf("Bookmarks listening at http://%s (data dir: %s)", listener.Addr(), settings.DataDir)
	if ready != nil {
		ready(listener.Addr().String())
	}
	serveErr := serve(ctx, listener, handler, 5*time.Second)

	// Stop the workers before the deferred db.Close.
	cancelQueue()
	select {
	case <-queueDone:
	case <-time.After(queueShutdownTimeout):
		log.Printf("job queue did not stop within %s", queueShutdownTimeout)
	}
	return serveErr
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, nil); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
