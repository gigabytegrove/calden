# Calden — Locked Product Direction (2026-10-20)

## Product identity
Calden is a self-hosted, family-first calendar and household organizer. It must feel straightforward for ordinary families, not like software designed for systems administrators. Preserve the existing working application, stored data, permission model, calendar-first experience, and existing deployment and updater architecture. Develop incrementally, without replacing or regressing functioning features.

## Immediate completion gate: reminders and notifications
Finish the existing end-to-end notification and reminder work; it must not be deferred behind appearance or optional feature additions. Continue from the existing authenticated server-to-client WebSocket, persistent notification inbox, reminder scheduling, Monita household delivery, and Android WorkManager recovery rather than introducing a second architecture.

- Direct server-to-Android notification delivery without Firebase/FCM as the required delivery path, with native Android notifications and an explicitly user-visible foreground service when appropriate.
- Durable delivery, reconnect and catch-up, deduplication, authorization, assignment/event/recurrence/bill coverage, cancellation and quiet/permission handling.
- Show understandable, actionable warnings when Android notification permission, battery optimization, manufacturer background restrictions or connectivity may delay reminders. Do not mention Firebase, WebSockets or internal protocols in routine user warnings; technical details may appear in About or advanced documentation.
- Do not claim guaranteed instantaneous background delivery under Android restrictions. Retain periodic catch-up and validate with real Android devices and server, proxy, and end-to-end integration tests.
- Preserve separate personal reminders and optional Monita household notifications without duplicate or unauthorized messages.
- Do not close the notification issue until operational tests and actual Android build acceptance are complete.

## Family customization
Offer **Family settings** managed by an authorized household administrator and **My settings** for each person. Use one shared feature registry/capability model exposed through the server so native Android and web behave consistently. Server authorization always applies regardless of visibility. Turning a feature off hides its navigation/widgets/alerts as appropriate but **does not delete its data**; turning it on again restores the configured experience. Respect access to different calendars and household members.

Potential modular capabilities include Family Calendar (permanent core), Chores & Tasks, Bills & Money, Appointments, Reminders & Alerts, Meal Planning, and Shopping Lists. Additional modules should be real, complete integrations, never decorative switches or nonfunctional placeholders. Modules not actually implemented must not appear as available toggles.

The experience should be themable and customizable on web and native Android: shared theme tokens/presets, colors, light/dark appearance, legible typography, adaptable density, per-user home layout and widget choices where supported. Preserve accessible contrast, reduced motion and usable small-screen layout. Allow household defaults and individual overrides without silently changing existing users' preferences.

## Language and positioning
Present Calden as a **family calendar and household organizer**. Prefer common-language labels: "Reminders & alerts," "Who's responsible?", "Repeat this event," "Who can see or change this?", "We couldn't send this reminder." Keep technical settings in an explicitly advanced area. Friendly does not mean childish.

## RecipeCove meal-planning relationship
Meal Planning in Calden should optionally connect to **RecipeCove** (https://github.com/gigabytegrove/recipecove), rather than creating a competing full recipe database or replacing RecipeCove.

- Calden owns family calendar placement, date/time, household visibility, and meal schedule. RecipeCove owns recipes, ingredients, cooking instructions and recipe management.
- Allow an authorized user to connect an existing RecipeCove installation and choose recipes for breakfast/lunch/dinner (and custom meal slots) on calendar days; open the chosen recipe in RecipeCove when permitted.
- Use explicitly scoped/revocable credentials or a documented secure integration API; do not scrape UI, hard-code tokens, require shared databases, or assume endpoints exist before verifying/implementing both sides.
- Keep a stable external recipe identity and minimal presentation snapshot for useful historical schedules; define behavior when RecipeCove is offline, a recipe is deleted, or permission changes.
- Meal Planning must remain optional; Calden continues to work when RecipeCove is absent, unavailable, or disconnected. Show honest connection/error states without breaking calendars.
- Shopping-list/ingredient integration may follow as a separate scoped capability, with user approval before merging quantities or modifying existing lists; never imply RecipeCove currently supports an API not yet confirmed.
- Keep both products' established names and brands. Test compatibility against the actual RecipeCove API before release.

## Delivery discipline
Document and test module enable/disable persistence, administrator permissions, web/Android parity, themes, reminder delivery, upgrades and data preservation. Feature work should preserve existing release behavior. Product-direction approval is **not** a claim of implementation or production acceptance.
