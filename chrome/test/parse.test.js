const test = require("node:test");
const assert = require("node:assert/strict");
require("../lib/gss.js");
require("../lib/parse.js");
const gss = require("../lib/parse.js");

test("parseChecks counts mixed contexts", () => {
  const pr = {
    number: 102,
    title: "API",
    url: "https://github.com/o/r/pull/102",
    state: "OPEN",
    headRefName: "api-endpoints",
    reviewDecision: "CHANGES_REQUESTED",
    statusCheckRollup: {
      state: "FAILURE",
      contexts: {
        nodes: [
          { __typename: "CheckRun", name: "lint", status: "COMPLETED", conclusion: "FAILURE" },
          { __typename: "CheckRun", name: "test", status: "COMPLETED", conclusion: "SUCCESS" },
          { __typename: "StatusContext", context: "deploy", state: "PENDING" },
        ],
      },
    },
    latestReviews: {
      nodes: [
        { author: { login: "bob" }, state: "CHANGES_REQUESTED" },
        { author: { login: "alice" }, state: "APPROVED" },
      ],
    },
    reviewRequests: {
      nodes: [
        { requestedReviewer: { __typename: "User", login: "carol" } },
        { requestedReviewer: { __typename: "User", login: "bob" } },
      ],
    },
  };
  const got = gss.prToModel(pr, 2);
  assert.equal(got.checks.state, "FAILURE");
  assert.equal(got.checks.failed, 1);
  assert.equal(got.checks.passed, 1);
  assert.equal(got.checks.pending, 1);
  assert.deepEqual(got.checks.failedNames, ["lint"]);
  assert.equal(got.reviews.decision, "CHANGES_REQUESTED");
  assert.deepEqual(got.reviews.approved, ["alice"]);
  assert.deepEqual(got.reviews.changesRequested, ["bob"]);
  assert.deepEqual(got.reviews.pending, ["carol"]);
});

test("parseGQLStack orders by position", () => {
  const meta = {
    number: 6,
    size: 3,
    baseRefName: "main",
    entries: {
      nodes: [
        { position: 3, pullRequest: { number: 103, headRefName: "frontend", state: "OPEN" } },
        { position: 1, pullRequest: { number: 101, headRefName: "auth-layer", state: "OPEN" } },
        { position: 2, pullRequest: { number: 102, headRefName: "api-endpoints", state: "OPEN" } },
      ],
    },
  };
  const stack = gss.parseGQLStack(meta, meta.entries.nodes);
  assert.equal(stack.number, 6);
  assert.equal(stack.base, "main");
  assert.deepEqual(stack.pullRequests.map((p) => p.number), [101, 102, 103]);
});

test("superseded cancelled run is not failing", () => {
  const auto = { workflowRun: { workflow: { name: "Auto approve" } } };
  const style = { workflowRun: { workflow: { name: "Code style" } } };
  const c = gss.parseChecks({
    state: "FAILURE",
    contexts: {
      nodes: [
        { __typename: "CheckRun", name: "build", status: "COMPLETED", conclusion: "CANCELLED", startedAt: "2026-08-26T02:09:05Z", checkSuite: auto },
        { __typename: "CheckRun", name: "build", status: "COMPLETED", conclusion: "SUCCESS", startedAt: "2026-08-26T02:09:10Z", checkSuite: auto },
        { __typename: "CheckRun", name: "code-style", status: "COMPLETED", conclusion: "CANCELLED", startedAt: "2026-08-26T02:09:08Z", checkSuite: style },
        { __typename: "CheckRun", name: "code-style", status: "COMPLETED", conclusion: "SUCCESS", startedAt: "2026-08-26T02:09:12Z", checkSuite: style },
      ],
    },
  });
  assert.equal(c.state, "SUCCESS");
  assert.equal(c.failed, 0);
  assert.equal(c.passed, 2);
});

test("same check name in different workflows is kept separate", () => {
  const c = gss.parseChecks({
    state: "FAILURE",
    contexts: {
      nodes: [
        { __typename: "CheckRun", name: "build", status: "COMPLETED", conclusion: "SUCCESS", startedAt: "2026-08-26T02:09:10Z", checkSuite: { workflowRun: { workflow: { name: "Auto approve" } } } },
        { __typename: "CheckRun", name: "build", status: "COMPLETED", conclusion: "FAILURE", startedAt: "2026-08-26T02:09:12Z", checkSuite: { workflowRun: { workflow: { name: "superquote-app checks" } } } },
      ],
    },
  });
  assert.equal(c.passed, 1);
  assert.equal(c.failed, 1);
  assert.equal(c.state, "FAILURE");
});

test("neutral and skipped count as skipped", () => {
  const c = gss.parseChecks({
    state: "SUCCESS",
    contexts: {
      nodes: [
        { __typename: "CheckRun", name: "Header rules", status: "COMPLETED", conclusion: "NEUTRAL" },
        { __typename: "CheckRun", name: "Pages changed", status: "COMPLETED", conclusion: "SKIPPED" },
        { __typename: "CheckRun", name: "build", status: "COMPLETED", conclusion: "SUCCESS" },
      ],
    },
  });
  assert.equal(c.skipped, 2);
  assert.equal(c.passed, 1);
  assert.equal(gss.checksCounted(c), 1);
  assert.equal(c.state, "SUCCESS");
});

test("status contexts are deduped by context", () => {
  const c = gss.parseChecks({
    state: "FAILURE",
    contexts: {
      nodes: [
        { __typename: "StatusContext", context: "deploy/netlify", state: "FAILURE", createdAt: "2026-08-26T02:09:00Z" },
        { __typename: "StatusContext", context: "deploy/netlify", state: "SUCCESS", createdAt: "2026-08-26T02:11:00Z" },
      ],
    },
  });
  assert.equal(c.passed, 1);
  assert.equal(c.failed, 0);
});

test("pending run wins over older failure", () => {
  const ci = { workflowRun: { workflow: { name: "CI" } } };
  const c = gss.parseChecks({
    state: "FAILURE",
    contexts: {
      nodes: [
        { __typename: "CheckRun", name: "test", status: "COMPLETED", conclusion: "FAILURE", startedAt: "2026-08-26T02:09:00Z", checkSuite: ci },
        { __typename: "CheckRun", name: "test", status: "IN_PROGRESS", startedAt: "2026-08-26T02:20:00Z", checkSuite: ci },
      ],
    },
  });
  assert.equal(c.pending, 1);
  assert.equal(c.failed, 0);
  assert.equal(c.state, "PENDING");
});

test("parseRebase matches CLI rules", () => {
  const cases = [
    ["MERGEABLE", "CLEAN", "UP_TO_DATE"],
    ["MERGEABLE", "UNSTABLE", "UP_TO_DATE"],
    ["MERGEABLE", "BLOCKED", "UP_TO_DATE"],
    ["MERGEABLE", "DRAFT", "UP_TO_DATE"],
    ["MERGEABLE", "BEHIND", "BEHIND"],
    ["CONFLICTING", "DIRTY", "CONFLICT"],
    ["CONFLICTING", "BLOCKED", "CONFLICT"],
    ["MERGEABLE", "DIRTY", "CONFLICT"],
    ["UNKNOWN", "UNKNOWN", "UNKNOWN"],
    ["", "", "UNKNOWN"],
  ];
  for (const [m, s, want] of cases) {
    assert.equal(gss.parseRebase(m, s).status, want, `${m} ${s}`);
  }
});
