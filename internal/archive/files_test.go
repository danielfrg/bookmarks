package archive

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesRoundTrip(t *testing.T) {
	f := Files{Dir: filepath.Join(t.TempDir(), "archives")}
	data := []byte(strings.Repeat("hello archive ", 100))
	size, err := f.Write("1/2-server.gz", bytes.NewReader(data), 1<<20)
	if err != nil || size != int64(len(data)) {
		t.Fatalf("Write = %d, %v", size, err)
	}
	raw, err := os.ReadFile(filepath.Join(f.Dir, "1", "2-server.gz"))
	if err != nil || len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		t.Fatalf("file is not gzip: %v", err)
	}
	rc, err := f.Open("1/2-server.gz")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	got.ReadFrom(rc)
	rc.Close()
	if !bytes.Equal(got.Bytes(), data) {
		t.Fatal("round trip mismatch")
	}
	if err := f.Remove("1/2-server.gz"); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove("1/2-server.gz"); err != nil {
		t.Fatalf("missing remove: %v", err)
	}
	if _, err := f.Open("1/2-server.gz"); err == nil {
		t.Fatal("open after remove should fail")
	}
}

func TestFilesTooLarge(t *testing.T) {
	f := Files{Dir: t.TempDir()}
	_, err := f.Write("1/3-upload.gz", strings.NewReader("12345678901"), 10)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.Write("1/4-upload.gz", strings.NewReader("1234567890"), 10); err != nil {
		t.Fatalf("exact limit: %v", err)
	}
	os.Remove(filepath.Join(f.Dir, "1", "4-upload.gz"))
	var files []string
	filepath.WalkDir(f.Dir, func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("leftover files: %v", files)
	}
}

func TestFilesRejectsBadNames(t *testing.T) {
	f := Files{Dir: t.TempDir()}
	for _, n := range []string{"/etc/passwd", "../x.gz", "1/../../x.gz", ""} {
		if _, err := f.Write(n, strings.NewReader("x"), 10); err == nil {
			t.Errorf("Write(%q) should fail", n)
		}
		if _, err := f.Open(n); err == nil {
			t.Errorf("Open(%q) should fail", n)
		}
		if err := f.Remove(n); err == nil {
			t.Errorf("Remove(%q) should fail", n)
		}
	}
}
