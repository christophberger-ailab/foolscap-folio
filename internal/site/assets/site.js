(function () {
  'use strict';
  var doc = document.documentElement;

  function toggle(btn, cls) {
    btn.addEventListener('click', function () {
      var open = doc.classList.toggle(cls);
      btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
  }

  var sidebarBtn = document.querySelector('.sidebar-toggle');
  var menuBtn = document.querySelector('.menu-toggle');
  if (sidebarBtn) toggle(sidebarBtn, 'sidebar-open');
  if (menuBtn) toggle(menuBtn, 'menu-open');

  function closeAll() {
    doc.classList.remove('sidebar-open', 'menu-open');
    if (sidebarBtn) sidebarBtn.setAttribute('aria-expanded', 'false');
    if (menuBtn) menuBtn.setAttribute('aria-expanded', 'false');
  }
  var scrim = document.querySelector('.scrim');
  if (scrim) scrim.addEventListener('click', closeAll);
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') closeAll();
  });
  var sidebar = document.getElementById('sidebar');
  if (sidebar) sidebar.addEventListener('click', function (e) {
    if (e.target.closest && e.target.closest('a')) closeAll();
  });

  // Keep the current page visible in the scrolled sidebar.
  var current = document.querySelector('.sidebar [aria-current="page"]');
  if (current && sidebar) {
    var r = current.getBoundingClientRect(), s = sidebar.getBoundingClientRect();
    if (r.top < s.top || r.bottom > s.bottom) {
      sidebar.scrollTop += r.top - s.top - s.height / 3;
    }
  }

  var article = document.querySelector('.page');

  // fold is s without case and diacritics, character by character, so that
  // its indices are those of s.
  function fold(s) {
    var f = '';
    for (var i = 0; i < s.length; i++) {
      var c = s[i].normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();
      f += c.length === 1 ? c : s[i];
    }
    return f;
  }

  // mark marks the terms wherever the page's text has them, ignoring case
  // and diacritics as the search does, and returns the marks in page order;
  // the marks of an earlier call are removed.
  function mark(terms) {
    var old = article.querySelectorAll('mark.page-mark');
    old.forEach(function (m) {
      var p = m.parentNode;
      m.replaceWith(m.textContent);
      p.normalize();
    });
    terms = terms.map(fold).filter(function (t) { return t.length > 1; });
    if (!terms.length) return [];
    terms.sort(function (a, b) { return b.length - a.length; });
    var re = new RegExp(terms.map(function (t) { return t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }).join('|'), 'g');
    var walk = document.createTreeWalker(article, NodeFilter.SHOW_TEXT, {
      acceptNode: function (n) {
        return n.parentElement.closest('script, style, svg') ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT;
      }
    });
    var nodes = [], n, found = [];
    while ((n = walk.nextNode())) nodes.push(n);
    nodes.forEach(function (n) {
      var text = fold(n.data), ranges = [], m, own = [];
      while ((m = re.exec(text))) ranges.push([m.index, m[0].length]);
      // from the last, so that n keeps the text before them
      for (var i = ranges.length - 1; i >= 0; i--) {
        var hit = n.splitText(ranges[i][0]);
        hit.splitText(ranges[i][1]);
        var el = document.createElement('mark');
        el.className = 'page-mark';
        hit.replaceWith(el);
        el.appendChild(hit);
        own.unshift(el);
      }
      found = found.concat(own);
    });
    return found;
  }
  // pick makes the i-th mark the current one and scrolls to it.
  function pick(marks, i) {
    marks.forEach(function (m, j) { m.classList.toggle('current', j === i); });
    if (marks[i]) marks[i].scrollIntoView({ block: 'center' });
  }

  // A page opened from a search hit marks the words the hit matched, which
  // the hit's link names as ?mark=, and scrolls to the first.
  if (article) {
    var terms = new URLSearchParams(location.search).getAll('mark').slice(0, 20);
    if (terms.length) pick(mark(terms), 0);
  }

  // The find field above the contents list marks its text in this page;
  // Enter and Shift+Enter step through the marks, Escape clears it.
  var finder = document.querySelector('.find');
  if (finder && article) find(finder);

  function find(box) {
    var input = box.querySelector('input'), count = box.querySelector('.find-count');
    var marks = [], at = 0;
    function go(i) {
      at = (i + marks.length) % marks.length;
      pick(marks, at);
      count.textContent = (at + 1) + '/' + marks.length;
    }
    input.addEventListener('input', function () {
      var q = input.value.trim();
      marks = mark(q.length > 1 ? [q] : []);
      count.textContent = q.length > 1 && !marks.length ? '0/0' : '';
      if (marks.length) go(0);
    });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && marks.length) {
        e.preventDefault();
        go(at + (e.shiftKey ? -1 : 1));
      } else if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        input.value = '';
        marks = mark([]);
        count.textContent = '';
      }
    });
  }

  // Search. Fuse.js and the index are loaded on first use, so a reader who
  // never searches does not download them. The search runs in Fuse.js's web
  // workers, which share the pages between them, so neither indexing nor
  // typing on a large site freezes the page.
  var box = document.querySelector('.search');
  if (box) search(box);

  function search(box) {
    var input = box.querySelector('input');
    var list = box.querySelector('.search-results');
    var clear = box.querySelector('.search-clear');
    var busy = box.querySelector('.search-busy');
    var base = box.getAttribute('data-base') || '';
    var find = null, loading = false, sel = -1, timer = 0, slow = 0, shown = '', stale = false;
    var options = {
      keys: [{ name: 't', weight: 3 }, { name: 'b', weight: 1 }],
      ignoreLocation: true, ignoreFieldNorm: true, threshold: 0.2,
      minMatchCharLength: 2, includeMatches: true, ignoreDiacritics: true
    };
    var radius = 70, max = 10, key = 'folio-search', hitsKey = 'folio-search-hits';

    // The query and its hits outlive the page for the browser tab, so the
    // hits are there again at once after opening one of them, before the
    // index has loaded. Storage may be unavailable (privacy settings, some
    // file:// setups); the search works without it.
    function store(k, v) {
      try { if (v) sessionStorage.setItem(k, v); else sessionStorage.removeItem(k); } catch (e) {}
    }
    function stored(k) {
      try { return sessionStorage.getItem(k) || ''; } catch (e) { return ''; }
    }
    function changed() {
      if (clear) clear.hidden = !input.value;
      store(key, input.value);
    }
    if (!input.value) input.value = stored(key);
    changed();

    function script(src, done) {
      var s = document.createElement('script');
      s.src = base + src;
      s.onload = done;
      document.head.appendChild(s);
    }
    function load() {
      if (find || loading) return;
      loading = true;
      script('_site/search.js', function () {
        // relative to this script, in _site/
        import('./fuse-worker.mjs').then(function (m) {
          var fuse = new m.FuseWorker(window.folioSearch || [], options);
          find = function (q, done) {
            fuse.search(q, { limit: max }).then(done, failed);
          };
          run();
        }, failed);
      });
    }
    function failed(e) {
      wait(false);
      console.error('search:', e);
    }
    // wait shows the busy indicator once a search has taken a second.
    function wait(on) {
      if (on) { if (!slow) slow = setTimeout(function () { busy.hidden = false; }, 1000); return; }
      clearTimeout(slow);
      slow = 0;
      busy.hidden = true;
    }

    // snippet is the body text around its longest match, as [text, marked]
    // parts.
    function snippet(doc, matches) {
      var m = null;
      matches.forEach(function (x) { if (x.key === 'b') m = x; });
      var text = doc.b;
      if (!m) return [[text.slice(0, 2 * radius) + (text.length > 2 * radius ? '…' : '')]];
      var best = m.indices[0];
      m.indices.forEach(function (r) { if (r[1] - r[0] > best[1] - best[0]) best = r; });
      var from = Math.max(0, best[0] - radius), to = Math.min(text.length, best[1] + 1 + radius);
      var parts = from > 0 ? [['…']] : [], at = from;
      m.indices.forEach(function (r) {
        if (r[1] < at || r[0] >= to || r[1] - r[0] < 1) return;
        var s = Math.max(r[0], at), e = Math.min(r[1] + 1, to);
        parts.push([text.slice(at, s)], [text.slice(s, e), 1]);
        at = e;
      });
      parts.push([text.slice(at, to) + (to < text.length ? '…' : '')]);
      return parts;
    }

    // matched is the words of the page that a hit matched, for its page to
    // mark.
    function matched(doc, matches) {
      var seen = {}, words = [];
      matches.forEach(function (x) {
        x.indices.forEach(function (r) {
          var w = doc[x.key].slice(r[0], r[1] + 1), k = fold(w);
          if (k.length > 1 && !seen[k] && words.length < 20) { seen[k] = true; words.push(w); }
        });
      });
      return words;
    }

    function el(tag, cls, text) {
      var e = document.createElement(tag);
      e.className = cls;
      if (text) e.textContent = text;
      return e;
    }
    function show(open) {
      list.hidden = !open;
      input.setAttribute('aria-expanded', open ? 'true' : 'false');
    }
    function run() {
      var q = input.value.trim();
      if (q.length < 2) {
        wait(false);
        store(hitsKey, '');
        list.textContent = '';
        sel = -1;
        shown = '';
        stale = false;
        return show(false);
      }
      // stored hits are shown while the search runs again, as the index
      // may have changed since
      if (q === shown && !stale) return wait(false);
      if (q !== shown) wait(true);
      if (!find) return load();
      find(q, function (hits) {
        // a later query may have been answered first
        if (q !== input.value.trim()) return;
        wait(false);
        hits = hits.map(function (h) {
          return { t: h.item.t, u: h.item.u, s: h.item.s, p: snippet(h.item, h.matches || []), m: matched(h.item, h.matches || []) };
        });
        store(hitsKey, JSON.stringify({ q: q, hits: hits }));
        render(q, hits, false);
        show(box.contains(document.activeElement));
      });
    }
    // render lists the hits for q; a new list of the same query keeps the
    // selected hit.
    function render(q, hits, old) {
      var keep = q === shown ? sel : -1;
      list.textContent = '';
      sel = -1;
      shown = q;
      stale = old;
      if (!hits.length) {
        list.appendChild(el('li', 'search-none', box.getAttribute('data-nohits')));
      }
      hits.forEach(function (h) {
        var a = el('a', 'search-hit');
        var q = new URLSearchParams();
        (h.m || []).forEach(function (w) { q.append('mark', w); });
        a.href = base + h.u + (h.m && h.m.length ? '?' + q : '');
        a.appendChild(el('span', 'search-title', h.t));
        if (h.s && h.s !== h.t) a.appendChild(el('span', 'search-section', h.s));
        var p = el('span', 'search-snippet');
        h.p.forEach(function (x) { p.appendChild(x[1] ? el('mark', '', x[0]) : document.createTextNode(x[0])); });
        a.appendChild(p);
        var li = document.createElement('li');
        li.appendChild(a);
        list.appendChild(li);
      });
      if (keep >= 0 && keep < hits.length) move(keep + 1);
    }
    try {
      var last = JSON.parse(stored(hitsKey) || 'null');
      if (last && last.q === input.value.trim()) render(last.q, last.hits, true);
    } catch (e) {}
    function move(d) {
      var hits = list.querySelectorAll('.search-hit');
      if (!hits.length) return;
      if (sel >= 0) hits[sel].classList.remove('selected');
      sel = (sel + d + hits.length) % hits.length;
      hits[sel].classList.add('selected');
      hits[sel].scrollIntoView({ block: 'nearest' });
    }

    input.addEventListener('focus', load);
    input.addEventListener('input', function () {
      changed();
      clearTimeout(timer);
      timer = setTimeout(run, 120);
    });
    input.addEventListener('keydown', function (e) {
      if ((e.key === 'ArrowDown' || e.key === 'ArrowUp') && list.hidden) {
        e.preventDefault();
        if (list.childNodes.length) show(true); else run();
      } else if (e.key === 'ArrowDown') { e.preventDefault(); move(1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1); }
      else if (e.key === 'Enter') {
        var hit = list.querySelectorAll('.search-hit')[Math.max(sel, 0)];
        if (hit) location.href = hit.href;
      } else if (e.key === 'Escape') {
        // Escape dismisses the hits but keeps them for the next focus; the
        // default would clear a search field.
        e.preventDefault();
        if (!list.hidden) { e.stopPropagation(); show(false); } else input.blur();
      }
    });
    if (clear) clear.addEventListener('click', function () {
      input.value = '';
      changed();
      run();
      input.focus();
    });
    box.addEventListener('focusout', function (e) {
      if (!box.contains(e.relatedTarget)) show(false);
    });
    input.addEventListener('focusin', function () { if (list.childNodes.length) show(true); });
    document.addEventListener('keydown', function (e) {
      var t = e.target.tagName;
      if (e.key === '/' && t !== 'INPUT' && t !== 'TEXTAREA' && !e.target.isContentEditable) {
        e.preventDefault();
        input.focus();
      }
    });
  }

  // A click on an image of the page shows it at full size over the page,
  // scrolled where it is larger than the window; a click or Escape closes
  // it. An image inside a link follows the link.
  if (article && window.HTMLDialogElement) article.addEventListener('click', function (e) {
    var img = e.target.closest && e.target.closest('img');
    if (img && !img.closest('a')) fullSize(img);
  });

  function fullSize(img) {
    var box = document.createElement('dialog');
    var big = document.createElement('img');
    box.className = 'lightbox';
    big.setAttribute('src', img.getAttribute('src'));
    big.alt = img.alt;
    box.appendChild(big);
    box.addEventListener('click', function () { box.close(); });
    box.addEventListener('close', function () { box.remove(); });
    document.body.appendChild(box);
    box.showModal();
  }

  // Scroll spy for the table of contents.
  var links = document.querySelectorAll('.toc a[href^="#"]');
  if (!links.length || !('IntersectionObserver' in window)) return;
  var byId = {}, targets = [];
  links.forEach(function (a) {
    var id = decodeURIComponent(a.getAttribute('href').slice(1));
    var el = document.getElementById(id);
    if (el) { byId[id] = a; targets.push(el); }
  });
  var visible = {};
  function update() {
    var active = null;
    for (var i = 0; i < targets.length; i++) {
      if (visible[targets[i].id]) { active = targets[i].id; break; }
    }
    if (active === null) return;
    links.forEach(function (a) { a.classList.remove('active'); a.removeAttribute('aria-current'); });
    var a = byId[active];
    if (a) { a.classList.add('active'); a.setAttribute('aria-current', 'location'); }
  }
  var io = new IntersectionObserver(function (entries) {
    entries.forEach(function (e) { visible[e.target.id] = e.isIntersecting; });
    update();
  }, { rootMargin: '-10% 0px -70% 0px' });
  targets.forEach(function (t) { io.observe(t); });
  // Highlight the first entry until something scrolls into view.
  if (targets.length) { visible[targets[0].id] = true; update(); visible[targets[0].id] = false; }
})();
