# Malten

A private stream of observations and reflections. Keep the experience quiet:
one input, the user's words, hashtags linking streams, and subdued time/place.
No generated responses, maps, dashboards, or notifications.

## Structure

- cmd/malten: HTTP entrypoint; preserve its build path and MALTEN_ADDR.
- internal/server: embedded PWA, health endpoint, reverse geocoding.
- internal/server/web: capture, stream rendering, local storage, offline shell.
- deploy and .github/workflows: existing automated server deployment.

## Constraints

Preserve existing browser storage and migrations. Old connections and coordinates
remain valid saved data; don't discard them when rendering the stream.
No server-side reflection storage. Location lookup sends coordinates to the
configured geocoder. Browser speech recognition may use a browser-provider service.
Keep the server standard-library-only and the web assets embedded. No CDN.
Prefer readable functions over abstractions or compressed one-liners.

## Validation

Run go test ./..., go vet ./..., and go build ./cmd/malten.
Test capture, hashtag navigation, reload persistence, and offline launches.
