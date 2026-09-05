# Malten

Malten is an open-source personal network for the things you think, notice,
experience and record.

Capture something as a point, connect it to what matters, and arrange it on an
open spatial surface. Place and time add context without turning the experience
into a map, feed or AI chat.

## Features

- Capture text, voice transcription and photos as points
- Create explicit connections between related points
- Pan, zoom and arrange a personal spatial network
- Attach optional location context to new captures
- Revisit captures in a timeline
- Store the network locally in the browser
- Install as a progressive web app
- Run as a self-contained Go server

## Run Malten

Requires Go 1.25 or later.

```bash
git clone https://github.com/asim/malten.git
cd malten
go run ./cmd/malten
```

Open [localhost:8080](http://localhost:8080). No API key is required.

## Build and test

```bash
go build ./...
go test ./...
```

Set `MALTEN_ADDR` to change the listen address. It defaults to `:8080`.

## Documentation

- [Product strategy](STRATEGY.md)
- [Deployment](deploy/DEPLOY.md)

## License

[GNU Affero General Public License v3.0](LICENSE)
