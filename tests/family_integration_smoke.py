#!/usr/bin/env python3
"""Integration acceptance against a disposable, already configured Calden server.

Runs in the existing Docker smoke test, after the installed-instance restore test.
Does not require any additional Python packages.
"""
import datetime
import json
import os
import urllib.error
import urllib.parse
import urllib.request
import uuid
from zoneinfo import ZoneInfo

BASE = os.environ.get("CALDEN_TEST_BASE", "http://127.0.0.1:8787").rstrip("/")
USERNAME = os.environ.get("CALDEN_TEST_USER", "testadmin")
PASSWORD = os.environ.get("CALDEN_TEST_PASSWORD", "test-password-123")

def request(method, path, payload=None, token=None, status=200):
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None
    if payload is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(payload).encode()
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            actual = response.status
            raw = response.read()
    except urllib.error.HTTPError as error:
        actual = error.code
        raw = error.read()
    if actual != status:
        raise AssertionError(f"{method} {path}: HTTP {actual}, expected {status}: {raw[:500]!r}")
    return json.loads(raw) if raw else None

def check(condition, message):
    if not condition:
        raise AssertionError(message)

def main():
    admin = request("POST", "/api/login", {"username": USERNAME, "password": PASSWORD})["token"]
    original_features = request("GET", "/api/household/features", token=admin)["features"]
    original_appearance = request("GET", "/api/household/appearance", token=admin)
    original_dashboard = request("GET", "/api/me/dashboard", token=admin)
    records = []
    member = None
    today = datetime.datetime.now(ZoneInfo("America/New_York")).date()
    today_text = today.isoformat()
    tomorrow = (today + datetime.timedelta(days=1)).isoformat()
    sections = ["shopping", "school", "maintenance", "pets", "routines"]
    try:
        updated = request("PUT", "/api/household/features",
                          {"features": {s: True for s in sections}}, admin)
        check(all(updated["features"][s] for s in sections), "feature toggles must persist")

        appearance = {
            "preset": "forest",
            "labels": {"shopping": "Groceries", "pets": "Our pets"},
            "icons": {"shopping": "shopping-cart", "pets": "heart"},
        }
        check(request("PUT", "/api/household/appearance", appearance, admin)["preset"] == "forest",
              "household theme update failed")
        check(request("GET", "/api/household/appearance", token=admin)["labels"]["shopping"] == "Groceries",
              "custom label did not persist")

        preferences = {"appearance": "dark", "layout": "simple",
                       "widgets": ["calendar", "notifications", "shopping"]}
        request("PUT", "/api/me/dashboard", preferences, admin)
        check(request("GET", "/api/me/dashboard", token=admin) == preferences,
              "dashboard settings failed to persist")

        for section, title in [("shopping", "Milk"), ("school", "Science project"),
                               ("maintenance", "Replace filter"), ("pets", "Vet visit"),
                               ("routines", "Morning checklist")]:
            private = section == "pets"
            payload = {
                "title": title, "notes": "Calden smoke test", "subject": "Family",
                "list_name": "Test", "quantity": 2, "unit": "items",
                "due_on": today_text, "recurrence": "daily" if section == "routines" else "none",
                "private": private,
            }
            created = request("POST", f"/api/household/{section}", payload, admin, status=201)
            records.append((section, created["id"]))
            listed = request("GET", f"/api/household/{section}?on={today_text}", token=admin)["items"]
            check(any(x["id"] == created["id"] and x["title"] == title for x in listed),
                  f"{section} create/read failed")

        suffix = uuid.uuid4().hex[:10]
        login = "smoke" + suffix
        password = "smoke-pass-123"
        user = request("POST", "/api/users", {
            "username": login, "display_name": "Smoke restricted member",
            "password": password, "role": "restricted"
        }, admin, status=201)
        member = user["id"]
        child = request("POST", "/api/login", {"username": login, "password": password})["token"]
        pet_records = request("GET", f"/api/household/pets?on={today_text}", token=child)["items"]
        check(not pet_records, "private pet record was disclosed to restricted member")
        shopping = request("GET", f"/api/household/shopping?on={today_text}", token=child)["items"]
        check(any(x["title"] == "Milk" for x in shopping), "shared shopping list not visible")
        request("PUT", "/api/household/features", {"features": {"shopping": False}}, child, status=403)
        request("PUT", "/api/household/appearance", appearance, child, status=403)
        request("DELETE", f"/api/household/shopping/{records[0][1]}", token=child, status=404)
        # Child's own dashboard must not replace the administrator's preferences.
        own = {"appearance": "light", "layout": "simple", "widgets": ["calendar"]}
        request("PUT", "/api/me/dashboard", own, child)
        check(request("GET", "/api/me/dashboard", token=child) == own,
              "restricted member dashboard did not persist")
        check(request("GET", "/api/me/dashboard", token=admin) == preferences,
              "one member's preferences changed another member")

        item_id = records[0][1]
        request("PUT", f"/api/household/shopping/{item_id}/completion",
                {"on": today_text, "done": True}, child)
        request("PUT", f"/api/household/shopping/{item_id}/completion",
                {"on": today_text, "done": True}, child)
        listed = request("GET", f"/api/household/shopping?on={today_text}", token=admin)["items"]
        check(next(x for x in listed if x["id"] == item_id)["done"],
              "shared completion was not durable and visible")

        routine_id = next(i for s, i in records if s == "routines")
        request("PUT", f"/api/household/routines/{routine_id}/completion",
                {"on": today_text, "done": True}, admin)
        current = request("GET", f"/api/household/routines?on={today_text}", token=admin)["items"]
        future = request("GET", f"/api/household/routines?on={tomorrow}", token=admin)["items"]
        check(next(x for x in current if x["id"] == routine_id)["done"],
              "today's routine completion is missing")
        check(not next(x for x in future if x["id"] == routine_id)["done"],
              "recurring completion incorrectly completed a future occurrence")

        request("PUT", "/api/household/features", {"features": {"shopping": False}}, admin)
        request("GET", "/api/household/shopping", token=admin, status=403)
        request("PUT", "/api/household/features", {"features": {"shopping": True}}, admin)
        restored = request("GET", f"/api/household/shopping?on={today_text}", token=admin)["items"]
        check(any(x["id"] == item_id for x in restored),
              "turning feature off/on deleted saved data")
        print("PASS: live family API acceptance (modules, permissions, privacy, recurrence, settings)")
    finally:
        # Disposable Docker smoke deployment: restore feature/personal settings for
        # subsequent checks, regardless of individual assertion results.
        for section, item_id in records:
            try:
                request("DELETE", f"/api/household/{section}/{item_id}", token=admin, status=204)
            except Exception:
                pass
        if member:
            try:
                request("DELETE", f"/api/users/{member}", token=admin, status=204)
            except Exception:
                pass
        request("PUT", "/api/household/appearance", original_appearance, admin)
        request("PUT", "/api/me/dashboard", original_dashboard, admin)
        request("PUT", "/api/household/features", {"features": original_features}, admin)

if __name__ == "__main__":
    main()
