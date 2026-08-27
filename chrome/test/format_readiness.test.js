const test = require("node:test");
const assert = require("node:assert/strict");
require("../lib/gss.js");
require("../lib/parse.js");
require("../lib/format.js");
const gss = require("../lib/readiness.js");

function pr(partial) {
  return Object.assign({
    number: 1,
    draft: false,
    state: "OPEN",
    rebase: { status: "UP_TO_DATE" },
    checks: { state: "SUCCESS", passed: 2, failed: 0, pending: 0, skipped: 0, failedNames: [] },
    reviews: { decision: "APPROVED", approved: ["alice"], changesRequested: [], pending: [] },
  }, partial);
}

test("formatChecks matches CLI short output", () => {
  assert.equal(gss.formatChecks({ state: "SUCCESS", passed: 12, failed: 0, pending: 0, skipped: 0, failedNames: [] }).text, "pass (12/12)");
  assert.equal(gss.formatChecks({ state: "FAILURE", passed: 7, failed: 1, pending: 0, skipped: 0, failedNames: ["lint"] }).text, "fail (lint)");
  assert.equal(gss.formatChecks({ state: "PENDING", passed: 3, failed: 0, pending: 5, skipped: 0, failedNames: [] }).text, "pending (3/8)");
  assert.equal(gss.formatChecks({ state: "", passed: 0, failed: 0, pending: 0, skipped: 0, failedNames: [] }).text, "—");
});

test("bottom not-ready blocks everything upstack", () => {
  const prs = [
    pr({ number: 17740, reviews: { decision: "REVIEW_REQUIRED", pending: ["bob"] } }),
    pr({ number: 17744 }),
    pr({ number: 17745 }),
    pr({ number: 17746 }),
    pr({ number: 17747 }),
  ];
  assert.equal(gss.overallStatus(prs, 0).key, "not-ready");
  assert.equal(gss.overallStatus(prs, 0).label, "Not ready");
  assert.equal(gss.overallStatus(prs, 1).key, "blocked-downstack");
  assert.equal(gss.overallStatus(prs, 4).label, "Blocked downstack");
});

test("ready stack is ready throughout", () => {
  const prs = [pr({ number: 1 }), pr({ number: 2 })];
  assert.equal(gss.overallStatus(prs, 0).key, "ready");
  assert.equal(gss.overallStatus(prs, 1).key, "ready");
});

test("behind or failing checks are not ready even if approved", () => {
  assert.equal(gss.layerReady(pr({ rebase: { status: "BEHIND" } })), false);
  assert.equal(gss.layerReady(pr({ checks: { state: "FAILURE", passed: 0, failed: 1, pending: 0, skipped: 0, failedNames: ["lint"] } })), false);
  assert.equal(gss.layerReady(pr({ draft: true })), false);
});

test("annotateStack attaches formatted fields", () => {
  const annotated = gss.annotateStack({
    pullRequests: [
      pr({ number: 101, reviews: { decision: "REVIEW_REQUIRED", pending: ["carol"] } }),
      pr({ number: 102 }),
    ],
  });
  assert.equal(annotated[0].overall.key, "not-ready");
  assert.equal(annotated[1].overall.key, "blocked-downstack");
  assert.match(annotated[0].reviewsFmt.text, /review required/);
});
