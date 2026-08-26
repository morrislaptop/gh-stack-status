var gss = globalThis.gss || (globalThis.gss = {});

function layerReady(pr) {
  if (!pr) return false;
  if (pr.draft) return false;
  var state = String(pr.state || "").toUpperCase();
  if (state && state !== "OPEN") return true;
  var rebase = String(pr.rebase && pr.rebase.status || "").toUpperCase();
  if (rebase === "CONFLICT" || rebase === "BEHIND") return false;
  var checks = String(pr.checks && pr.checks.state || "").toUpperCase();
  if (checks === "FAILURE" || checks === "ERROR" || checks === "PENDING" || checks === "EXPECTED") return false;
  var decision = String(pr.reviews && pr.reviews.decision || "").toUpperCase();
  if (decision === "CHANGES_REQUESTED" || decision === "REVIEW_REQUIRED") return false;
  return true;
}

function layerBlockers(pr) {
  var reasons = [];
  if (pr.draft) reasons.push("draft");
  var rebase = gss.formatRebase(pr.rebase || {});
  if (rebase.tone === "attention" || rebase.tone === "danger") reasons.push(rebase.text);
  var checks = gss.formatChecks(pr.checks || { state: "", passed: 0, failed: 0, pending: 0, skipped: 0, failedNames: [] });
  if (checks.tone === "attention" || checks.tone === "danger") reasons.push(checks.text);
  var reviews = gss.formatReviews(pr.reviews || {});
  if (reviews.tone === "attention" || reviews.tone === "danger") reasons.push(reviews.text);
  return reasons;
}

/**
 * Overall merge-readiness for a layer. `prs` is bottom-to-top (position 1 first).
 * `index` is into that array.
 */
function overallStatus(prs, index) {
  var pr = prs[index];
  if (!layerReady(pr)) {
    return { key: "not-ready", label: "Not ready", tone: "attention", reasons: layerBlockers(pr) };
  }
  for (var i = 0; i < index; i++) {
    if (!layerReady(prs[i])) {
      return {
        key: "blocked-downstack",
        label: "Blocked downstack",
        tone: "attention",
        reasons: ["#" + prs[i].number + " is not ready"],
      };
    }
  }
  return { key: "ready", label: "Ready", tone: "success", reasons: [] };
}

function annotateStack(stack) {
  var prs = (stack && stack.pullRequests) || [];
  return prs.map(function (pr, i) {
    return Object.assign({}, pr, {
      overall: overallStatus(prs, i),
      rebaseFmt: gss.formatRebase(pr.rebase),
      checksFmt: gss.formatChecks(pr.checks),
      reviewsFmt: gss.formatReviews(pr.reviews),
    });
  });
}

gss.layerReady = layerReady;
gss.layerBlockers = layerBlockers;
gss.overallStatus = overallStatus;
gss.annotateStack = annotateStack;

if (typeof module !== "undefined" && module.exports) module.exports = gss;
