// Package archive stores gzipped archive files on disk.
package archive

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrTooLarge = errors.New("archive too large")

// Files stores gzipped archives under Dir. Names are relative slash paths such
// as "12/1700000000-server.gz", built by the service.
type Files struct{ Dir string }

func (f Files) path(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsRune(name, '\\') || filepath.IsAbs(name) {
		return "", errors.New("invalid archive name")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "" || part == "." {
			return "", errors.New("invalid archive name")
		}
	}
	return filepath.Join(f.Dir, filepath.FromSlash(name)), nil
}

// Write gzips r into name via a temp file and rename. size is the number of
// uncompressed bytes; more than maxBytes fails with ErrTooLarge.
func (f Files) Write(name string, r io.Reader, maxBytes int64) (size int64, err error) {
	dst, err := f.path(name)
	if err != nil {
		return 0, err
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, fmt.Errorf("create archive directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("create archive file: %w", err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	zw := gzip.NewWriter(tmp)
	size, err = io.Copy(zw, io.LimitReader(r, maxBytes+1))
	if err != nil {
		return 0, err
	}
	if size > maxBytes {
		return 0, ErrTooLarge
	}
	if err = zw.Close(); err != nil {
		return 0, err
	}
	if err = tmp.Sync(); err != nil {
		return 0, err
	}
	if err = tmp.Close(); err != nil {
		return 0, err
	}
	if err = os.Rename(tmp.Name(), dst); err != nil {
		return 0, err
	}
	return size, nil
}

type readCloser struct {
	*gzip.Reader
	file *os.File
}

func (r readCloser) Close() error {
	zerr := r.Reader.Close()
	if err := r.file.Close(); err != nil {
		return err
	}
	return zerr
}

// Open returns a decompressing reader for name.
func (f Files) Open(name string) (io.ReadCloser, error) {
	p, err := f.path(name)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	return readCloser{zr, file}, nil
}

// Remove deletes name; a missing file is not an error.
func (f Files) Remove(name string) error {
	p, err := f.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
