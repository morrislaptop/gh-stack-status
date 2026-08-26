var gss = globalThis.gss || (globalThis.gss = {});

function upper(s) {
  return String(s || "").trim().toUpperCase();
}

function workflowName(ctx) {
  return ctx && ctx.checkSuite && ctx.checkSuite.workflowRun &&
    ctx.checkSuite.workflowRun.workflow && ctx.checkSuite.workflowRun.workflow.name
    ? ctx.checkSuite.workflowRun.workflow.name
    : "";
}

function startedTime(ctx) {
  return (ctx && (ctx.startedAt || ctx.createdAt)) || "";
}

function normalizeTypename(ctx) {
  if (!ctx) return "";
  if (ctx.__typename) return ctx.__typename;
  if (ctx.name || ctx.conclusion != null || ctx.status) return "CheckRun";
  if (ctx.context) return "StatusContext";
  return "";
}

function classifyContext(ctx) {
  switch (normalizeTypename(ctx)) {
    case "CheckRun": {
      var name = ctx.name || "";
      var status = upper(ctx.status);
      if (status && status !== "COMPLETED") return { kind: "pending", name: name };
      if (ctx.conclusion == null) return { kind: "pending", name: name };
      switch (upper(ctx.conclusion)) {
        case "SUCCESS":
          return { kind: "passed", name: name };
        case "NEUTRAL":
        case "SKIPPED":
          return { kind: "skipped", name: name };
        default:
          return { kind: "failed", name: name };
      }
    }
    case "StatusContext": {
      var ctxName = ctx.context || "";
      switch (upper(ctx.state)) {
        case "SUCCESS":
          return { kind: "passed", name: ctxName };
        case "NEUTRAL":
        case "SKIPPED":
          return { kind: "skipped", name: ctxName };
        case "FAILURE":
        case "ERROR":
          return { kind: "failed", name: ctxName };
        default:
          return { kind: "pending", name: ctxName };
      }
    }
    default:
      return { kind: "", name: "" };
  }
}

function dedupeContexts(nodes) {
  var ordered = (nodes || []).slice().sort(function (a, b) {
    return startedTime(a) < startedTime(b) ? 1 : startedTime(a) > startedTime(b) ? -1 : 0;
  });
  var seen = {};
  var unique = [];
  for (var i = 0; i < ordered.length; i++) {
    var ctx = ordered[i];
    var key;
    if (normalizeTypename(ctx) === "StatusContext") {
      key = "status:" + ctx.context;
    } else {
      key = "check:" + ctx.name + "/" + workflowName(ctx);
    }
    if (seen[key]) continue;
    seen[key] = true;
    unique.push(ctx);
  }
  return unique;
}

function checksTotal(c) {
  return c.passed + c.failed + c.pending + c.skipped;
}

function checksCounted(c) {
  return c.passed + c.failed + c.pending;
}

function rollupState(c, reported) {
  if (checksTotal(c) === 0) return upper(reported);
  if (c.failed > 0) return "FAILURE";
  if (c.pending > 0) return "PENDING";
  return "SUCCESS";
}

function parseChecks(rollup) {
  var c = { state: "", passed: 0, failed: 0, pending: 0, skipped: 0, failedNames: [] };
  if (!rollup) return c;
  var seenFail = {};
  var nodes = rollup.contexts && rollup.contexts.nodes ? rollup.contexts.nodes : [];
  var unique = dedupeContexts(nodes);
  for (var i = 0; i < unique.length; i++) {
    var classified = classifyContext(unique[i]);
    switch (classified.kind) {
      case "passed":
        c.passed++;
        break;
      case "skipped":
        c.skipped++;
        break;
      case "failed":
        c.failed++;
        if (classified.name && !seenFail[classified.name]) {
          seenFail[classified.name] = true;
          c.failedNames.push(classified.name);
        }
        break;
      case "pending":
        c.pending++;
        break;
    }
  }
  c.state = rollupState(c, rollup.state);
  return c;
}

function reviewerName(r) {
  if (!r) return "";
  return r.login || r.combinedSlug || r.name || "";
}

function parseReviews(decision, pr) {
  var r = { decision: decision ? upper(decision) : "", approved: [], changesRequested: [], pending: [] };
  var approved = {};
  var changes = {};
  var latest = pr && pr.latestReviews && pr.latestReviews.nodes ? pr.latestReviews.nodes : [];
  for (var i = 0; i < latest.length; i++) {
    var n = latest[i];
    var login = n.author && n.author.login;
    if (!login) continue;
    switch (upper(n.state)) {
      case "APPROVED":
        if (!approved[login]) {
          approved[login] = true;
          r.approved.push(login);
        }
        break;
      case "CHANGES_REQUESTED":
        if (!changes[login]) {
          changes[login] = true;
          r.changesRequested.push(login);
        }
        break;
    }
  }
  var pendingSeen = {};
  var reqs = pr && pr.reviewRequests && pr.reviewRequests.nodes ? pr.reviewRequests.nodes : [];
  for (var j = 0; j < reqs.length; j++) {
    var name = reviewerName(reqs[j].requestedReviewer);
    if (!name || approved[name] || changes[name] || pendingSeen[name]) continue;
    pendingSeen[name] = true;
    r.pending.push(name);
  }
  return r;
}

function parseRebase(mergeable, mergeState) {
  var m = upper(mergeable);
  var s = upper(mergeState);
  var r = { status: "UNKNOWN", mergeable: m, mergeStateStatus: s };
  if (m === "CONFLICTING" || s === "DIRTY") r.status = "CONFLICT";
  else if (s === "BEHIND") r.status = "BEHIND";
  else if (m === "UNKNOWN" || s === "UNKNOWN" || (!m && !s)) r.status = "UNKNOWN";
  else r.status = "UP_TO_DATE";
  return r;
}

function prToModel(pr, position) {
  return {
    number: pr.number,
    title: pr.title || "",
    url: pr.url || "",
    branch: pr.headRefName || "",
    state: pr.state || "",
    draft: !!pr.isDraft,
    position: position,
    checks: parseChecks(pr.statusCheckRollup),
    reviews: parseReviews(pr.reviewDecision, pr),
    rebase: parseRebase(pr.mergeable, pr.mergeStateStatus),
  };
}

function parseGQLStack(meta, entries) {
  var byPos = {};
  (entries || []).forEach(function (e) {
    if (!e || !e.pullRequest) return;
    byPos[e.position] = prToModel(e.pullRequest, e.position);
  });
  var positions = Object.keys(byPos).map(Number).sort(function (a, b) { return a - b; });
  return {
    number: meta.number,
    base: meta.baseRefName || "",
    pullRequests: positions.map(function (p) { return byPos[p]; }),
  };
}

gss.parseChecks = parseChecks;
gss.parseReviews = parseReviews;
gss.parseRebase = parseRebase;
gss.prToModel = prToModel;
gss.parseGQLStack = parseGQLStack;
gss.checksTotal = checksTotal;
gss.checksCounted = checksCounted;
gss.dedupeContexts = dedupeContexts;

if (typeof module !== "undefined" && module.exports) module.exports = gss;
