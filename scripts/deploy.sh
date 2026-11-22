#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT

# Build as the invoking user before touching the running service.
CGO_ENABLED=1 go build -o "$build_dir/malten" .
admin=()
if (( EUID != 0 )); then
    admin=(sudo)
fi

bin=/usr/local/bin/malten
unit=/etc/systemd/system/malten.service
"${admin[@]}" install -d /usr/local/bin
stage=$("${admin[@]}" mktemp "${bin}.XXXXXX")
trap '"${admin[@]}" rm -f "$stage"; rm -rf "$build_dir"' EXIT
"${admin[@]}" install -m 0755 "$build_dir/malten" "$stage"
"${admin[@]}" mv -fT "$stage" "$bin"

# Respect an operator's existing unit on subsequent deployments.
if ! systemctl cat malten.service >/dev/null 2>&1; then
    "${admin[@]}" install -m 0644 config/malten.service "$unit"
fi
"${admin[@]}" systemctl daemon-reload
"${admin[@]}" systemctl enable malten.service
"${admin[@]}" systemctl restart malten.service

for ((attempt=0; attempt<15; attempt++)); do
    if systemctl is-active --quiet malten.service &&
        curl -fsS --max-time 1 http://127.0.0.1:9090/streams >/dev/null 2>&1; then
        echo "Malten is running on port 9090."
        exit 0
    fi
    sleep 1
done
echo "Malten failed its startup check. See journalctl -u malten.service." >&2
exit 1
