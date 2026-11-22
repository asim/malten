#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
case "$#:${1:-}" in
    0:|1:--update) ;;
    *) echo "Usage: $0 [--update]" >&2; exit 2 ;;
esac

state=$(git rev-parse --absolute-git-dir)
marker="$state/malten-deployed"
if [[ ${MALTEN_DEPLOY_LOCKED:-} != "$state" ]]; then
    exec 9>"$state/malten-deploy.lock"
    if [[ ${1:-} == --update ]]; then
        flock -n -E 75 9 || { status=$?; (( status == 75 )) && exit 0; exit "$status"; }
    else
        flock 9
    fi
    export MALTEN_DEPLOY_LOCKED="$state"
fi

if [[ ${1:-} == --update ]]; then
    export GIT_TERMINAL_PROMPT=0
    if [[ -n $(git status --porcelain) ]]; then
        echo "Automatic deployment requires a clean checkout." >&2
        exit 1
    fi
    git pull --quiet --ff-only
    revision=$(git rev-parse HEAD)
    if [[ $revision != "$(git rev-parse '@{upstream}')" ]]; then
        echo "Local branch is ahead of its upstream. Skipping deployment." >&2
        exit 1
    fi
    if [[ -f $marker && $(cat "$marker") == "$revision" ]]; then
        exit 0
    fi
    # Use the deployment logic from the newly checked-out revision under the same lock.
    exec ./scripts/deploy.sh
fi

revision=$(git rev-parse HEAD)
if [[ -n $(git status --porcelain) ]]; then
    revision=dirty
fi
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
rm -f "$marker"
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
        printf '%s\n' "$revision" > "$marker.tmp"
        mv -f "$marker.tmp" "$marker"
        echo "Malten is running on port 9090."
        exit 0
    fi
    sleep 1
done
echo "Malten failed its startup check. See journalctl -u malten.service." >&2
exit 1
