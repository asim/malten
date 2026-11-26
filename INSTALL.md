# Installing Malten

Use the Go toolchain to install

```
go install github.com/asim/malten@latest
```

Or download the latest release binary

- https://github.com/asim/malten/releases/latest

Run `malten` if the Go install directory is on your PATH. For a downloaded binary, run

```
./malten
```

Browse to `http://localhost:9090` locally.

## Deploy or redeploy on Linux

On a systemd host with Go 1.25 or newer and curl installed, run this from
the checkout you want to deploy (update the checkout first for a redeploy):

```sh
./scripts/deploy.sh
```

The script builds the current checkout, installs the binary at
`/usr/local/bin/malten`, enables the service at boot, restarts it, and checks
`/streams` on port 9090. The server keeps live state in memory and writes changed streams to
`streams.json` in its working directory once a second and on clean shutdown.
The snapshot covers the last 24 hours, or less for streams with a shorter TTL.
If the process stops abruptly, changes since the last checkpoint can be lost.
While stopped, the file remains on disk. Startup prunes it before serving requests.
The supplied unit
reads optional settings from `/etc/default/malten`. When deployed with the existing
`malten` account, it also reads `/home/malten/.env` for settings such as `MU_TOKEN`. It runs in `/var/lib/malten`.
If you already manage a `malten.service`, its executable must point to
`/usr/local/bin/malten`.

### nginx on malten.me

nginx handles public HTTPS and proxies requests to Malten on `127.0.0.1:9090`. For an existing HTTPS site, put the `map` in nginx’s `http` context and use the example’s proxy directives in the active `location /`. Keep the site’s TLS settings. The upgrade headers carry live stream events, including moderation removals. [Example nginx configuration](config/nginx.conf)

nginx appends the visitor’s address to `X-Forwarded-For`. Malten uses the last address when the connection comes from loopback. For other connections it uses the peer address. After editing the active site, run `sudo nginx -t && sudo systemctl reload nginx`. The Malten deployment script restarts the Go service separately.

### Moderation and AI usage

Readers can report public posts as soon as Malten is running. To review them, set `MALTEN_MOD_TOKEN` to a strong unique password in the service environment and restart Malten. Open `/moderation` over HTTPS and sign in with the username `moderator` and that password. The queue uses the live stream snapshot. Dismissing a report keeps the post visible. Removing it clears its text and quoted previews.

Malten allows five free AI requests per network address per ten minutes, up to four outstanding at once and 100 per UTC day across the site. Mentions, the command endpoint, private **Ask Malten** dialogs and MCP share the allowance. Set `MALTEN_AI_DAILY_LIMIT` to change the daily request count, or to `0` to pause free AI. Reading and ordinary posts stay available. The date and total are saved in `ai-usage.json` before Mu is called. Network addresses are kept in memory.

Ordinary stream posts are limited to 15 per minute per network and reports to five. The scheduled daily reflection has its own publishing path. Paid rooms allow one AI question at a time and ten per participant per ten minutes, separately from the free allowance.

### Enable paid private reflections

In Mu, open **Edit agent → Mu Pay** for Malten. Enable payments and set the price
per participant to `300` minor units, currency to `GBP`, duration to `60` minutes,
and the app return URL to `https://malten.me/session`. Malten displays the quote
returned by Mu for each booking. Use the existing server-side `MU_TOKEN` with
agent API read and write access. Mu handles Stripe checkout and its webhook.
Malten reads the booking from Mu when the host returns or chooses **Check payment**.
The Mu payment notification URL can be left blank.

Serve Malten over HTTPS and forward the original Host and protocol from the reverse proxy.
Guest places are tied to an HTTP-only browser cookie. The service writes booking
and seat changes to its private `sessions/` directory before acknowledging them.
Paid bookings stay available until the host starts them. Once the shared time ends,
Malten closes access immediately and removes the room and its messages from disk
on its next one-second cleanup or on startup. Mu manages its own payment records
and agent thread.

### Automatic updates

To check the current branch’s upstream every five minutes, add this to the
root user's crontab, adjusting the checkout path:

```cron
*/5 * * * * /opt/malten/scripts/deploy.sh --update
```

Use a root-owned checkout on a clean tracking branch with noninteractive Git access.
Root pulls, builds and installs. When the `malten` account exists, a systemd
drop-in runs the service under that account and its primary group. Otherwise,
the supplied service uses a dynamic account.
The first check deploys if this checkout has no successful deployment recorded.
Later checks fast-forward and restart only when the revision differs from the
last healthy deployment. Overlapping cron runs skip the busy checkout.

**Ask Malten** and stream mentions use the same agent-scoped `MU_TOKEN`. The
agent can use the lookup services enabled for it in Mu. If a request fails,
check the Malten service journal for the HTTP status returned by Mu.
