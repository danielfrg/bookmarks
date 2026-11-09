package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/danielfrg/bookmarks/internal/mcp"
)

const defaultFaviconProvider = "https://t1.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url={url}&size=32"

type Settings struct {
	Addr, DataDir, DatabasePath, PublicURL, APIToken string
	MCPAllowedHosts                                  []string
	MaxUploadBytes                                   int64
	FaviconProvider                                  string
	AllowPrivateFetch                                bool
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadSettings() (Settings, error) {
	s := Settings{
		Addr:            env("BOOKMARKS_ADDR", "127.0.0.1:8080"),
		DataDir:         env("BOOKMARKS_DATA_DIR", "./data"),
		PublicURL:       env("BOOKMARKS_PUBLIC_URL", "http://127.0.0.1:8080"),
		FaviconProvider: env("BOOKMARKS_FAVICON_PROVIDER", defaultFaviconProvider),
	}
	s.DatabasePath = filepath.Join(s.DataDir, "bookmarks.db")
	if _, _, err := net.SplitHostPort(s.Addr); err != nil {
		return s, fmt.Errorf("BOOKMARKS_ADDR: %w", err)
	}
	u, err := url.Parse(s.PublicURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return s, errors.New("BOOKMARKS_PUBLIC_URL must be an HTTP(S) origin without a path")
	}
	s.PublicURL = strings.TrimSuffix(s.PublicURL, "/")

	token, tokenFile := os.Getenv("BOOKMARKS_API_TOKEN"), os.Getenv("BOOKMARKS_API_TOKEN_FILE")
	if token != "" && tokenFile != "" {
		return s, errors.New("set only one of BOOKMARKS_API_TOKEN and BOOKMARKS_API_TOKEN_FILE")
	}
	if tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return s, errors.New("BOOKMARKS_API_TOKEN_FILE: cannot read file")
		}
		token = strings.TrimSpace(string(data))
	}
	s.APIToken = token

	hosts, err := mcp.ParseAllowedHosts(os.Getenv("BOOKMARKS_MCP_ALLOWED_HOSTS"))
	if err != nil {
		return s, fmt.Errorf("BOOKMARKS_MCP_ALLOWED_HOSTS: %w", err)
	}
	s.MCPAllowedHosts = hosts

	mb, err := strconv.ParseInt(env("BOOKMARKS_MAX_UPLOAD_MB", "100"), 10, 64)
	if err != nil || mb <= 0 || mb > (1<<62)>>20 {
		return s, errors.New("BOOKMARKS_MAX_UPLOAD_MB must be a positive integer")
	}
	s.MaxUploadBytes = mb << 20

	s.AllowPrivateFetch, err = strconv.ParseBool(env("BOOKMARKS_ALLOW_PRIVATE_FETCH", "false"))
	if err != nil {
		return s, fmt.Errorf("BOOKMARKS_ALLOW_PRIVATE_FETCH: %w", err)
	}
	return s, nil
}
