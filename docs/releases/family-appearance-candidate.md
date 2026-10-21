# Calden household themes, module labels and safe icons — development candidate

Status: additive, unreleased source candidate. This is part of the approved family customization requirements; it does not change module IDs, permissions, calendars, routing or data.

## Delivered source

- One administrator-managed household theme selection: Classic, Ocean, Forest, Sunset or Charcoal.
- Custom names and icons for thirteen existing Calden modules. Every module maintains its canonical ID regardless of display name. A disabled module remains disabled.
- Browser-wide theme accent tokens, controlled navigation labels and accessible fixed SVG icon shapes. No user-provided HTML, SVG, scripts or font files are accepted.
- Optional individual appearance, layout and widget preferences remain in the original personal dashboard preference API and are never overwritten by family theme changes.
- Non-destructive migration 035 records household labels/icons and theme; the new data belongs in the existing PostgreSQL backup/restore system.
- Authenticated GET for all household members including children, administrator-only PUT. Default names/icons are returned for modules not yet customized.

## Endpoints

`GET /api/family/appearance` returns `{"theme":"classic","modules":[{"module_id":"calendar","label":"Calendar","icon":"calendar"},...]}`.
`PUT /api/family/appearance` accepts a complete validated array with the same format. Unsupported IDs, duplicate IDs, unsafe icon names, unknown themes and empty/long labels are rejected atomically.

## Verification / nonclaims

- Checked JavaScript syntax and client source assertions for route wiring, safe labeling and icon catalogs.
- Go validation tests are included in source but a full compiled Go test run is NOT verified.
- The native Android source is not part of the current Calden repository. Android theme-token parity remains an acceptance requirement and is NOT claimed here.
- Accessibility/contrast checks in actual browser/Android combinations and database upgrade/restore are pending.
- Do not release stable solely from source-level checks.
