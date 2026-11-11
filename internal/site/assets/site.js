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

  // Search. Fuse.js and the index are loaded as scripts on first use, so a
  // reader who never searches does not download them, and the site keeps
  // working from file://, where a page may not fetch files.
  var box = document.querySelector('.search');
  if (box) search(box);

  function search(box) {
    var input = box.querySelector('input');
    var list = box.querySelector('.search-results');
    var clear = box.querySelector('.search-clear');
    var base = box.getAttribute('data-base') || '';
    var fuse = null, loading = false, sel = -1, timer = 0;
    var radius = 70, max = 10, key = 'folio-search';

    // The query outlives the page for the browser tab, so the hits are
    // there again after opening one of them. Storage may be unavailable
    // (privacy settings, some file:// setups); the search works without it.
    function store(q) {
      try { if (q) sessionStorage.setItem(key, q); else sessionStorage.removeItem(key); } catch (e) {}
    }
    function stored() {
      try { return sessionStorage.getItem(key) || ''; } catch (e) { return ''; }
    }
    function changed() {
      if (clear) clear.hidden = !input.value;
      store(input.value);
    }
    if (!input.value) input.value = stored();
    changed();

    function script(src, done) {
      var s = document.createElement('script');
      s.src = base + src;
      s.onload = done;
      document.head.appendChild(s);
    }
    function load() {
      if (fuse || loading) return;
      loading = true;
      script('_site/fuse.min.js', function () {
        script('_site/search.js', function () {
          fuse = new window.Fuse(window.folioSearch || [], {
            keys: [{ name: 't', weight: 3 }, { name: 'b', weight: 1 }],
            ignoreLocation: true, ignoreFieldNorm: true, threshold: 0.2,
            minMatchCharLength: 2, includeMatches: true, ignoreDiacritics: true
          });
          run();
        });
      });
    }

    // snippet is the body text around its longest match, with the matches
    // in it marked.
    function snippet(doc, matches) {
      var m = null;
      matches.forEach(function (x) { if (x.key === 'b') m = x; });
      var text = doc.b, frag = document.createDocumentFragment();
      if (!m) { frag.textContent = text.slice(0, 2 * radius) + (text.length > 2 * radius ? '…' : ''); return frag; }
      var best = m.indices[0];
      m.indices.forEach(function (r) { if (r[1] - r[0] > best[1] - best[0]) best = r; });
      var from = Math.max(0, best[0] - radius), to = Math.min(text.length, best[1] + 1 + radius);
      if (from > 0) frag.appendChild(document.createTextNode('…'));
      var at = from;
      m.indices.forEach(function (r) {
        if (r[1] < at || r[0] >= to || r[1] - r[0] < 1) return;
        var s = Math.max(r[0], at), e = Math.min(r[1] + 1, to);
        frag.appendChild(document.createTextNode(text.slice(at, s)));
        var mk = document.createElement('mark');
        mk.textContent = text.slice(s, e);
        frag.appendChild(mk);
        at = e;
      });
      frag.appendChild(document.createTextNode(text.slice(at, to) + (to < text.length ? '…' : '')));
      return frag;
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
      list.textContent = '';
      sel = -1;
      if (q.length < 2) return show(false);
      if (!fuse) return load();
      var hits = fuse.search(q, { limit: max });
      if (!hits.length) {
        list.appendChild(el('li', 'search-none', box.getAttribute('data-nohits')));
      }
      hits.forEach(function (h) {
        var a = el('a', 'search-hit');
        a.href = base + h.item.u;
        a.appendChild(el('span', 'search-title', h.item.t));
        if (h.item.s && h.item.s !== h.item.t) a.appendChild(el('span', 'search-section', h.item.s));
        var p = el('span', 'search-snippet');
        p.appendChild(snippet(h.item, h.matches || []));
        a.appendChild(p);
        var li = document.createElement('li');
        li.appendChild(a);
        list.appendChild(li);
      });
      show(true);
    }
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
