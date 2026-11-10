// Command import-linkding copies everything from a linkding instance into
// Bookmarks through both REST APIs: bookmarks (active and archived) with
// their dates, the complete assets as archives, and bundles as saved
// searches. It is safe to re-run: bookmarks merge by URL, archives already
// copied are skipped and saved searches are matched by name.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import-linkding", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: LINKDING_TOKEN=... BOOKMARKS_TOKEN=... import-linkding --linkding-url URL --bookmarks-url URL [flags]")
		fs.PrintDefaults()
	}
	var cfg Config
	fs.StringVar(&cfg.LinkdingURL, "linkding-url", "", "linkding base URL, e.g. https://linkding.example.com")
	fs.StringVar(&cfg.BookmarksURL, "bookmarks-url", "", "Bookmarks base URL, e.g. https://bookmarks.example.com")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "read everything, write nothing, print counts")
	fs.IntVar(&cfg.Limit, "limit", 0, "only import the first N bookmarks (0 = all)")
	fs.BoolVar(&cfg.SkipAssets, "skip-assets", false, "do not copy snapshots and uploaded files")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	// Tokens only come from the environment so they stay out of ps and
	// shell history.
	cfg.LinkdingToken = os.Getenv("LINKDING_TOKEN")
	cfg.BookmarksToken = os.Getenv("BOOKMARKS_TOKEN")
	if cfg.Limit < 0 {
		fmt.Fprintln(stderr, "--limit must not be negative")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// No client timeout: JSON requests get 60 s each, asset transfers are
	// bounded only by ctx (see linkding.go).
	sum, err := Run(ctx, cfg, &http.Client{}, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "import-linkding:", err)
		return 1
	}
	if sum.Failures > 0 {
		fmt.Fprintf(stderr, "import-linkding: %d failures; fix them and re-run (re-runs skip what was already copied)\n", sum.Failures)
		return 1
	}
	return 0
}
