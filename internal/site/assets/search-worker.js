// One shard of the site search, run off the page's main thread so that
// typing never waits for it. site.js starts a few of these; each one
// indexes its share of the pages and answers queries with its best hits.
importScripts('fuse.min.js', 'search.js');

var fuse, off = 0;
onmessage = function (e) {
  var m = e.data;
  if (m.options) {
    var docs = self.folioSearch || [], n = Math.ceil(docs.length / m.of);
    off = m.shard * n;
    fuse = new Fuse(docs.slice(off, off + n), m.options);
    return postMessage({ ready: true });
  }
  postMessage({ id: m.id, hits: fuse.search(m.q, { limit: m.limit }).map(function (h) {
    return { item: h.item, matches: h.matches, score: h.score, i: h.refIndex + off };
  }) });
};
