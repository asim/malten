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

On a systemd host with Go 1.25 or newer, a C compiler, and curl installed, run this from
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
`/usr/local/bin/malten`. For an existing WhatsApp deployment, retain the working
directory containing your paired `malten.db` or migrate it before switching units.

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
