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

Browse to `localhost:9090` or host behind an nginx proxy

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

### Enable paid sessions

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

Translation uses the same agent-scoped `MU_TOKEN` as mentions. Enable the text
service on the Malten agent in Mu. If translation fails, check the Malten
service journal for the HTTP status returned by Mu.
