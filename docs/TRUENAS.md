# Running MineOS (SvelteKit) on TrueNAS SCALE

> **On the "will this be merged into the TrueNAS app?" question:** this project
> is a separate, rewritten MineOS — a .NET API plus a SvelteKit web UI. It is
> not a drop-in replacement for the Node.js MineOS that the TrueNAS catalog app
> packages, and there is no shared deployment. Whether the catalog app ever
> switches is not something this repository controls. You do not have to wait
> for that, though: MineOS ships as two ordinary container images and runs on
> TrueNAS SCALE today, via a custom app or Docker Compose.

This guide covers what is specific to TrueNAS. For everything else see the
[README](../README.md).

> These are the TrueNAS-specific considerations derived from the compose files
> in this repository. TrueNAS moves its app UI around between releases, so treat
> the click-paths as a description of what to configure, not exact menu labels.

## Before you start

Create the datasets MineOS will use, so its data lives on your pool and
survives an app redeploy:

| Dataset | Holds | Mounted at |
|---------|-------|------------|
| `tank/apps/mineos/minecraft` | All server data (worlds, jars, backups) | `/var/games/minecraft` |
| `tank/apps/mineos/data` | The MineOS database and settings | `/app/data` |

Use whatever pool name you actually have in place of `tank`.

## Option A — Docker Compose (recommended)

TrueNAS SCALE can run a compose file directly (recent releases expose this as a
custom app; older ones need the `docker compose` CLI over SSH). This is the
closest match to how MineOS is developed and released.

1. Download the install bundle from the
   [latest release](https://github.com/freeman412/mineos-sveltekit/releases/latest)
   and unpack it into a dataset, or clone this repository.

2. Copy `.env.template` to `.env` and set at least:

   ```sh
   # Point at the datasets you created, not at paths inside the container.
   HOST_BASE_DIRECTORY=/mnt/tank/apps/mineos/minecraft
   Data__Directory=/mnt/tank/apps/mineos/data

   # The address you will actually type in the browser.
   ORIGIN=http://truenas.local:3000
   WEB_ORIGIN_PROD=http://truenas.local:3000

   API_PORT=5078
   WEB_PORT=3000
   ```

3. Start it:

   ```sh
   docker compose up -d
   ```

The web UI is then on `WEB_PORT` (3000 by default).

## Option B — Custom app

If you prefer the TrueNAS app UI, add the two images as a custom app:

- **API** — `ghcr.io/freeman412/mineos-api:latest`, container port `5078`
- **Web** — `ghcr.io/freeman412/mineos-web:latest`, container port `3000`

Give the API container both host path mounts from the table above. The web
container needs no storage.

The web container must be able to reach the API container. Set on the web
container:

```
PRIVATE_API_BASE_URL=http://<api-container-name-or-ip>:5078
PRIVATE_API_KEY=<the same value as the API's ApiKey__SeedKey>
ORIGIN=http://truenas.local:3000
```

and on the API container, the same `Cors__AllowedOrigins__0` value as `ORIGIN`.
Compare `docker-compose.yml` for the full set — the custom-app route means
setting by hand everything compose would have set for you.

## TrueNAS-specific notes

### Ports below 1024, and ports TrueNAS already uses

The TrueNAS web interface occupies 80 and 443. Leave MineOS on its defaults
(3000 and 5078) unless you have a reason not to; if you change them, change
`WEB_PORT`/`API_PORT` **and** `ORIGIN` together, or login will bounce (see
[TROUBLESHOOTING](TROUBLESHOOTING.md)).

### Minecraft server ports

Each Minecraft server needs its own port published. The API container publishes
a range, `MC_PORT_RANGE` (default `25565-25570`) plus `BEDROCK_PORT_RANGE`
(default `19132-19137`, UDP). Widen those in `.env` if you run more servers than
the defaults cover. On a custom app, add the equivalent ports to the API
container's port list.

### LAN discovery

Minecraft's LAN server discovery needs host networking, which on TrueNAS means
giving the app host network mode. This disables Docker network isolation for
the containers, so only do it if you actually want servers to show up in the
Minecraft client's LAN list. Without it everything still works — clients just
connect by address.

### Dataset permissions

The API writes into `/var/games/minecraft` as the UID/GID configured by
`HostOptions.RunAsUid`/`RunAsGid` (1000/1000 by default). If the dataset is
owned by a different user, either change the dataset's ownership or set those
values, or server creation will fail with a permissions error.

### Behind the TrueNAS reverse proxy or Traefik

If you front MineOS with a proxy that terminates TLS, forward the original
scheme and host:

```
X-Forwarded-Proto: https
X-Forwarded-Host: <the hostname you type in the browser>
```

Without `X-Forwarded-Proto`, MineOS cannot tell that the browser is on HTTPS
and will issue the session cookie without the `Secure` flag.

## Migrating from the TrueNAS MineOS app

There is no automatic import from the Node.js MineOS's database — the two
projects store their settings differently. Server *data* is portable, though,
because both keep servers as ordinary directories:

1. Stop both applications.
2. Copy the old `servers/` (and `backup/`, `archive/` if you want them) into the
   dataset you mounted at `/var/games/minecraft`.
3. Start MineOS. Existing server directories are picked up on startup.

Check ownership after copying — see *Dataset permissions* above.
