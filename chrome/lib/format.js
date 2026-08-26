var gss = globalThis.gss || (globalThis.gss = {});

function joinLimited(names, n) {
  if (!names || !names.length) return "";
  if (names.length > n) return names.slice(0, n).join(", ") + ", …";
  return names.join(", ");
}

function checkDetail(c) {
  var state = String(c.state || "").toUpperCase();
  if (state === "FAILURE" || state === "ERROR") {
    if (c.failedNames && c.failedNames.length) {
      var names = c.failedNames.length > 3 ? c.failedNames.slice(0, 3) : c.failedNames;
      return "(" + names.join(", ") + ")";
    }
  }
  if (gss.checksTotal(c) === 0) return "";
  var counts = c.passed + "/" + gss.checksCounted(c);
  if (state === "FAILURE" || state === "ERROR") {
    counts = c.failed + "/" + gss.checksCounted(c);
  }
  if (c.skipped > 0) counts += ", " + c.skipped + " skipped";
  return "(" + counts + ")";
}

function formatChecks(c) {
  if (!c.state && gss.checksTotal(c) === 0) {
    return { text: "—", tone: "muted", label: "—" };
  }
  var state = String(c.state || "").toUpperCase();
  var label, tone;
  switch (state) {
    case "SUCCESS":
      label = "pass";
      tone = "success";
      break;
    case "FAILURE":
    case "ERROR":
      label = "fail";
      tone = "danger";
      break;
    case "PENDING":
    case "EXPECTED":
      label = "pending";
      tone = "attention";
      break;
    default:
      label = String(c.state || "").toLowerCase() || "—";
      tone = "muted";
  }
  var detail = checkDetail(c);
  return { text: detail ? label + " " + detail : label, tone: tone, label: label, detail: detail };
}

function formatRebase(r) {
  switch (String(r.status || "").toUpperCase()) {
    case "UP_TO_DATE":
      return { text: "up to date", tone: "success" };
    case "BEHIND":
      return { text: "behind", tone: "attention" };
    case "CONFLICT":
      return { text: "conflict", tone: "danger" };
    default:
      return { text: "—", tone: "muted" };
  }
}

function reviewPeople(r, decision) {
  switch (decision) {
    case "APPROVED":
      return joinLimited(r.approved, 3);
    case "CHANGES_REQUESTED":
      return joinLimited(r.changesRequested, 3);
    case "REVIEW_REQUIRED":
      return joinLimited(r.pending, 3);
    default: {
      var all = [].concat(r.approved || [], r.changesRequested || [], r.pending || []);
      return joinLimited(all, 3);
    }
  }
}

function formatReviews(r) {
  var decision = String(r.decision || "").toUpperCase();
  var label, tone;
  switch (decision) {
    case "APPROVED":
      label = "approved";
      tone = "success";
      break;
    case "CHANGES_REQUESTED":
      label = "changes requested";
      tone = "danger";
      break;
    case "REVIEW_REQUIRED":
      label = "review required";
      tone = "attention";
      break;
    case "":
      return { text: "—", tone: "muted", label: "—" };
    default:
      label = decision.toLowerCase().replace(/_/g, " ");
      tone = "muted";
  }
  var people = reviewPeople(r, decision);
  return {
    text: people ? label + " (" + people + ")" : label,
    tone: tone,
    label: label,
    detail: people ? "(" + people + ")" : "",
  };
}

gss.formatChecks = formatChecks;
gss.formatRebase = formatRebase;
gss.formatReviews = formatReviews;
gss.joinLimited = joinLimited;

if (typeof module !== "undefined" && module.exports) module.exports = gss;
