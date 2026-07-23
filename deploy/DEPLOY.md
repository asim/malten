# Deploying Malten

This deploys Malten to a single Linux server behind nginx at **malten.ai**, with
a GitHub Action that builds the binary in CI and restarts the service on every
push to `main`.

Layout on the server:

```
/home/malten/            # home of the `malten` user, and the app's working dir
├── malten               # the binary (shipped by CI)
├── .env                 # config incl. ANTHROPIC_API_KEY (you create this)
└── malten.db            # SQLite database (created on first run)
```

The pieces:

- **systemd** (`deploy/malten.service`) supervises the process, reads `.env`, and
  gives you `start`/`restart`/`status`.
- **nginx** (`deploy/nginx/malten.ai.conf`) terminates TLS for malten.ai and
  reverse-proxies to `127.0.0.1:8080`. The app binds to localhost only, so it is
  reachable exclusively through nginx.
- **GitHub Actions** (`.github/workflows/deploy.yml`) builds + tests + ships the
  binary over SSH and restarts systemd.

Do the one-time server setup below once; after that, merging to `main` deploys.

---

## 1. One-time server setup (as root/sudo)

### 1.1 Create the `malten` user and directory

```bash
# A real login shell (bash) is required: the deploy Action runs `ssh malten@host
# 'bash -s'`, which a nologin shell would refuse.
sudo useradd --system --create-home --home-dir /home/malten --shell /bin/bash malten
```

### 1.2 Create the config file

```bash
sudo -u malten cp deploy/env.example /home/malten/.env   # or write it by hand
sudo -u malten chmod 600 /home/malten/.env
sudoedit /home/malten/.env   # set ANTHROPIC_API_KEY, MALTEN_LLM=claude, etc.
```

(See `deploy/env.example` for the full list. The service starts even without
`.env` — it falls back to the offline stub model — but production wants the key.)

### 1.3 Install the systemd service

```bash
sudo cp deploy/malten.service /etc/systemd/system/malten.service
sudo systemctl daemon-reload
sudo systemctl enable malten          # start on boot (don't `--now` yet: no binary)
```

### 1.4 Let the deploy user restart the service without a password

The Action restarts systemd over SSH, which needs `sudo`. Grant exactly that and
nothing more. Confirm your `systemctl` path first (`command -v systemctl` — this
assumes `/usr/bin/systemctl`):

```bash
sudo tee /etc/sudoers.d/malten >/dev/null <<'EOF'
malten ALL=(root) NOPASSWD: /usr/bin/systemctl restart malten, /usr/bin/systemctl start malten, /usr/bin/systemctl stop malten, /usr/bin/systemctl status malten
EOF
sudo chmod 440 /etc/sudoers.d/malten
sudo visudo -c            # validate
```

### 1.5 Add the CI deploy key

Generate a dedicated keypair (on your machine, not the server) and authorize it
for the `malten` user:

```bash
ssh-keygen -t ed25519 -f malten_deploy -N '' -C 'github-actions deploy'
# copy malten_deploy.pub to the server:
sudo -u malten install -d -m 700 /home/malten/.ssh
sudo -u malten tee -a /home/malten/.ssh/authorized_keys < malten_deploy.pub
sudo -u malten chmod 600 /home/malten/.ssh/authorized_keys
```

The **private** key `malten_deploy` goes into a GitHub secret (step 3). The
`malten` user has a real login shell (set in 1.1) so `ssh malten@host '<cmd>'`
works; if your `sshd` restricts logins (e.g. an `AllowUsers` list), add `malten`.

---

## 2. nginx + TLS (as root/sudo)

Because the final config references certificates that don't exist yet, bring up a
minimal HTTP site first, let certbot obtain the cert and rewrite it, then
(optionally) drop in the hardened config from this repo.

```bash
sudo apt-get update
sudo apt-get install -y nginx certbot python3-certbot-nginx
sudo install -d -m 0755 /var/www/certbot

# Minimal HTTP site that proxies to the app.
sudo tee /etc/nginx/sites-available/malten.ai >/dev/null <<'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name malten.ai www.malten.ai;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }
}
EOF
sudo ln -sf /etc/nginx/sites-available/malten.ai /etc/nginx/sites-enabled/malten.ai
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx

# Point malten.ai (and www) A/AAAA records at this server, then:
sudo certbot --nginx -d malten.ai -d www.malten.ai   # obtains cert + adds TLS + redirect
```

certbot installs a renewal timer automatically (`systemctl list-timers | grep certbot`).

Optionally replace the certbot-edited file with the version-controlled one for a
consistent, reviewed config (the certs now exist, so it will load):

```bash
sudo cp deploy/nginx/malten.ai.conf /etc/nginx/sites-available/malten.ai
sudo nginx -t && sudo systemctl reload nginx
```

**Firewall:** open 80/443 and keep 8080 closed to the world (the app is
localhost-only anyway):

```bash
sudo ufw allow 'Nginx Full'    # 80 + 443
sudo ufw allow 22/tcp          # SSH — or your custom port, e.g. `sudo ufw allow 61194/tcp`
```

---

## 3. GitHub repository secrets

Settings → Secrets and variables → Actions → **New repository secret**:

| Secret | Value |
| --- | --- |
| `DEPLOY_SSH_KEY` | contents of the **private** key `malten_deploy` (whole file) |
| `DEPLOY_HOST` | `malten.ai` (or the server IP) |
| `DEPLOY_USER` | `malten` |
| `DEPLOY_PORT` | SSH port; omit for `22`, otherwise set it (e.g. `61194`) |
| `DEPLOY_KNOWN_HOSTS` | optional but recommended; output of `ssh-keyscan -p <port> malten.ai` to pin the host key |

Without `DEPLOY_KNOWN_HOSTS` the workflow trust-on-first-uses the host key each
run; pinning it is more secure.

---

## 4. First deploy

Everything after this deploys automatically when a PR is merged into `main`, but
kick off the first one manually: **Actions → deploy → Run workflow**.

The job will: `go vet` + `go test`, build a static `linux/amd64` binary, `scp` it
to `/home/malten/malten.new`, atomically `mv` it into place, and
`sudo systemctl restart malten`. It fails loudly (and prints `journalctl`) if the
service doesn't come back up.

Verify:

```bash
curl -s https://malten.ai/api/health      # {"model":"claude:...","status":"ok"}
sudo systemctl status malten
sudo journalctl -u malten -f               # live logs
```

---

## How ongoing deploys work

1. You merge a PR to `main`.
2. The `deploy` workflow builds and tests; a red test blocks the deploy.
3. The new binary is shipped and systemd is restarted.

The SQLite database at `/home/malten/malten.db` is never touched by a deploy, so
sessions, tickets and the audit log persist across releases. The binary is
replaced by rename, so an in-flight request finishes on the old process before
the restart swaps in the new one.

## Troubleshooting

- **Service won't start** → `sudo journalctl -u malten -n 50`. Usually a bad
  `.env` line or a port already in use.
- **502 from nginx** → the app isn't listening on `127.0.0.1:8080`; check
  `MALTEN_ADDR` in `.env` and that `malten` is active.
- **Deploy step "sudo: a password is required"** → the sudoers rule (1.4) is
  missing or its `systemctl` path doesn't match; fix and re-run.
- **Deploy step "Text file busy"** → shouldn't happen (we rename, not overwrite);
  if it does, the service didn't stop — check the unit's `ExecStart` path.
