const test = require("node:test");
const assert = require("node:assert/strict");
require("../lib/gss.js");
const gss = require("../lib/inject.js");

test("parsePRHref reads GitHub pull URLs", () => {
  assert.equal(gss.parsePRHref("/mannum/superquote/pull/17747"), 17747);
  assert.equal(gss.parsePRHref("https://github.com/a/b/pull/12/files"), 12);
  assert.equal(gss.parsePRHref("/a/b/issues/3"), 0);
});
