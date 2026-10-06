# Deploying CalDen with Dokploy or Raw Docker Compose

Use this guide when the deployment platform accepts a Docker Compose file but does not check out the CalDen Git repository into the deployment directory.

## Use the image-based example

Use the repository file:

```text
docker-compose.example.yml
```

Do **not** use a Compose file containing:

```yaml
build: .
```

in a raw Compose deployment. That form expects the CalDen source tree and Dockerfile to already exist in the deployment directory.

The raw deployment example uses:

```yaml
image: ghcr.io/gigabytegrove/calden:latest
```

instead.

## Environment

Set at least:

```env
CALDEN_DB_PASSWORD=CHANGE-ME-TO-A-LONG-RANDOM-PASSWORD
```

Optional:

```env
CALDEN_PORT=8787
CALDEN_DB_NAME=calden
CALDEN_DB_USER=calden
```

The external port can be changed with `CALDEN_PORT`. The CalDen container itself always listens on port `8787`.

## Persistent storage

The example creates:

```text
calden-db
calden-data
```

Both volumes should be included in backups.

## First launch

After both services report healthy, open:

```text
http://YOUR-SERVER-IP:8787
```

or the hostname configured in your reverse proxy.

The first-run screen creates the initial CalDen administrator. CalDen does not ship with a default application password.

## Registry authentication

If the CalDen container is not yet publicly readable from GitHub Container Registry, add `ghcr.io` as a registry in your deployment platform using GitHub credentials with package read access.

A registry error looks different from a source-build error:

```text
denied
unauthorized
```

A source-build error looks like:

```text
open Dockerfile: no such file or directory
```

The latter means the wrong Compose style was selected.

## Reverse proxy

Route your CalDen hostname to the CalDen service on port `8787`.

PostgreSQL should remain internal to the Compose network and should not be published through the reverse proxy or directly to the internet.
