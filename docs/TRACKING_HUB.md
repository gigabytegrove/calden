# Calden Tracking Hub — one provider account for every installation

Calden Tracking Hub is a deployable server component for the **Calden operator**. Each Calden household continues to self-host its own calendar, accounts and package records. The operator maintains a single authorized AfterShip Tracking API subscription behind HTTPS; individual households do **not** create AfterShip accounts or enter provider credentials.

**This is not a built-in free public carrier API.** Reliable carrier data requires a paid, authorized tracking-data source. The central hub consolidates that requirement for the Calden project.

## Architecture

1. A household enables optional Package Tracking (still disabled by default). Calden generates a pseudonymous, stable 256-bit HMAC credential from its **existing persistent instance secret**. It does not share user names, household identifiers, or the source secret.
2. Calden's local server calls `https://YOUR-HUB/v1/installations` to enroll. The hub hashes the credential in PostgreSQL; raw installation credentials are not persisted. Enrollment is controlled by the operator.
3. Calden sends a parcel's tracking number and approximate carrier to `POST /v1/trackings` with the installation bearer credential. Only Calden's **server** accesses the hub, not browsers.
4. The hub creates or retrieves an official AfterShip tracking record. It keeps the provider key server-side, caches responses for three hours and returns a restricted tracking payload. Updates flow into Calden's existing package status, checkpoints, history and calendar projection.
5. Calden's current background worker schedules the request every three hours. Delivered and archived shipments are not polled. The hub independently enforces the three-hour cache, a maximum shipment count per installation, global and per-installation daily request budgets, and maximum enrollments per day.

There is **no credential in Calden source code, Docker images, GitHub releases, static UI, or update artifacts**.

## One-time operator deployment

An authorized **AfterShip Tracking Premium or higher** plan and a working API key are required. Verify the right to operate your specific multi-household application under the [AfterShip API Agreement](https://www.aftership.com/terms/api) and [Terms of Service](https://www.aftership.com/legal/terms-of-service). Section 3.3 permits processing on behalf of recipients, but sections 3.2, 4.3 and 4.4 place credential, policy and downstream-app obligations on developers; seek provider confirmation for publicly distributed open-source builds.

Deploy this hub once, **not inside every Calden installation**:

```sh
mkdir -p tracking-hub-secrets
chmod 700 tracking-hub-secrets
umask 077
DB_PASSWORD="$(openssl rand -hex 28)"
printf '%s' "$DB_PASSWORD" > tracking-hub-secrets/db-password
printf 'postgres://trackinghub:%s@hub-db:5432/trackinghub?sslmode=disable' "$DB_PASSWORD" > tracking-hub-secrets/database-url
printf 'Paste the AfterShip API key: '
stty -echo
IFS= read -r AFTERSHIP_KEY
stty echo
printf '\n'
printf '%s' "$AFTERSHIP_KEY" > tracking-hub-secrets/aftership-key
unset AFTERSHIP_KEY DB_PASSWORD
chmod 600 tracking-hub-secrets/*
CALDEN_HUB_PUBLIC_ENROLLMENT=true docker compose -f compose.tracking-hub.yaml up -d
```

The listener binds to **127.0.0.1:8790**. Proxy it through your HTTPS reverse proxy (for example, an operator-controlled `https://tracking.example.net`), with modern TLS, request-size controls, request throttling and abuse monitoring. Do not expose 8790 directly to the internet. If the reverse proxy is on another Docker host, place the hub on a private trusted network rather than publishing an unrestricted port.

For each Calden installation image, configure its server-side environment variable:

```yaml
CALDEN_TRACKING_HUB_URL: https://tracking.example.net
```

The **publisher/operator** can set that default in the official Calden distribution once the hub is deployed and verified, so end users do not configure anything. The repository does not invent or preconfigure a public hostname that does not exist.

After setup, a Calden installation displays **Managed carrier tracking** on its Packages page. The user enters a tracking number; Calden registers the shipment and refreshes actual carrier data on the normal three-hour schedule.

## Operational and security rules

- Enrollments are **disabled by default** on the hub; set `CALDEN_HUB_PUBLIC_ENROLLMENT=true` deliberately. Anonymous public enrollment has inherent abuse risk even with quotas. Do not enable open enrollment until you have carrier billing limits, monitoring, a clear privacy notice, and a revocation process. For large public distribution, put a paid/licensed enrollment entitlement in front of enrollment.
- Default quotas: **100 new installations/day**, **50 distinct parcels/installation**, **400 API calls/installation/day** and **5,000 calls/day globally**. Configure with `CALDEN_HUB_MAX_INSTALLS_DAILY`, `CALDEN_HUB_MAX_SHIPMENTS_PER_INSTALL`, `CALDEN_HUB_MAX_API_CALLS_PER_INSTALL`, `CALDEN_HUB_MAX_API_CALLS_DAILY`. These are ceilings, not guarantees of AfterShip plan capacity or cost.
- The hub keeps installation-specific snapshots separate; it does not return another installation's record, even when tracking numbers match. Shipment snapshots are deleted after **45 days of inactivity** by the hub's daily cleanup.
- The operator must safeguard tracking IDs, addresses and scan messages as personal data. Provide notice that tracking numbers and carrier events are processed by a centrally operated service and AfterShip. Calden's package sharing preference only controls **local household visibility**.
- No user passwords or carrier login credentials are collected, and the hub never shares its AfterShip key.
- A hub outage, exhausted quota or unsupported carrier returns an error, **not an invented status**. The service requires actual API access and an actively deployed operator hub. A separate public-page mode remains available only if no hub is configured.

## Readiness criteria before public rollout

The GitHub workflow `Calden Central Tracking Hub — Secure Integration` must pass PostgreSQL multi-installation isolation, credential-hashing, provider-key non-disclosure, rate-budget, three-hour cache, Calden client API, existing web regressions and Docker build. Then validate with a real authorized AfterShip account using live UPS, USPS and FedEx shipments, confirm supported courier slug requirements, monitor usage and billing, and publish the operator-hosted HTTPS endpoint into the official Calden configuration.

**Until a real hub is deployed and tested with an authorized AfterShip key, shipping the hub code alone does not give every Calden user live tracking.**
