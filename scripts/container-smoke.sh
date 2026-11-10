#!/usr/bin/env bash
set -euo pipefail
image_ref=${1:?usage: container-smoke.sh IMAGE}
smoke_dir=$(mktemp -d)
smoke_suffix=$(basename "$smoke_dir" | tr -cd '[:alnum:]')
containers=()
volumes=()
cleanup() {
  if ((${#containers[@]})); then docker rm -f "${containers[@]}" >/dev/null 2>&1 || true; fi
  if ((${#volumes[@]})); then docker volume rm "${volumes[@]}" >/dev/null 2>&1 || true; fi
  rm -rf "$smoke_dir"
}
trap cleanup EXIT
[[ $(docker image inspect "$image_ref" --format '{{.Config.User}}') == 65532:65532 ]]
new_volume() {
  smoke_volume="bookmarks-smoke-${smoke_suffix}-$1"
  volumes+=("$smoke_volume")
  docker volume create "$smoke_volume" >/dev/null
}
start_server() {
  local server_name="bookmarks-smoke-${smoke_suffix}-$1" data_volume=$2
  local listen_port
  listen_port=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()')
  smoke_base="http://127.0.0.1:$listen_port"
  containers+=("$server_name")
  docker run -d --name "$server_name" --read-only --tmpfs /tmp \
    -p "127.0.0.1:$listen_port:8080" -v "$data_volume:/data" \
    -e "BOOKMARKS_PUBLIC_URL=$smoke_base" -e BOOKMARKS_API_TOKEN=smoke "$image_ref" >/dev/null
  for _ in {1..60}; do
    if curl --fail --silent "$smoke_base/readyz" >/dev/null; then return; fi
    sleep .2
  done
  docker logs "$server_name"
  return 1
}
request() {
  curl --fail --silent -H 'Authorization: Bearer smoke' "$@"
}
new_volume data; data_volume=$smoke_volume
start_server data "$data_volume"; base=$smoke_base
curl --fail --silent "$base/healthz" >/dev/null
curl --fail --silent "$base/" > "$smoke_dir/index.html"
python3 - "$smoke_dir/index.html" > "$smoke_dir/asset" <<'PY'
import re,sys
text=open(sys.argv[1]).read()
assert '<title>Bookmarks</title>' in text
asset=re.search(r'src="(/assets/[^"]+\.js)"',text)
assert asset, 'embedded frontend asset missing'
print(asset.group(1))
PY
curl --fail --silent "$base$(cat "$smoke_dir/asset")" >/dev/null
curl --fail --silent "$base/api/docs" >/dev/null

request -H 'Content-Type: application/json' \
  -d '{"url":"https://example.org/smoke","title":"Smoke","tags":["smoke"]}' \
  "$base/api/bookmarks?capture=none" > "$smoke_dir/bookmark.json"
bookmark_id=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["id"])' "$smoke_dir/bookmark.json")
printf '<!doctype html><html><body><h1>Smoke archive</h1></body></html>\n' > "$smoke_dir/archive.html"
request -F source=extension -F "file=@$smoke_dir/archive.html;type=text/html" \
  "$base/api/bookmarks/$bookmark_id/archives" > "$smoke_dir/archive.json"
archive_id=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["id"])' "$smoke_dir/archive.json")

verify_data() {
  local server=$1
  request "$server/api/bookmarks/$bookmark_id" > "$smoke_dir/got-bookmark.json"
  request "$server/api/bookmarks/$bookmark_id/archives" > "$smoke_dir/got-archives.json"
  python3 - "$smoke_dir/got-bookmark.json" "$smoke_dir/got-archives.json" "$archive_id" <<'PY'
import json,sys
b=json.load(open(sys.argv[1]));a=json.load(open(sys.argv[2]))
assert b['url']=='https://example.org/smoke' and b['title']=='Smoke', b
assert [x['id'] for x in a]==[int(sys.argv[3])], a
assert a[0]['source']=='extension' and a[0]['content_type']=='text/html', a
PY
  curl --fail --silent "$server/archives/$archive_id" | grep -q 'Smoke archive'
}
verify_data "$base"

docker rm -f "bookmarks-smoke-${smoke_suffix}-data" >/dev/null
start_server data "$data_volume"; base=$smoke_base
verify_data "$base"
printf 'Container verified: embedded UI, API, nonroot storage and restart persistence.\n'
