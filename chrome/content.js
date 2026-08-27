(function () {
  var lastKey = "";
  var lastStack = null;
  var inflight = false;
  var lastError = null;

  function parsePage() {
    var m = location.pathname.match(/^\/([^/]+)\/([^/]+)\/pull\/(\d+)/);
    if (!m) return null;
    return { owner: m[1], repo: m[2], number: parseInt(m[3], 10) };
  }

  function optionsUrl() {
    return chrome.runtime.getURL("options.html");
  }

  function apply(stack, error) {
    var roots = gss.collectStackRoots(document);
    if (!roots.length) return;
    roots.forEach(function (root) {
      if (error) {
        var href = optionsUrl();
        var msg = error.needsToken
          ? "gh-stack-status needs a GitHub token to load CI and review status."
          : "gh-stack-status could not load this stack: " + (error.error || "unknown error");
        gss.showBanner(root, msg, error.needsToken ? "Add a token" : "Open settings", href);
        return;
      }
      var prev = root.querySelector(".gss-banner");
      if (prev) prev.remove();
      gss.injectIntoRoot(root, stack.pullRequests || []);
    });
  }

  function refresh(force) {
    var page = parsePage();
    if (!page) return;
    var roots = gss.collectStackRoots(document);
    if (!roots.length) return;
    var key = page.owner + "/" + page.repo + "#" + page.number;
    if (!force && inflight) return;
    if (!force && lastKey === key && lastStack && !lastError) {
      var painted = true;
      for (var i = 0; i < roots.length; i++) {
        if (!roots[i].querySelector(".gss-status")) painted = false;
      }
      if (painted) return;
      apply(lastStack, null);
      return;
    }
    inflight = true;
    chrome.runtime.sendMessage(
      { type: "gss:loadStack", owner: page.owner, repo: page.repo, number: page.number },
      function (res) {
        inflight = false;
        if (chrome.runtime.lastError) {
          lastError = { error: chrome.runtime.lastError.message };
          apply(null, lastError);
          return;
        }
        if (!res || !res.ok) {
          lastError = res || { error: "no response" };
          apply(null, lastError);
          return;
        }
        lastError = null;
        lastKey = key;
        lastStack = res.stack;
        apply(res.stack, null);
      }
    );
  }

  var scheduled = null;
  function schedule() {
    if (scheduled) return;
    scheduled = setTimeout(function () {
      scheduled = null;
      refresh(false);
    }, 200);
  }

  var observer = new MutationObserver(function (mutations) {
    for (var i = 0; i < mutations.length; i++) {
      var t = mutations[i].target;
      if (t && t.closest && t.closest(".gss-status, .gss-banner")) continue;
      if (mutations[i].addedNodes) {
        var skip = true;
        for (var j = 0; j < mutations[i].addedNodes.length; j++) {
          var n = mutations[i].addedNodes[j];
          if (n.nodeType !== 1) continue;
          if (n.classList && (n.classList.contains("gss-status") || n.classList.contains("gss-banner"))) continue;
          if (n.closest && n.closest(".gss-status, .gss-banner")) continue;
          skip = false;
          break;
        }
        if (skip && mutations[i].addedNodes.length) continue;
      }
      schedule();
      return;
    }
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  document.addEventListener("click", function () { setTimeout(schedule, 50); }, true);
  schedule();
})();
