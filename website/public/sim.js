// Hero simulation: a toy model of the Peregrine engine. Eight connections download one
// file; when a connection finishes it takes over half of the slowest remaining segment
// (dynamic segmentation). Part-way through, one connection is reset by the "server"
// and resumes from the byte where it stopped. Runs only while visible.
(function () {
  "use strict";
  var data = JSON.parse(document.getElementById("i18n").textContent);
  var S = data.sim, LANES = data.lanes;

  // Main button: point at the visitor's platform.
  (function () {
    var ua = navigator.userAgent;
    var phone = /Android|iPhone|iPad|iPod/.test(ua) || (navigator.maxTouchPoints > 1 && /Macintosh/.test(ua));
    var os = "windows";
    if (/Mac OS X|Macintosh/.test(ua) && !phone) os = "macos";
    else if (/Linux|X11/.test(ua) && !phone) os = "linux";
    var cta = document.getElementById("cta");
    var arrow = ' <span aria-hidden="true">↓</span>';
    if (phone) { cta.href = "#download"; cta.innerHTML = data.cta.phone + arrow; return; }
    cta.href = "/download/" + os; cta.innerHTML = data.cta[os] + arrow;
    var row = document.querySelector('.dl a[data-os="' + os + '"]');
    if (row) row.closest("li,tr").classList.add("you");
  })();

  // Close the language menu when clicking elsewhere or pressing Escape.
  var langMenu = document.querySelector(".lang");
  if (langMenu) {
    document.addEventListener("click", function (e) { if (!langMenu.contains(e.target)) langMenu.open = false; });
    document.addEventListener("keydown", function (e) { if (e.key === "Escape") langMenu.open = false; });
  }

  // Newest version number (same origin, CDN cached).
  fetch("/api/dl?p=info").then(function (r) { return r.ok ? r.json() : null; }).then(function (j) {
    if (j && j.version) document.querySelectorAll(".v").forEach(function (e) { e.textContent = j.version; });
  }).catch(function () {});

  var root = document.querySelector(".sim");
  if (!root) return;
  var mapEl = root.querySelector(".map");
  var laneEls = Array.prototype.slice.call(root.querySelectorAll(".lanes li"));
  var pctEl = root.querySelector(".sim-pct"), spdEl = root.querySelector(".sim-spd");
  var etaEl = root.querySelector(".sim-eta"), msgEl = root.querySelector(".sim-msg");
  var replay = root.querySelector(".replay");

  var MB = 1048576, SIZE = 3790 * MB, N = 8, DROP_LANE = 5, DROP_AT = 0.36;
  var segs, lanes, nextId, dropped, phase, phaseAt, clock, raf = 0, last = 0, visible = false;

  function reset() {
    segs = []; lanes = []; nextId = 0; dropped = false; phase = "run"; phaseAt = 0; clock = 0;
    mapEl.innerHTML = "";
    var part = SIZE / N;
    for (var i = 0; i < N; i++) {
      var base = data.speeds[i] * MB;
      lanes.push({ base: base, speed: base, target: base, retarget: 0, seg: null, status: "", cls: "", until: 0, hotUntil: 0 });
      addSeg(i, Math.round(i * part), Math.round((i + 1) * part));
    }
    setMsg("", "");
    replay.hidden = true;
  }

  function addSeg(lane, start, end) {
    var el = document.createElement("div");
    el.className = "seg";
    el.style.setProperty("--c", LANES[lane]);
    el.appendChild(document.createElement("i"));
    mapEl.appendChild(el);
    var s = { id: nextId++, lane: lane, start: start, end: end, pos: start, el: el };
    segs.push(s);
    lanes[lane].seg = s;
    return s;
  }

  function status(l, text, cls, secs) { l.status = text; l.cls = cls; l.until = clock + secs; }
  function setMsg(text, cls) { msgEl.textContent = text; msgEl.className = "sim-msg" + (cls ? " " + cls : ""); }

  function steal(laneIdx) {
    var best = null, bestRem = 0;
    for (var i = 0; i < segs.length; i++) {
      var s = segs[i], rem = s.end - s.pos;
      if (s.pos < s.end && lanes[s.lane].seg === s && rem > bestRem) { best = s; bestRem = rem; }
    }
    var l = lanes[laneIdx];
    if (!best || bestRem < 40 * MB) { l.seg = null; status(l, S.idle, "", 1e9); return; }
    var mid = Math.round(best.pos + bestRem / 2);
    var victim = best.lane;
    var oldEnd = best.end;
    best.end = mid;
    addSeg(laneIdx, mid, oldEnd);
    status(l, S.split + (victim + 1), "info", 1.6);
  }

  function step(dt) {
    clock += dt;
    if (phase === "run") {
      var done = 0;
      for (var i = 0; i < N; i++) {
        var l = lanes[i];
        if (clock > l.retarget) { l.target = l.base * (0.7 + Math.random() * 0.6); l.retarget = clock + 0.5 + Math.random() * 0.8; }
        l.speed += (l.target - l.speed) * Math.min(1, dt * 3);
        if (!l.seg) continue;
        if (clock < l.hotUntil) continue; // connection reset: no bytes
        var s = l.seg;
        s.pos = Math.min(s.end, s.pos + l.speed * dt);
        if (s.pos >= s.end) steal(i);
      }
      for (var k = 0; k < segs.length; k++) done += segs[k].pos - segs[k].start;
      var frac = done / SIZE;
      if (!dropped && frac > DROP_AT) {
        dropped = true;
        var d = lanes[DROP_LANE];
        d.hotUntil = clock + 1.8;
        status(d, S.reset, "hot", 1.8);
        setMsg("#" + (DROP_LANE + 1) + " " + S.reset, "hot");
        setTimeoutClock(1.8, function () {
          status(d, S.resumed, "ok", 2.4);
          setMsg("#" + (DROP_LANE + 1) + " " + S.resumed, "ok");
        });
      }
      var active = segs.some(function (s) { return s.pos < s.end; });
      if (!active) { phase = "verify"; phaseAt = clock; setMsg(S.verifying, ""); }
    } else if (phase === "verify" && clock - phaseAt > 1.3) {
      phase = "done"; phaseAt = clock; setMsg("✓ " + S.done, "ok"); replay.hidden = false;
    } else if (phase === "done" && clock - phaseAt > 6) {
      reset();
    }
    runTimers();
  }

  var timers = [];
  function setTimeoutClock(secs, fn) { timers.push({ at: clock + secs, fn: fn }); }
  function runTimers() {
    for (var i = timers.length - 1; i >= 0; i--) if (clock >= timers[i].at) { var f = timers[i].fn; timers.splice(i, 1); f(); }
  }

  function fmtTime(sec) {
    sec = Math.max(0, Math.ceil(sec));
    return Math.floor(sec / 60) + ":" + ("0" + (sec % 60)).slice(-2);
  }

  function draw() {
    var done = 0, speed = 0;
    for (var i = 0; i < segs.length; i++) {
      var s = segs[i], w = s.end - s.start;
      done += s.pos - s.start;
      s.el.style.insetInlineStart = (s.start / SIZE * 100) + "%";
      s.el.style.width = (w / SIZE * 100) + "%";
      s.el.firstChild.style.width = (w > 0 ? (s.pos - s.start) / w * 100 : 100) + "%";
      s.el.classList.toggle("hot", lanes[s.lane].seg === s && clock < lanes[s.lane].hotUntil);
    }
    for (var j = 0; j < N; j++) {
      var l = lanes[j], el = laneEls[j];
      var working = phase === "run" && l.seg && clock >= l.hotUntil;
      var v = working ? l.speed / MB : 0;
      speed += v;
      el.querySelector(".lb i").style.width = Math.min(100, v / 40 * 100) + "%";
      el.querySelector(".ls").textContent = v.toFixed(1) + " MB/s";
      var lt = el.querySelector(".lt");
      if (clock > l.until && l.cls !== "") { l.status = l.seg ? "" : S.idle; l.cls = ""; }
      var text = l.status;
      if (!text && l.seg) text = Math.round((l.seg.end - l.seg.pos) / MB) + " MB " + S.left;
      lt.textContent = text;
      lt.className = "lt" + (l.cls ? " " + l.cls : "");
      el.classList.toggle("hot", clock < l.hotUntil);
    }
    var frac = Math.min(1, done / SIZE);
    pctEl.textContent = (phase === "run" ? Math.floor(frac * 100) : 100) + "%";
    spdEl.textContent = speed.toFixed(1);
    etaEl.textContent = (speed > 0 ? fmtTime((SIZE - done) / MB / speed) : "0:00") + " " + S.left;
  }

  function loop(ts) {
    raf = 0;
    if (!visible || document.hidden) { last = 0; return; }
    var dt = last ? Math.min(0.1, (ts - last) / 1000) : 0;
    last = ts;
    step(dt);
    draw();
    raf = requestAnimationFrame(loop);
  }
  function wake() { if (!raf && visible && !document.hidden) raf = requestAnimationFrame(loop); }

  reset();

  if (window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches) {
    // No animation: run the whole download instantly and show the result.
    for (var g = 0; g < 20000 && phase !== "done"; g++) step(0.05);
    draw();
    replay.hidden = true;
    return;
  }

  replay.addEventListener("click", function () { reset(); wake(); });
  document.addEventListener("visibilitychange", wake);
  if ("IntersectionObserver" in window) {
    new IntersectionObserver(function (es) { visible = es[0].isIntersecting; wake(); }).observe(root);
  } else { visible = true; wake(); }
})();
