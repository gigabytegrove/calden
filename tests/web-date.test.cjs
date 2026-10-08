"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const source = fs.readFileSync(path.resolve(__dirname, "../web/app.js"), "utf8");
const first = source.indexOf("function dateInputValue(");
const last = source.indexOf("function eventStartDate(", first);
assert.ok(first >= 0 && last > first, "Could not locate production date helpers");
const { dateInputValue, calendarDate } = vm.runInNewContext(
  source.slice(first, last) + "\n({dateInputValue,calendarDate})",
  { Date, Number, String }
);

test("date-only bill/payment dates remain unchanged in US Eastern timezone", () => {
  const before = process.env.TZ;
  try {
    process.env.TZ = "America/New_York";
    assert.equal(dateInputValue("2026-10-07"), "2026-10-07");
    assert.equal(dateInputValue("2026-01-01"), "2026-01-01");
    assert.equal(dateInputValue("2026-12-31"), "2026-12-31");
  } finally {
    process.env.TZ = before;
  }
});

test("date-only strings do not shift in western time zones", () => {
  const before = process.env.TZ;
  try {
    for (const zone of ["America/Los_Angeles", "Pacific/Honolulu"]) {
      process.env.TZ = zone;
      assert.equal(dateInputValue("2026-10-07"), "2026-10-07", zone);
      assert.equal(calendarDate("2026-10-07").getDate(), 7, zone);
    }
  } finally {
    process.env.TZ = before;
  }
});

test("invalid dates do not generate NaN form values", () => {
  assert.equal(dateInputValue("not-a-date"), "");
});

test("timestamp inputs remain converted to local calendar date", () => {
  const before = process.env.TZ;
  try {
    process.env.TZ = "America/New_York";
    assert.equal(dateInputValue("2026-10-07T12:00:00Z"), "2026-10-07");
  } finally {
    process.env.TZ = before;
  }
});
