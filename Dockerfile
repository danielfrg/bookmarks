# Frontend
FROM --platform=$BUILDPLATFORM node:26.10.0-bookworm-slim@sha256:3ffc19ea878019d9e9ae8971732ad4a03cda44f167107173174b60ed7c65bed3 AS frontend
WORKDIR /src
RUN npm install --global pnpm@10.28.0
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm test && pnpm check && pnpm build && test -s dist/index.html

# Backend
FROM --platform=$BUILDPLATFORM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=frontend /src/dist/ ./internal/web/dist/
RUN test -s internal/web/dist/index.html && go test ./... && go vet ./...
ARG TARGETOS
ARG TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/bookmarks ./cmd/bookmarks
RUN mkdir -p /out/data && chmod 700 /out/data

# Final image with single binary
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=build /out/bookmarks /bookmarks
COPY --from=build --chown=65532:65532 /out/data/ /data/
ARG VCS_REF
LABEL org.opencontainers.image.source="https://github.com/danielfrg/bookmarks" \
      org.opencontainers.image.revision=$VCS_REF \
      org.opencontainers.image.licenses="MIT"
ENV BOOKMARKS_ADDR=:8080 BOOKMARKS_DATA_DIR=/data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/bookmarks"]
