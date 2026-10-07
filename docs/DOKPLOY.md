# Deploying CalDen with Dokploy or Raw Docker Compose

CalDen supports both a normal Git deployment and a Docker Raw deployment.

## Recommended: Git deployment

Point Dokploy at:

```text
https://github.com/gigabytegrove/calden
```

Use the repository's normal Compose file:

```text
compose.yaml
```

That file builds from the checked-out source with `build: .`.

## Docker Raw deployment

If Dokploy is configured as **Docker Raw** and only accepts pasted Compose, use:

```text
docker-compose.dokploy.yml
```

Do **not** paste `compose.yaml` into Docker Raw. Its `build: .` context expects the repository to already exist on disk.

The Docker Raw file builds CalDen directly from:

```text
https://github.com/gigabytegrove/calden.git#main
```

No GitHub Container Registry login is required.

## Environment

A new installation requires no environment variables.

CalDen generates a private PostgreSQL password on first start and stores it in the persistent `calden-secrets` volume.

Optional settings:

```env
CALDEN_PORT=8787
CALDEN_DB_NAME=calden
CALDEN_DB_USER=calden
```

Only set `CALDEN_DB_PASSWORD` when migrating an existing installation that already uses a manually configured database password.

The external browser port can be changed with `CALDEN_PORT`. The CalDen container itself always listens on port `8787`.

## Persistent storage

All three named volumes are part of a complete installation:

```text
calden-db
calden-data
calden-secrets
```

- `calden-db` contains PostgreSQL data.
- `calden-data` contains CalDen application data.
- `calden-secrets` contains the generated PostgreSQL password.

Back up all three. Do not remove them during a normal update.

## First launch

After PostgreSQL and CalDen report healthy, open:

```text
http://YOUR-SERVER-IP:8787
```

or the HTTPS hostname configured in your reverse proxy.

The first-run setup creates the household and first administrator account. CalDen does not ship with a default application password.

## Updating

For a Git deployment, pull the current branch and rebuild:

```bash
git pull --ff-only
docker compose up -d --build
```

For Docker Raw, redeploy the application. The Docker build context is the current public `main` branch.

CalDen runs database migrations automatically on startup.

## Reverse proxy

Route the CalDen hostname to the CalDen service on port `8787`.

PostgreSQL should remain internal to the Compose network and should not be published to the internet.

## Troubleshooting

### `open Dockerfile: no such file or directory`

A local-source Compose file was used in a Docker Raw deployment. Use `docker-compose.dokploy.yml`.

### `unauthorized` or `denied` from `ghcr.io`

Current Docker Raw deployment does not require GHCR. Replace an older image-based Compose definition with `docker-compose.dokploy.yml` and redeploy.

### Database password

New installs generate the password automatically. Existing installs with a manual password should set `CALDEN_DB_PASSWORD` to the existing password before first start with the current Compose stack.

### Health checks

Use:

```bash
docker compose ps
docker compose logs db
docker compose logs calden
```

CalDen's HTTP health endpoint is:

```text
/api/health
```
