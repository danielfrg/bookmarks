set dotenv-load := true

install:
    [ -d frontend ] && pnpm --dir frontend install --frozen-lockfile || true

api:
    go run ./cmd/bookmarks

check:
    go test -race ./...
    go vet ./...

build:
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bookmarks ./cmd/bookmarks
