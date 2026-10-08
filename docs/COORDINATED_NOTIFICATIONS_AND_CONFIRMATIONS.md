# CalDen coordinated development: device notifications, confirmations, integrations

Status: DESIGN BASELINE (not implemented)
Date: 2026-10-08

## Accepted baseline
- Android 1.2.5 is the user-accepted baseline; preserve calendar display, widgets, avatar priority (photo always wins over initials), bill status conventions, and working login.
- CalDen server/web is authoritative for events, membership, reminder preferences, and event confirmation.
- Coordinate changes across gigabytegrove/calden and gigabytegrove/calden-android; Monita lives at gigabytegrove/monita (default branch master).
- No feature removal or untested release claims.

## Device enrollment
- Each authenticated household user registers their Android device with explicit notification permission and a revocable device record.
- Registry should store random device ID, user ID, platform, human-readable device label, push transport token when available (encrypted at rest), created/last-seen timestamps, enabled state and capability metadata.
- Device registration must never send user passwords to push providers. Rotating or revoking a token must be supported.
- Allow users to see and revoke own devices; admins can manage devices per established roles.
- API candidates: POST /api/devices, GET /api/devices, PATCH /api/devices/{id}, DELETE /api/devices/{id}; POST /api/devices/{id}/token.

## Notification model
- Persist durable notification records and delivery attempts for audit/deduplication.
- Separate event semantics (invitation, upcoming reminder, change request, schedule modification) from transport (native device, Monita, ntfy, Gotify, SMTP, webhook).
- Native Android should support informational, reminders, important alerts and actionable confirmation/change-request notifications, subject to OS permissions/channels.
- Schedule according to server timezone/user preference and individual reminder offsets. Deduplicate by event occurrence + recipient + reminder offset + delivery type.
- Push transport implementation choice must be confirmed; consider FCM for reliable background delivery; do not claim remote push exists before provisioning.

## Event confirmations
- Event creator can select 'Request confirmation'; recipients see event provisionally.
- Per-user/per-occurrence response: pending, confirmed, change_requested.
- 'Request Change' includes optional reason and notifies original scheduler. Does not delete event or modify others' responses.
- Changes to event time/assignees should invalidate affected confirmations where appropriate and re-notify.
- Blocked-off work periods should be available to conflict detection without exposing private event details unnecessarily.
- APIs and schema migrations must be additive and backward compatible, with tests for recurring series/occurrences.

## Integrations
- Native device notifications function without external providers.
- Monita: 'Connect to Monita' auth flow and accessible channel picker; prefer scoped delegated authorization, not stored admin password. Validate actual Monita implementation before choosing OAuth/OIDC or API keys.
- ntfy: topic and optional URL, auth and priority support.
- Other adapters: Gotify, SMTP email, webhook, optionally Discord.
- Connection testing, secure credential encryption, retry/backoff, per-provider delivery logs and channel/topic routing.

## Implementation gates
1. Confirm actual API/schema and Monita authentication/channel contracts.
2. Add migrations + documented APIs and tests on server.
3. Add web settings, request confirmation and conflict response UI.
4. Add Android device enrollment, notification channels, response screens and deep links.
5. Integrate optional providers only after native notifications are coherent.
6. Build/test both applications; never rename a ZIP as APK or claim an unbuilt binary is verified.
