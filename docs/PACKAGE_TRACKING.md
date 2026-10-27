# Package Tracking (opt-in)

Calden's Package Tracking is **disabled by default** for existing and new households. An administrator enables it in Settings → Optional Features → Package Tracking. Disabling the feature hides Packages, stops email ingestion and hides shipment overlays without deleting shipment data.

## Working functions

- Track Amazon Logistics (TBA IDs), USPS, UPS, FedEx and other carriers manually. Package labels, status, expected day, notes and privacy are editable.
- Private-by-default ownership: an adult sees only their own packages and packages expressly shared by other adults. Only the owner can update or archive a package. Restricted and child accounts cannot modify shipments. Child accounts cannot access package endpoints.
- Each shipment has persistent, deduplicated tracking identity and status history. The calendar draws an overlay on its expected date, without creating an event or affecting recurrence/calendar permissions.
- Optional import of raw RFC 822 shipping emails from the UI (paste headers and body or upload an `.eml` file). Currently recognized IDs: UPS `1Z` numbers, USPS long numeric / international `US` numbers, Amazon Logistics `TBA` numbers and FedEx numeric IDs when FedEx is mentioned.
- Email imports are unverified suggestions: they never overwrite an existing tracking status, and they never represent a successful carrier API request.
- Owner-specific mail-gateway forwarding: generate a token in Packages → Import email. Calden stores only its SHA-256 digest and displays the raw value once. Configure an independently operated mail gateway to POST a raw RFC 822 email (up to 256 KiB) to `/api/packages/inbound-email` with `Authorization: Bearer cpk_...`. Rotating or revoking the token immediately invalidates the previous credential.
- **Included free local tracking engine in Docker:** Calden bundles `universal-parcel-scraper`, Node.js 26, and Chromium into the existing **single Calden container**. Users need no subscription, developer API key, sign-up, extra container, or external tracking service to configure. Carrier IDs are recognized and checked approximately every three hours. Available carrier scan history, current status, facility/location and estimated delivery date are saved directly in PostgreSQL. The source engine is Apache-2.0 with attribution and bundled NOTICE.
- When a carrier's accessible website does not provide shipment events, Calden retains its last verified scan and retries on schedule. The native Go-only binary archives **do not** currently include a Node/browser runtime; the fully equipped distribution is the Docker image.
- The engine may consult public third-party aggregators as fallbacks (ParcelsApp, Ship24, 17TRACK, Postal Ninja, UPU); this transmits the tracking number to those sources without a paid subscription or account. The app operator can restrict outbound access for privacy. These data sources are not guaranteed by Calden.

## Built-in mailbox scanning (opt-in)

Packages → Import email → **Automatically discover packages from your inbox** supports **Gmail IMAP with an app password** and other IMAP hosts explicitly approved by the Calden server operator. No carrier usernames or passwords are collected. Gmail recommends OAuth and an app password is only an alternative when 2-Step Verification is enabled and Google allows app-password generation; if unavailable, use the existing mail-gateway forwarding mode.

1. Enable Packages under Settings → Optional Features.
2. Open Packages → Import email, enter `imap.gmail.com`, your Gmail address, and a **dedicated app password**. Select “Automatically check this inbox,” then save. The password is encrypted at rest, never returned by GET, and never logged.
3. Calden's background worker checks the inbox over verified TLS (port 993) roughly every 15 minutes. The first run inspects at most 40 recent emails within the previous 14 days; later runs resume using UID and UIDVALIDITY, without marking messages as read.
4. “Check now” schedules a scan; Refresh status reports whether a scan completed. Turning the mailbox off pauses scanning. **Disconnect** deletes the saved credential and cursor, not the imported packages.

Only read-only IMAP commands (`EXAMINE`, `UID SEARCH`, `UID FETCH BODY.PEEK[]`) are sent after TLS authentication. Individual fetched messages are limited to 256 KiB and raw email is not retained in the database. Connections have fixed timeouts, inbox scans are bounded, and a global Packages disable blocks scheduling and persistence.

Custom IMAP servers are not accepted by default; the operator must set `CALDEN_PACKAGE_IMAP_ALLOWED_HOSTS=imap.example.com,imap2.example.com` in the server environment. This prevents regular users from directing the app at arbitrary internal services. All supported hosts must provide valid TLS certificates on TCP 993. Outlook.com and Microsoft 365 require modern OAuth authentication and **do not work with username/password IMAP** in this implementation; use email forwarding until OAuth is added.

If a shipping email says, for example, “Estimated delivery: 2026-10-30,” Calden will attach that tentative date to **newly imported** packages and display them on the calendar. Dates extracted from email are unverified estimates. Existing manually maintained packages, including their status and dates, are never overwritten by incoming email.

## External setup required

Calden **does not** automatically log in to Amazon, Informed Delivery, My Choice or FedEx Delivery Manager. The Docker release includes a local, free, credentialless browser/HTTP tracking engine and several public aggregator fallbacks. Most eligible carrier shipments can be attempted without configuration. The carrier-specific result depends on public accessibility and may fail if the carrier restricts automated retrieval or requires recipient-specific information. In particular, Amazon Logistics TBA identifiers **may not return tracking events** without a signed-in order. There is no unrestricted Amazon TBA API. An authorized AfterShip integration remains available internally as an optional legacy advanced alternative, not part of the normal interface. An independently managed mail service is required only if you choose HTTP forwarding instead of the built-in Gmail/approved-IMAP mailbox scanner. A POST endpoint is not itself an email server. Put the Calden endpoint behind HTTPS before routing any internet-originated mail, and never publish a token or full raw email in logs.

The feature is useful without any external credentials: paste tracking IDs, save estimated dates, and use the official carrier tracking links.

## Server endpoints

All endpoints below require a normal authenticated adult session, the global packages feature switch, and ownership permissions, except inbound email:

- `GET/POST /api/packages`
- `GET/PUT/DELETE /api/packages/{id}`
- `GET /api/packages/{id}/history`
- `POST /api/packages/import-email` with `{"raw_email":"From: ...\\nSubject: ...\\n\\n..."}`
- `GET/POST/DELETE /api/packages/email-token`
- `POST /api/packages/inbound-email` — raw MIME body and per-user token, not a session JWT.
- `GET/PUT/DELETE /api/packages/mailbox` — configure, inspect or disconnect your encrypted mailbox.
- `POST /api/packages/mailbox/check` — request a bounded background inbox scan.

The import handler does not load embedded URLs, fetch untrusted images, save email attachments, or persist original raw messages. Inactive users, revoked tokens and disabled features cannot import.

## Operational details

- Migration `041_package_tracking.sql` adds shipment and checkpoint storage. Migration `042_package_inbound_tokens.sql` adds hashed per-user ingress credentials. Migration `043_package_mailboxes.sql` stores encrypted mailbox settings and UID cursors.
- Neither migration changes regular calendar records. Existing data and UI are preserved when Packages is disabled.
- Delivery dates are user-supplied dates (not guaranteed carrier estimates). Updating the date moves the calendar overlay automatically.
- Archive is a soft archive; past records remain in PostgreSQL.

## Limitations

Automatic **three-hour keyless local carrier checks** are provided by the complete Docker installation. Carrier adapters and free public fallbacks can fail or require additional recipient details; successful live tracking for every carrier cannot be guaranteed. An optional authorized AfterShip integration can add coverage but is not mandatory. Consumer Amazon OAuth, USPS Informed Delivery login, Gmail OAuth, Outlook OAuth, carrier-account scraping, and native Android package screens are not implemented. Manual or email-based data is never represented as carrier-verified.


## Third-party attribution

Calden's complete Docker image includes [Universal Parcel Scraper](https://github.com/plhery/universal-parcel-scraper), copyright its respective authors, distributed under Apache License 2.0. The included npm package retains its LICENSE and NOTICE files and any additional attributions. Browser transport runs as a subprocess inside the existing Calden container, not as a network-exposed sidecar. Its child environment does **not** inherit Calden database, JWT, email, or other service credentials. Public lookup behavior and adapter support may change when carriers update their websites.
