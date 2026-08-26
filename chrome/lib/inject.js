var gss = globalThis.gss || (globalThis.gss = {});

var MARK = "data-gss-status";

function parsePRHref(href) {
  if (!href) return 0;
  var m = String(href).match(/\/pull\/(\d+)/);
  return m ? parseInt(m[1], 10) : 0;
}

function parsePRHash(text) {
  if (!text) return 0;
  var m = String(text).match(/#(\d+)/);
  return m ? parseInt(m[1], 10) : 0;
}

function textLooksLikeStackHeading(el) {
  var t = (el.textContent || "").replace(/\s+/g, " ").trim();
  return /^Stack #\d+/.test(t) && t.length < 40;
}

function collectStackRoots(doc) {
  var roots = [];
  var seen = new Set();
  var nodes = doc.querySelectorAll("h1, h2, h3, h4, span, div, button, a");
  for (var i = 0; i < nodes.length; i++) {
    var el = nodes[i];
    if (!textLooksLikeStackHeading(el) && !/Add to stack/i.test((el.textContent || "").trim())) continue;
    var root = el;
    for (var up = 0; up < 10 && root; up++) {
      var links = root.querySelectorAll('a[href*="/pull/"]');
      var prs = {};
      for (var j = 0; j < links.length; j++) {
        var n = parsePRHref(links[j].getAttribute("href"));
        if (n) prs[n] = true;
      }
      if (Object.keys(prs).length >= 1 && (textLooksLikeStackHeading(el) || /Add to stack/i.test(root.textContent || ""))) {
        if (!seen.has(root)) {
          seen.add(root);
          roots.push(root);
        }
        break;
      }
      root = root.parentElement;
    }
  }
  return roots;
}

function prNumbersIn(el) {
  var found = {};
  var own = el.getAttribute ? parsePRHref(el.getAttribute("href")) : 0;
  if (own) found[own] = true;
  var links = el.querySelectorAll ? el.querySelectorAll('a[href*="/pull/"]') : [];
  for (var i = 0; i < links.length; i++) {
    var n = parsePRHref(links[i].getAttribute("href"));
    if (n) found[n] = true;
  }
  var text = el.textContent || "";
  var re = /#(\d+)/g;
  var m;
  while ((m = re.exec(text))) found[parseInt(m[1], 10)] = true;
  return Object.keys(found).map(Number);
}

function stackNumberFrom(root) {
  var t = (root.textContent || "").replace(/\s+/g, " ");
  var m = t.match(/Stack #(\d+)/);
  return m ? parseInt(m[1], 10) : 0;
}

function closestRow(el, root, number) {
  var stackNo = stackNumberFrom(root);
  var cur = el;
  var best = el;
  while (cur && cur !== root) {
    var nums = prNumbersIn(cur).filter(function (n) { return n !== number && n !== stackNo; });
    if (nums.length) break;
    best = cur;
    cur = cur.parentElement;
  }
  return best;
}

function findRowForPR(root, number) {
  var links = root.querySelectorAll('a[href*="/pull/"]');
  for (var i = 0; i < links.length; i++) {
    if (parsePRHref(links[i].getAttribute("href")) === number) {
      return closestRow(links[i], root, number);
    }
  }
  var all = root.querySelectorAll("a, span, div, p, li");
  for (var j = 0; j < all.length; j++) {
    var el = all[j];
    if (el.childElementCount > 8) continue;
    var t = (el.textContent || "").replace(/\s+/g, " ").trim();
    if (t === "#" + number || t.indexOf("#" + number + " ") === 0 || t.indexOf("#" + number + "·") === 0 || t.indexOf("#" + number + " ·") !== -1) {
      return closestRow(el, root, number);
    }
  }
  return null;
}

function toneClass(tone) {
  return "gss-tone-" + (tone || "muted");
}

function iconSVG(key) {
  if (key === "ready") {
    return '<svg class="gss-icon" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7" fill="currentColor"/><path d="M5.2 8.2l1.8 1.8 3.8-4" fill="none" stroke="#0d1117" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  }
  if (key === "blocked-downstack") {
    return '<svg class="gss-icon" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7" fill="currentColor"/><rect x="4.2" y="7.2" width="7.6" height="1.6" rx="0.8" fill="#0d1117"/></svg>';
  }
  return '<svg class="gss-icon" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7" fill="currentColor"/></svg>';
}

function docOf(el) {
  return (el && el.ownerDocument) || (typeof document !== "undefined" ? document : null);
}

function pill(doc, fmt) {
  var span = doc.createElement("span");
  span.className = "gss-pill " + toneClass(fmt.tone);
  span.textContent = fmt.text;
  return span;
}

function buildStatusNode(pr, doc) {
  doc = doc || (typeof document !== "undefined" ? document : null);
  var wrap = doc.createElement("div");
  wrap.className = "gss-status";
  wrap.setAttribute(MARK, String(pr.number));

  var overall = pr.overall || { key: "not-ready", label: "Not ready", tone: "attention", reasons: [] };
  var badge = doc.createElement("span");
  badge.className = "gss-badge " + toneClass(overall.tone);
  badge.innerHTML = iconSVG(overall.key) + '<span class="gss-badge-label">' + overall.label + "</span>";
  var title = [pr.rebaseFmt && pr.rebaseFmt.text, pr.checksFmt && pr.checksFmt.text, pr.reviewsFmt && pr.reviewsFmt.text]
    .filter(Boolean)
    .join(" · ");
  if (overall.reasons && overall.reasons.length) title = overall.reasons.join("; ") + " — " + title;
  badge.title = title;
  wrap.appendChild(badge);

  var details = doc.createElement("div");
  details.className = "gss-details";
  details.appendChild(pill(doc, pr.rebaseFmt || { text: "—", tone: "muted" }));
  details.appendChild(pill(doc, pr.checksFmt || { text: "—", tone: "muted" }));
  details.appendChild(pill(doc, pr.reviewsFmt || { text: "—", tone: "muted" }));
  wrap.appendChild(details);
  return wrap;
}

function ensureRowLayout(row) {
  var win = docOf(row).defaultView;
  var style = win && win.getComputedStyle ? win.getComputedStyle(row) : null;
  if (style && (style.display === "flex" || style.display === "grid")) return;
  row.classList.add("gss-row-host");
}

function injectIntoRoot(root, annotated) {
  var byNumber = {};
  annotated.forEach(function (pr) { byNumber[pr.number] = pr; });
  var injected = 0;
  Object.keys(byNumber).forEach(function (num) {
    var n = parseInt(num, 10);
    var row = findRowForPR(root, n);
    if (!row) return;
    var existing = row.querySelector(".gss-status[" + MARK + '="' + n + '"]');
    var node = buildStatusNode(byNumber[n], docOf(root));
    if (existing) {
      existing.replaceWith(node);
    } else {
      ensureRowLayout(row);
      row.appendChild(node);
    }
    injected++;
  });
  return injected;
}

function showBanner(root, message, actionLabel, actionHref) {
  var prev = root.querySelector(".gss-banner");
  if (prev) prev.remove();
  var doc = docOf(root);
  var banner = doc.createElement("div");
  banner.className = "gss-banner";
  banner.textContent = message + " ";
  if (actionLabel) {
    var a = doc.createElement("a");
    a.textContent = actionLabel;
    a.href = actionHref || "#";
    a.className = "gss-banner-link";
    banner.appendChild(a);
  }
  var heading = null;
  var kids = root.querySelectorAll("h1, h2, h3, span, div");
  for (var i = 0; i < kids.length; i++) {
    if (textLooksLikeStackHeading(kids[i])) {
      heading = kids[i];
      break;
    }
  }
  if (heading && heading.parentElement) heading.parentElement.insertBefore(banner, heading.nextSibling);
  else root.insertBefore(banner, root.firstChild);
}

gss.parsePRHref = parsePRHref;
gss.collectStackRoots = collectStackRoots;
gss.findRowForPR = findRowForPR;
gss.buildStatusNode = buildStatusNode;
gss.injectIntoRoot = injectIntoRoot;
gss.showBanner = showBanner;

if (typeof module !== "undefined" && module.exports) module.exports = gss;
