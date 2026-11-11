// Foolscap Folio - the dock beside the editor (plain ES6, no build step)
//
// The panes beside the editor -- the preview, the copyedit tool, the
// comments, the render and Git panes, and the help -- stand in a dock of
// tabbed groups. A group shows one of its panes at a time, the one whose
// tab is active. Dragging a tab onto another group's tabs moves it there;
// dropping it on a group's edge splits the group, putting the pane beside
// or below it. A tab's ✕ closes its pane, and the Panes menu in the top
// bar brings it back.
//
// The layout is a tree: a group is a leaf, and a split holds two or more
// groups or splits side by side ('row') or stacked ('col'), each with its
// share of the room. It is kept in localStorage, so it outlives page
// switches, reloads, and the session.
//
// The dock stands outside #content, so a page switch leaves it standing,
// and with it the panes of the project. The panes of the page arrive with
// the page and are put into their place here (adoptPanes). A pane that is
// closed waits in #pane-store. app.js owns what the panes show; this file
// only says where they stand, and fires folio-shown at a pane whenever it
// comes into view, which is when the pane brings itself up to date.

var DOCK_KEY = 'dockLayout';
var DOCK_WIDTH_KEY = 'dockWidth';
var PANE_TYPE = 'application/x-folio-pane';

// PANES are the panes the dock can hold, in the order a fresh dock lists
// them: the element each is, the part of it that scrolls (kept across the
// moves that would reset it), and what its tab shows while there is no
// such element -- no page open, or no project.
var PANES = {
  preview: { label: 'Preview', id: 'preview-pane', scroll: '#preview', empty: 'Open a page to see its preview.' },
  copyedit: { label: 'Copyedit', id: 'copyedit', scroll: '#copyedit-body', empty: 'Open a page to copyedit it.' },
  comments: { label: 'Comments', id: 'comments-pane', scroll: '#comments-list', empty: '' },
  render: { label: 'Render', id: 'render-pane', scroll: '.render-body', empty: 'Open a project to render it.' },
  git: { label: 'Git', id: 'git-pane', scroll: '#git-body', empty: 'Open a project to see its changes.' },
  help: { label: 'Help', id: 'help-pane', scroll: '#help', empty: '' }
};
var PANE_NAMES = Object.keys(PANES);

// PAGE_PANES come with the page; the others with the project.
var PAGE_PANES = ['preview', 'copyedit'];

var groupSeq = 0;
var lastGroup = null; // the group a pane opened from the menu joins
var shownPanes = {}; // the panes in view after the last layout
var paneScroll = {}; // where each pane's scroller stood when it went out of view

function newGroup(tabs, active) {
  return { group: 'g' + (++groupSeq), tabs: tabs, active: active || tabs[0] };
}

function defaultLayout() {
  return newGroup(PANE_NAMES.slice(), 'preview');
}

// migrateLayout builds the dock from what the side-by-side panes before it
// left in localStorage: every pane that was open in a group of its own, in
// the order they stood, and the panes that were not -- the render and Git
// popups of the top bar among them -- as further tabs of the last group.
// Without any of it, the dock starts as one group holding every pane.
function migrateLayout() {
  var get = function (key) { return localStorage.getItem(key); };
  if (get('previewOpen') === null && get('copyeditOpen') === null && get('helpOpen') === null && get('paneOrder') === null) {
    return defaultLayout();
  }
  var open = {
    preview: get('previewOpen') !== 'closed',
    copyedit: get('copyeditOpen') === null ? get('paneTab') === 'copyedit' : get('copyeditOpen') === 'open',
    help: get('helpOpen') === 'open'
  };
  var order = (get('paneOrder') || '').split(',').concat(['preview', 'copyedit', 'help']).filter(function (name, i, all) {
    return open[name] !== undefined && all.indexOf(name) === i;
  });
  var groups = order.filter(function (name) { return open[name]; }).map(function (name) { return newGroup([name]); });
  if (!groups.length) {
    return null;
  }
  var last = groups[groups.length - 1];
  PANE_NAMES.forEach(function (name) {
    if (!open[name]) {
      last.tabs.push(name);
    }
  });
  return groups.length === 1 ? groups[0] : { dir: 'row', kids: groups, sizes: groups.map(function () { return 1; }) };
}

// normalize checks a layout read back or just changed: unknown and
// repeated panes go, a group left without tabs goes, a split left with one
// part is replaced by that part, a split inside a split of the same
// direction is merged into it, and the shares are scaled to add up to one.
function normalize(node, seen) {
  if (!node || typeof node !== 'object') {
    return null;
  }
  if (Array.isArray(node.tabs)) {
    var tabs = node.tabs.filter(function (name) {
      if (!PANES[name] || seen[name]) {
        return false;
      }
      seen[name] = true;
      return true;
    });
    if (!tabs.length) {
      return null;
    }
    return { group: node.group || 'g' + (++groupSeq), tabs: tabs, active: tabs.indexOf(node.active) >= 0 ? node.active : tabs[0] };
  }
  if (!Array.isArray(node.kids)) {
    return null;
  }
  var dir = node.dir === 'col' ? 'col' : 'row';
  var kids = [];
  var sizes = [];
  node.kids.forEach(function (kid, i) {
    var size = node.sizes && node.sizes[i] > 0 ? Number(node.sizes[i]) : 1;
    kid = normalize(kid, seen);
    if (!kid) {
      return;
    }
    if (kid.kids && kid.dir === dir) {
      kid.kids.forEach(function (inner, j) {
        kids.push(inner);
        sizes.push(size * kid.sizes[j]);
      });
      return;
    }
    kids.push(kid);
    sizes.push(size);
  });
  if (!kids.length) {
    return null;
  }
  if (kids.length === 1) {
    return kids[0];
  }
  var total = sizes.reduce(function (a, b) { return a + b; }, 0);
  return { dir: dir, kids: kids, sizes: sizes.map(function (s) { return s / total; }) };
}

function loadLayout() {
  var saved = null;
  try {
    saved = localStorage.getItem(DOCK_KEY);
  } catch (e) {
    // Storage off: the dock starts fresh every time.
  }
  if (saved === null) {
    return normalize(migrateLayout(), {});
  }
  try {
    return normalize(JSON.parse(saved), {});
  } catch (e) {
    return defaultLayout();
  }
}

var dock = loadLayout();

// saveLayout keeps the layout, without the group ids, which only name the
// groups on screen.
function saveLayout() {
  var strip = function (node) {
    return node.tabs ? { tabs: node.tabs, active: node.active } : { dir: node.dir, kids: node.kids.map(strip), sizes: node.sizes };
  };
  try {
    localStorage.setItem(DOCK_KEY, dock ? JSON.stringify(strip(dock)) : 'null');
  } catch (e) {
    // Out of room or storage off: the layout lasts this session only.
  }
}

// eachGroup calls fn for every group of the layout, in the order they
// stand, with the split holding it and its place there.
function eachGroup(fn) {
  (function walk(node, parent, at) {
    if (!node) {
      return;
    }
    if (node.tabs) {
      fn(node, parent, at);
      return;
    }
    node.kids.forEach(function (kid, i) { walk(kid, node, i); });
  })(dock, null, 0);
}

function findGroup(id) {
  var found = null;
  eachGroup(function (g) {
    if (g.group === id) {
      found = g;
    }
  });
  return found;
}

function groupOf(name) {
  var found = null;
  eachGroup(function (g) {
    if (g.tabs.indexOf(name) >= 0) {
      found = g;
    }
  });
  return found;
}

function parentOf(target) {
  var found = { parent: null, at: 0 };
  eachGroup(function (g, parent, at) {
    if (g === target) {
      found = { parent: parent, at: at };
    }
  });
  return found;
}

// commit settles a change to the layout: it is checked, kept, and shown.
function commit() {
  dock = normalize(dock, {});
  saveLayout();
  renderDock();
}

// takeTab takes the pane out of its group, the tab beside it becoming the
// active one if the pane was. The group may be left empty; commit drops it.
function takeTab(name) {
  var g = groupOf(name);
  if (!g) {
    return;
  }
  var at = g.tabs.indexOf(name);
  g.tabs.splice(at, 1);
  if (g.active === name) {
    g.active = g.tabs[Math.max(0, at - 1)];
  }
}

function paneOpen(name) {
  return !!groupOf(name);
}

// paneShown tells whether the pane is in view: open, and its tab the
// active one of its group.
function paneShown(name) {
  return !!shownPanes[name];
}

// openPane brings the pane into view: an open one by making its tab the
// active one, a closed one by adding it to the group last used.
function openPane(name) {
  if (!PANES[name]) {
    return;
  }
  var g = groupOf(name);
  if (!g) {
    g = findGroup(lastGroup);
    if (!g) {
      eachGroup(function (first) { g = g || first; });
    }
    if (g) {
      g.tabs.push(name);
    } else {
      dock = g = newGroup([name]);
    }
  }
  g.active = name;
  lastGroup = g.group;
  commit();
}

// closePane takes the pane out of the dock; the editor takes the room of a
// group left empty.
function closePane(name) {
  takeTab(name);
  commit();
}

function activateTab(name) {
  var g = groupOf(name);
  if (!g) {
    return;
  }
  g.active = name;
  lastGroup = g.group;
  saveLayout();
  applyDock();
}

// moveTab moves the pane to the group with id. With zone 'center' it joins
// the group's tabs at index (at the end without one); with 'left',
// 'right', 'top', or 'bottom' it gets a group of its own on that side of
// the group, which is split to make room.
function moveTab(name, id, zone, index) {
  var target = findGroup(id);
  var source = groupOf(name);
  if (!target || !source) {
    return;
  }
  if (zone === 'center') {
    var from = source === target ? target.tabs.indexOf(name) : -1;
    takeTab(name);
    var at = index === undefined || index === null ? target.tabs.length : index;
    if (from >= 0 && at > from) {
      at--; // the tab's own place, now gone, came before it
    }
    target.tabs.splice(Math.min(at, target.tabs.length), 0, name);
    target.active = name;
    lastGroup = target.group;
  } else {
    if (source === target && target.tabs.length === 1) {
      return; // a group split by its only pane is the same group
    }
    takeTab(name);
    var g = newGroup([name]);
    splitGroup(target, g, zone);
    lastGroup = g.group;
  }
  commit();
}

// splitGroup puts the group g on the side zone of target. A split in the
// direction the zone asks for gets g as one more part, beside target and
// with half its room; any other place gets a split of its own.
function splitGroup(target, g, zone) {
  var dir = zone === 'left' || zone === 'right' ? 'row' : 'col';
  var before = zone === 'left' || zone === 'top';
  var place = parentOf(target);
  var parent = place.parent;
  if (parent && parent.dir === dir) {
    var half = parent.sizes[place.at] / 2;
    parent.sizes[place.at] = half;
    var at = before ? place.at : place.at + 1;
    parent.kids.splice(at, 0, g);
    parent.sizes.splice(at, 0, half);
    return;
  }
  var split = { dir: dir, kids: before ? [g, target] : [target, g], sizes: [0.5, 0.5] };
  if (parent) {
    parent.kids[place.at] = split;
  } else {
    dock = split;
  }
}

function resetLayout() {
  dock = defaultLayout();
  commit();
}

// Drawing the dock

function paneElement(name) {
  return document.getElementById(PANES[name].id);
}

function keepScroll(name) {
  var el = paneElement(name);
  var scroller = el && el.querySelector(PANES[name].scroll);
  if (scroller && !el.hidden && el.offsetParent !== null) {
    paneScroll[name] = scroller.scrollTop;
  }
}

function restoreScroll(name) {
  var el = paneElement(name);
  var scroller = el && el.querySelector(PANES[name].scroll);
  if (scroller && paneScroll[name]) {
    scroller.scrollTop = paneScroll[name];
  }
}

// renderDock draws the layout anew and puts every pane where it stands:
// in its group, or in the store while it is closed.
function renderDock() {
  var host = document.getElementById('dock');
  var store = document.getElementById('pane-store');
  if (!host || !store) {
    return;
  }
  PANE_NAMES.forEach(function (name) {
    keepScroll(name);
    var el = paneElement(name);
    if (el) {
      store.appendChild(el);
    }
  });
  host.innerHTML = '';
  var divider = document.getElementById('dock-divider');
  host.hidden = !dock;
  if (divider) {
    divider.hidden = !dock;
  }
  if (dock) {
    host.appendChild(drawNode(dock));
    var width = parseFloat(localStorage.getItem(DOCK_WIDTH_KEY));
    host.style.width = (width > 0 ? width : 45) + '%';
  }
  applyDock();
}

function drawNode(node) {
  if (node.tabs) {
    return drawGroup(node);
  }
  var el = document.createElement('div');
  el.className = 'dock-split dock-' + node.dir;
  node.kids.forEach(function (kid, i) {
    if (i > 0) {
      var sash = document.createElement('div');
      sash.className = 'dock-sash';
      sash.setAttribute('role', 'separator');
      sash.setAttribute('aria-orientation', node.dir === 'row' ? 'vertical' : 'horizontal');
      sash.title = 'Drag to resize';
      sash.dockSplit = node;
      sash.dockAt = i - 1;
      el.appendChild(sash);
    }
    var part = drawNode(kid);
    part.style.flex = node.sizes[i] + ' 1 0';
    el.appendChild(part);
  });
  return el;
}

function drawGroup(g) {
  var el = document.createElement('div');
  el.className = 'dock-group';
  el.dataset.group = g.group;
  var bar = document.createElement('div');
  bar.className = 'dock-tabs';
  bar.setAttribute('role', 'tablist');
  var body = document.createElement('div');
  body.className = 'dock-body';
  g.tabs.forEach(function (name) {
    var tab = document.createElement('div');
    tab.className = 'dock-tab';
    tab.dataset.pane = name;
    tab.setAttribute('role', 'tab');
    tab.tabIndex = 0;
    tab.draggable = true;
    tab.title = PANES[name].label + ' — drag to move it, or onto the edge of a group to split it';
    var label = document.createElement('span');
    label.className = 'dock-tab-label';
    label.textContent = PANES[name].label;
    var close = document.createElement('button');
    close.type = 'button';
    close.className = 'dock-tab-close';
    close.title = 'Close ' + PANES[name].label;
    close.setAttribute('aria-label', 'Close ' + PANES[name].label);
    close.textContent = '×';
    tab.appendChild(label);
    tab.appendChild(close);
    bar.appendChild(tab);
    var pane = paneElement(name);
    if (!pane) {
      pane = document.createElement('p');
      pane.className = 'empty dock-empty';
      pane.dataset.pane = name;
      pane.textContent = PANES[name].empty;
    }
    body.appendChild(pane);
  });
  var drop = document.createElement('div');
  drop.className = 'dock-drop';
  drop.hidden = true;
  body.appendChild(drop);
  el.appendChild(bar);
  el.appendChild(body);
  return el;
}

// applyDock shows the active pane of every group and hides the others,
// marks the tabs and the menu to match, and fires folio-shown at every
// pane that has just come into view.
function applyDock() {
  var now = {};
  document.querySelectorAll('#dock .dock-group').forEach(function (el) {
    var g = findGroup(el.dataset.group);
    if (!g) {
      return;
    }
    el.querySelectorAll('.dock-tab').forEach(function (tab) {
      var on = tab.dataset.pane === g.active;
      tab.classList.toggle('active', on);
      tab.setAttribute('aria-selected', on ? 'true' : 'false');
    });
    el.querySelectorAll('.dock-body > [data-pane]').forEach(function (pane) {
      var on = pane.dataset.pane === g.active;
      if (!on && !pane.hidden) {
        keepScroll(pane.dataset.pane);
      }
      pane.hidden = !on;
      if (on && pane.classList.contains('side-pane')) {
        now[pane.dataset.pane] = true;
      }
    });
  });
  var before = shownPanes;
  shownPanes = now;
  Object.keys(now).forEach(function (name) {
    restoreScroll(name);
    if (!before[name]) {
      var el = paneElement(name);
      if (window.htmx) {
        htmx.process(el); // a pane just swapped in may not be wired up yet
      }
      el.dispatchEvent(new CustomEvent('folio-shown', { bubbles: true }));
    }
  });
  fillPaneMenu();
}

// adoptPanes makes way for the panes a swap just brought: a page switch
// (#content) brings the page's own panes, which replace those of the page
// before, and changes what the comments pane lists; an /open (#main)
// brings everything anew. It runs first thing after the swap, so that no
// id stands twice in the page; app.js then lays the dock out (renderDock)
// once the editor is mounted.
function adoptPanes(target) {
  if (target.id === 'content') {
    PAGE_PANES.forEach(function (name) {
      document.querySelectorAll('[id="' + PANES[name].id + '"]').forEach(function (el) {
        if (!target.contains(el)) {
          el.remove();
        }
      });
      delete shownPanes[name];
      delete paneScroll[name];
    });
    delete shownPanes.comments;
  } else {
    shownPanes = {};
  }
}

document.body.addEventListener('htmx:afterSwap', function (evt) {
  var target = evt.detail && evt.detail.target;
  if (target && (target.id === 'content' || target.id === 'main')) {
    adoptPanes(target);
  }
});

// Settling puts back the attributes the swapped-in panes came with,
// hidden among them, on those whose id stood in the page before -- every
// pane, after an /open. The dock's say is put back after it.
document.body.addEventListener('htmx:afterSettle', function (evt) {
  var target = evt.detail && evt.detail.target;
  if (target && (target.id === 'content' || target.id === 'main')) {
    applyDock();
  }
});

// The Panes menu: every pane, ticked while it is open. Picking one opens
// or closes it; the last entry puts the layout back as it started.
function fillPaneMenu() {
  var menu = document.getElementById('pane-menu');
  if (!menu) {
    return;
  }
  menu.innerHTML = '';
  PANE_NAMES.forEach(function (name) {
    var item = document.createElement('button');
    item.type = 'button';
    item.className = 'pane-menu-item';
    item.dataset.pane = name;
    item.setAttribute('role', 'menuitemcheckbox');
    item.setAttribute('aria-checked', paneOpen(name) ? 'true' : 'false');
    item.textContent = PANES[name].label;
    menu.appendChild(item);
  });
  var reset = document.createElement('button');
  reset.type = 'button';
  reset.className = 'pane-menu-item pane-menu-reset';
  reset.setAttribute('role', 'menuitem');
  reset.textContent = 'Reset layout';
  reset.title = 'Every pane back in one group beside the editor';
  menu.appendChild(reset);
}

function showPaneMenu(show) {
  var menu = document.getElementById('pane-menu');
  var button = document.getElementById('pane-menu-button');
  if (menu && button) {
    menu.hidden = !show;
    button.setAttribute('aria-expanded', show ? 'true' : 'false');
  }
}

document.addEventListener('click', function (evt) {
  var t = evt.target;
  if (!t.closest) {
    return;
  }
  if (t.closest('#pane-menu-button')) {
    var menu = document.getElementById('pane-menu');
    showPaneMenu(menu && menu.hidden);
    return;
  }
  var item = t.closest('.pane-menu-item');
  if (item) {
    if (item.classList.contains('pane-menu-reset')) {
      resetLayout();
    } else if (paneOpen(item.dataset.pane)) {
      closePane(item.dataset.pane);
    } else {
      openPane(item.dataset.pane);
    }
    showPaneMenu(false);
    refreshEditor();
    return;
  }
  if (!t.closest('#pane-menu')) {
    showPaneMenu(false);
  }
  var close = t.closest('.dock-tab-close');
  if (close) {
    closePane(close.closest('.dock-tab').dataset.pane);
    refreshEditor(); // the editor may have gained the dock's room
    return;
  }
  var tab = t.closest('.dock-tab');
  if (tab) {
    activateTab(tab.dataset.pane);
  }
});

// A middle click closes a tab, as it does in a browser.
document.addEventListener('auxclick', function (evt) {
  var tab = evt.button === 1 && evt.target.closest && evt.target.closest('.dock-tab');
  if (tab) {
    evt.preventDefault();
    closePane(tab.dataset.pane);
    refreshEditor();
  }
});

document.addEventListener('keydown', function (evt) {
  if (evt.key === 'Escape') {
    showPaneMenu(false);
  }
  var tab = evt.target.closest && evt.target.closest('.dock-tab');
  if (tab && (evt.key === 'Enter' || evt.key === ' ')) {
    evt.preventDefault();
    activateTab(tab.dataset.pane);
  }
});

// Resizing
//
// The divider between the editor and the dock trades width between the
// two; a sash between two parts of a split trades room between them.

document.addEventListener('pointerdown', function (evt) {
  if (evt.button !== 0 || !evt.target.closest) {
    return;
  }
  var divider = evt.target.closest('#dock-divider');
  var sash = evt.target.closest('.dock-sash');
  if (divider) {
    dragDockWidth(evt, divider);
  } else if (sash) {
    dragSash(evt, sash);
  }
});

function dragWith(evt, el, onMove, onUp) {
  evt.preventDefault();
  el.setPointerCapture(evt.pointerId);
  el.classList.add('dragging');
  function up() {
    el.removeEventListener('pointermove', onMove);
    el.removeEventListener('pointerup', up);
    el.removeEventListener('pointercancel', up);
    el.classList.remove('dragging');
    onUp();
  }
  el.addEventListener('pointermove', onMove);
  el.addEventListener('pointerup', up);
  el.addEventListener('pointercancel', up);
}

function dragDockWidth(evt, divider) {
  var host = document.getElementById('dock');
  var content = document.getElementById('content');
  if (!host || !content) {
    return;
  }
  var room = host.getBoundingClientRect().width + content.getBoundingClientRect().width;
  var right = host.getBoundingClientRect().right;
  var minDock = parseFloat(getComputedStyle(host).minWidth) || 0;
  var minContent = parseFloat(getComputedStyle(content).minWidth) || 0;
  dragWith(evt, divider, function (e) {
    var width = Math.max(minDock, Math.min(right - e.clientX, room - minContent));
    host.style.width = (width / room * 100) + '%';
    refreshEditor(); // CodeMirror measures its own width
  }, function () {
    localStorage.setItem(DOCK_WIDTH_KEY, parseFloat(host.style.width));
  });
}

function dragSash(evt, sash) {
  var split = sash.dockSplit;
  var at = sash.dockAt;
  var row = split.dir === 'row';
  var a = sash.previousElementSibling;
  var b = sash.nextElementSibling;
  var size = function (el) { var r = el.getBoundingClientRect(); return row ? r.width : r.height; };
  var startA = size(a);
  var startB = size(b);
  var pair = split.sizes[at] + split.sizes[at + 1];
  var start = row ? evt.clientX : evt.clientY;
  var min = Math.min(80, (startA + startB) / 2);
  dragWith(evt, sash, function (e) {
    var d = (row ? e.clientX : e.clientY) - start;
    var newA = Math.max(min, Math.min(startA + d, startA + startB - min));
    split.sizes[at] = pair * newA / (startA + startB);
    split.sizes[at + 1] = pair - split.sizes[at];
    a.style.flex = split.sizes[at] + ' 1 0';
    b.style.flex = split.sizes[at + 1] + ' 1 0';
  }, saveLayout);
}

// Moving tabs
//
// A tab is dragged with the browser's own drag and drop, under a type of
// its own so that the editor does not take the drop for text. Over a
// group's tabs it joins them where the pointer is; over a group's body it
// joins the group in the middle, or splits it at the edge nearest the
// pointer, a quarter of the body deep. The part of the body it would take
// is shaded while the pointer is there.

var draggedTab = null;

document.addEventListener('dragstart', function (evt) {
  var tab = evt.target.closest && evt.target.closest('.dock-tab');
  if (!tab) {
    return;
  }
  draggedTab = tab.dataset.pane;
  evt.dataTransfer.effectAllowed = 'move';
  evt.dataTransfer.setData(PANE_TYPE, draggedTab);
  tab.classList.add('dragging');
});

// dropZone is where a drop at the pointer would put the tab dragged over
// the group el: the zone, and for 'center' the place among the tabs.
function dropZone(el, evt) {
  var bar = el.querySelector('.dock-tabs');
  var body = el.querySelector('.dock-body');
  var r = bar.getBoundingClientRect();
  if (evt.clientY <= r.bottom) {
    var tabs = Array.prototype.slice.call(bar.querySelectorAll('.dock-tab'));
    var index = tabs.length;
    for (var i = 0; i < tabs.length; i++) {
      var t = tabs[i].getBoundingClientRect();
      if (evt.clientX < t.left + t.width / 2) {
        index = i;
        break;
      }
    }
    return { zone: 'center', index: index, bar: true };
  }
  r = body.getBoundingClientRect();
  var x = (evt.clientX - r.left) / r.width;
  var y = (evt.clientY - r.top) / r.height;
  var edges = { left: x, right: 1 - x, top: y, bottom: 1 - y };
  var zone = 'center';
  var nearest = 0.25;
  Object.keys(edges).forEach(function (edge) {
    if (edges[edge] < nearest) {
      nearest = edges[edge];
      zone = edge;
    }
  });
  return { zone: zone, index: null };
}

function clearDrop() {
  document.querySelectorAll('#dock .dock-drop').forEach(function (el) {
    el.hidden = true;
  });
  document.querySelectorAll('#dock .drop-before, #dock .drop-end, #dock .dock-tab.dragging').forEach(function (el) {
    el.classList.remove('drop-before', 'drop-end', 'dragging');
  });
}

document.addEventListener('dragover', function (evt) {
  if (!draggedTab) {
    return;
  }
  var el = evt.target.closest && evt.target.closest('.dock-group');
  clearDrop();
  if (!el) {
    return;
  }
  evt.preventDefault();
  evt.dataTransfer.dropEffect = 'move';
  var drop = dropZone(el, evt);
  if (drop.bar) {
    var tabs = el.querySelectorAll('.dock-tab');
    if (drop.index < tabs.length) {
      tabs[drop.index].classList.add('drop-before');
    } else {
      el.querySelector('.dock-tabs').classList.add('drop-end');
    }
    return;
  }
  var shade = el.querySelector('.dock-drop');
  shade.className = 'dock-drop dock-drop-' + drop.zone;
  shade.hidden = false;
});

document.addEventListener('drop', function (evt) {
  if (!draggedTab) {
    return;
  }
  var el = evt.target.closest && evt.target.closest('.dock-group');
  var name = draggedTab;
  draggedTab = null;
  clearDrop();
  if (!el) {
    return;
  }
  evt.preventDefault();
  var drop = dropZone(el, evt);
  moveTab(name, el.dataset.group, drop.zone, drop.index);
  refreshEditor();
});

document.addEventListener('dragend', function () {
  draggedTab = null;
  clearDrop();
});
