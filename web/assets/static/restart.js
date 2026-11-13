// Foolscap Folio - wait for folio web to come back after an update
//
// After an update folio web restarts itself on the new binary. An element
// with data-restart-to names the release it restarts with; the page asks
// /version until that release answers, then loads itself again. When it
// does not answer within a minute, the element says to restart by hand.
(function () {
  var TRIES = 60;

  function wait(el) {
    if (el.dataset.waiting) {
      return;
    }
    el.dataset.waiting = '1';
    var want = el.dataset.restartTo;
    var tries = 0;
    var timer = setInterval(function () {
      if (++tries > TRIES) {
        clearInterval(timer);
        el.textContent = 'folio ' + want + ' is installed. Restart folio web to use it.';
        return;
      }
      // A server that answers /version with 404 is a release from before
      // it had one: a different binary from this one, so it is back too.
      fetch('/version', { cache: 'no-store' })
        .then(function (r) { return r.status === 404 ? want : r.ok ? r.text() : ''; })
        .then(function (v) {
          if (v.trim() === want) {
            clearInterval(timer);
            // The Config page was posted to; load it rather than post again.
            location.replace(location.pathname.indexOf('/config') === 0 ? '/config' : '/');
          }
        })
        .catch(function () { /* still restarting */ });
    }, 1000);
  }

  function scan() {
    document.querySelectorAll('[data-restart-to]').forEach(wait);
  }

  document.addEventListener('DOMContentLoaded', scan);
  document.addEventListener('htmx:afterSwap', scan);
})();
