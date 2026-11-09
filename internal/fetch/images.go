package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxImageBytes = 5 << 20

var previewExt = map[string]string{
	"image/jpeg":    ".jpg",
	"image/png":     ".png",
	"image/gif":     ".gif",
	"image/svg+xml": ".svg",
	"image/webp":    ".webp",
}

func mediaType(ct string) string {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	}
	return mt
}

// DownloadFavicon fetches the favicon for pageURL from provider (a template
// containing "{url}") into dir and returns the file name.
func (c *Client) DownloadFavicon(ctx context.Context, provider, pageURL, dir string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid page URL")
	}
	origin := strings.ToLower(u.Scheme + "://" + u.Host)
	target := strings.ReplaceAll(provider, "{url}", url.QueryEscape(origin))
	resp, err := c.Get(ctx, target, maxImageBytes)
	if err != nil {
		return "", err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return "", fmt.Errorf("favicon provider returned status %d", resp.Status)
	}
	if resp.Truncated {
		return "", errors.New("favicon too large")
	}
	if !strings.HasPrefix(mediaType(resp.ContentType), "image/") {
		return "", fmt.Errorf("favicon response is not an image")
	}
	name := sanitizeHost(strings.ToLower(u.Hostname())) + ".png"
	if err := writeAtomic(dir, name, resp.Body); err != nil {
		return "", err
	}
	return name, nil
}

// DownloadPreview downloads imageURL into dir and returns the file name.
func (c *Client) DownloadPreview(ctx context.Context, imageURL, dir string) (string, error) {
	resp, err := c.Get(ctx, imageURL, maxImageBytes)
	if err != nil {
		return "", err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return "", fmt.Errorf("unexpected status %d", resp.Status)
	}
	if resp.Truncated {
		return "", errors.New("image too large")
	}
	ext, ok := previewExt[mediaType(resp.ContentType)]
	if !ok {
		return "", fmt.Errorf("unsupported image type %q", mediaType(resp.ContentType))
	}
	sum := sha256.Sum256([]byte(imageURL))
	name := hex.EncodeToString(sum[:]) + ext
	if err := writeAtomic(dir, name, resp.Body); err != nil {
		return "", err
	}
	return name, nil
}

func sanitizeHost(h string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return '_'
	}, h)
}

func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}
