# Deploying Malten

The existing GitHub workflow builds ./cmd/malten, runs Go tests and vet, uploads
the binary to /home/malten/malten.new, renames it to /home/malten/malten, and
restarts the malten systemd service on pushes to main.

## Server setup

Keep the malten user, /home/malten working directory, deploy/malten.service,
and nginx configuration in deploy/nginx/malten.ai.conf. The service listens on
127.0.0.1:8080 behind nginx and TLS. Install the unit under /etc/systemd/system,
run systemctl daemon-reload, and enable malten. The deploy user needs permission
to restart and inspect that service through sudo.

Configuration is optional in /home/malten/.env; see env.example. No AI, map,
transport, or push credentials are needed. Reflections stay in the browser.
There is no server database. Deployment does not delete existing server files.

## GitHub secrets

Preserve DEPLOY_SSH_KEY, DEPLOY_HOST, DEPLOY_USER, DEPLOY_PORT (default 22), and
DEPLOY_KNOWN_HOSTS. The known-hosts value pins the server SSH identity. Configure
DNS and nginx TLS certificates before exposing the service publicly.

## Verify

Run systemctl status malten and check /api/health through the public HTTPS URL.
Use journalctl -u malten for startup failures. For a 502, check the service and
MALTEN_ADDR. Verify PWA installation and an offline launch after deployment.
