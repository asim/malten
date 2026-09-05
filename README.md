# Malten

Streams of consciousness.

Malten is an open-source tool for building a personal mental model from your
observations and reflections.

Write observations and reflections in a private stream. Hashtags link streams;
time and optional location preserve context.

## Features

- Capture text, voice transcription and photos as points
- Link streams of thought with hashtags
- Attach optional location context to new captures
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
