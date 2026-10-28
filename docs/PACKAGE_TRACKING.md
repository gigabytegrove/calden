# Package Tracking (opt-in)

Calden's Package Tracking is **disabled by default** for existing and new households. An administrator enables it in Settings → Optional Features → Package Tracking. Disabling the feature hides Packages, stops email ingestion and hides shipment overlays without deleting shipment data.

## Working functions

- Track Amazon Logistics (TBA IDs), USPS, UPS, FedEx and other carriers manually. Package labels, status, expected day, notes and privacy are editable.
- Private-by-default ownership: an adult sees only their own packages and packages expressly shared by other adults. Only the owner can update or archive a package. Restricted and child accounts cannot modify shipments. Child accounts cannot access package endpoints.
- Each shipment has persistent, deduplicated tracking identity and status history. The calendar draws an overlay on its expected date, without creating an event or affecting recurrence/calendar permissions.
- Optional import of raw RFC 822 shipping emails from the UI (paste headers and body or upload an `.eml` file). Currently recognized IDs: UPS `1Z` numbers, USPS long numeric / international `US` numbers, Amazon Logistics `TBA` numbers and FedEx numeric IDs when FedEx is mentioned.
- Email imports are unverified suggestions: they never overwrite an existing tracking status, and they never represent a successful carrier API request.
- Owner-specific mail-gateway forwarding: generate a token in Packages → Import email. Calden stores only its SHA-256 digest and displays the raw value once. Configure an independently operated mail gateway to POST a raw RFC 822 email (up to 256 KiB) to `/api/packages/inbound-email` with `Authorization: Bearer cpk_...`. Rotating or revoking the token immediately invalidates the previous credential.
- **Built-in local tracking** in the single Calden Docker container runs Universal Parcel Scraper with Node and Chromium, identifying supported carriers and automatically retrieving actual scan locations, status and ETAs approximately every three hours, without a paid subscription, developer signup or API key. The child process receives an allowlisted environment, not Calden's database or authentication secrets. Public tracker fallbacks also receive the tracking number.

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

Calden **does not** automatically log in to Amazon, Informed Delivery, My Choice or FedEx Delivery Manager. The Docker build now includes a free, Apache-2.0 local carrier tracking engine with UPS, USPS, FedEx and other adapters and several keyless public aggregator fallbacks. No provider account, API key, paid service, or separate tracking container is required. Calden attempts real carrier scans when a package is created and repeats checks every three hours. These public pages often require JavaScript, browser checks or authentication; when no reliable status is readable, Calden reports that public tracking data is unavailable and links to the official carrier website. There is no unrestricted public Amazon Logistics TBA tracking API. An authorized AfterShip integration is available only as an optional advanced alternative. An independently managed mail service is required only if you choose HTTP forwarding instead of the built-in Gmail/approved-IMAP mailbox scanner. A POST endpoint is not itself an email server. Put the Calden endpoint behind HTTPS before routing any internet-originated mail, and never publish a token or full raw email in logs.

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
- **Delete is permanent**: removing a package deletes the owner's shipment and tracking history. Existing archived test records are cleared when the same owner re-adds that number.

## Limitations

Automatic **three-hour local carrier checks** are provided in the complete Docker installation; successful live tracking cannot be guaranteed where a carrier blocks automation, requires customer-specific data, or exposes no machine-readable history. An optional authorized AfterShip integration can add coverage but is not mandatory. Consumer Amazon OAuth, USPS Informed Delivery login, Gmail OAuth, Outlook OAuth, carrier-account scraping, and native Android package screens are not implemented. Manual or email-based data is never represented as carrier-verified.


## Built-in keyless local tracking (1.5.0-alpha.9)

The **Docker image** embeds the Apache-2.0 [Universal Parcel Scraper](https://github.com/plhery/universal-parcel-scraper) and Chromium/Node runtime in the **same Calden container**. Tracking numbers are detected and actual visible carrier scan events (including known locations, times, status, and tentative delivery estimates) are imported into the existing package timeline and calendar view. Multiple public tracker fallbacks are enabled, meaning tracking numbers may be sent to those public services as well as the carrier. Tracking remains globally off by default until enabled under Settings.

The bundled engine uses site adapters and browser/HTTP sources; carriers frequently change websites or block automated requests. It cannot guarantee that any arbitrary tracking number can be resolved, particularly Amazon TBA IDs and USPS/FedEx browser challenges. Calden preserves previous genuine scans rather than substituting fabricated statuses. **No subscription or developer account is required**, and you do not deploy a separate API service. Image download and outgoing HTTPS access to carrier sites are the only network requirements. **The alpha.9 Docker image is the complete bundled build; the native binary-only archive does not contain Node/browser dependencies and is not equivalent.**


### Alpha.9 tracking safeguards

The scanner uses the provider's real stage vocabulary, including registered, customs and ready-for-pickup. It records facility descriptions and scan timestamps only when the source provides a timezone, avoiding silently interpreting unzoned local clocks as UTC. It verifies any primary tracking number supplied by the adapter before applying a status; updates for an unrelated shipment are rejected. Carrier events remain deduplicated across three-hour updates. The Node subprocess receives a **strictly allowlisted environment**, excluding Calden's database, JWT and SMTP secrets. End-to-end integration tests simulate the actual Node CLI and verify PostgreSQL persistence without requiring paid credentials.

## Guided Packages workflow (v1.5.0-alpha.11)

A package requires only a **name and tracking number**. Advanced carrier/status/date controls are optional and collapsed, and the saved package is sent to Calden's local tracking flow. Owned packages can be permanently deleted from their row or editor after confirmation; other household members cannot delete them. When the Packages view is open, it refreshes automatically every minute to show newly discovered information.

**Connect email** defaults to Gmail and chooses `imap.gmail.com` automatically based on the entered Gmail email address. Create a dedicated app password from Google's [App Passwords](https://myaccount.google.com/apppasswords) page (when available) and paste it into Calden. The normal Google account password is not appropriate. Connections are stored encrypted, and enabled inboxes are polled approximately every 15 minutes without marking messages read. Admin-approved custom IMAP servers and HTTP gateway tokens remain under Advanced. This is **not Google OAuth** and it cannot automatically connect to Microsoft 365/Outlook OAuth-only inboxes.
