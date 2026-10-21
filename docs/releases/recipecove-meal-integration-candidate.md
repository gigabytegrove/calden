# Calden + RecipeCove meal planning — integrated development candidate

This work implements the **previously approved** RecipeCove meal-planning link. It uses an authoritative RecipeCove plan instead of adding a competing Calden database table. **Not yet a stable or production-tested release.**

## Available in source

- Calden web route `/meals` is accessible to adult members and managers, with a responsive recipe picker, scheduled meals, add/edit/delete, date window and explicit refresh.
- The Calden backend proxies six authenticated endpoints under `/api/integrations/recipecove/`. Both the website and the separate native Android client use these Calden endpoints, not RecipeCove credentials.
- Calden refuses direct access to child and restricted accounts. Integration credentials live only in the server environment.
- Existing RecipeCove meal edits appear in Calden when refreshed; Calden meal writes modify RecipeCove itself and appear there immediately. One authoritative record prevents silent two-database conflict/overwrite cycles.
- Safe request IDs protect retries. Responses are size-limited; outbound proxy follows no redirects, has a time limit, and requires HTTPS unless HTTP is explicitly enabled on trusted private networks.

## Deployment configuration

In Calden's server/Dokploy environment, set:
```
RECIPECOVE_URL=https://your-recipecove-host.example
RECIPECOVE_SYNC_KEY=fnk_REPLACE_WITH_A_NEW_MEALS_SYNC_KEY
# Only for intentionally trusted internal HTTP deployments:
# RECIPECOVE_ALLOW_HTTP=true
```
Create the key in the RecipeCove administrator UI using the new **Calden meal plan** scope. Do not use an existing published-recipes-only key. Do not commit real credentials to GitHub, expose them in HTML, or store them in the mobile app.

## Explicit scope/validation boundaries

- Approved meal planning with RecipeCove: included. Automatic shopping-list recipe ingredient import or calendar event materialization: **not** part of this implementation and not claimed.
- RecipeCove API/Calden Go test files and browser assertions are added. Browser syntax/route assertions can be executed in the assistant's isolated JavaScript harness; full pytest, Go/PostgreSQL, running browser and native Android acceptance are still required.
- Native Kotlin implementation uses Retrofit and Jetpack Compose, not WebView. Native parity still needs a compiled APK and real device testing.
- The integration is optional; an unconfigured deployment preserves Calden's previous behavior, aside from showing a connection prompt on the meal-planning page.

## Follow-up: meal-planning usability and concurrency hardening

The two-week view now provides forward/back navigation, date-grouped meals, accessible live range labels and a clear empty state. Meal entries and recipe options use DOM text nodes instead of HTML templates, preventing provider-controlled recipe names or notes from being interpreted as markup. Browser fetch sequence IDs prevent an older overlapping refresh from overwriting newer results. Writes disable competing actions, preserving request IDs for safe retries on failure. The UI clears records after a failed read instead of showing stale data and validates the provider's expected list payloads.

This remains an unreleased candidate; server and browser integration tests must pass before production use.
