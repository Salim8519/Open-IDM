// Renders one language version of the landing page. Strings come from src/i18n/*.json.
// Strings are written by us (trusted) and may contain inline HTML; attribute values are escaped.

const SITE = "https://peregrinedownloader.app";
const GH = "https://github.com/Salim8519/Peregrine";

const esc = (s) =>
  String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const strip = (s) => String(s).replace(/<[^>]+>/g, "");
const url = (p) => SITE + (p === "/" ? "/" : p);

// 8 lane colours for the simulation, also used for the lane chips.
const LANES = ["#5b8cff", "#22d3ee", "#a78bfa", "#34d399", "#fbbf24", "#f472b6", "#60a5fa", "#2dd4bf"];
const START_SPEEDS = [36, 15, 28, 8, 31, 21, 34, 13];

export function render(t, all, version) {
  const alternates = all
    .map((l) => `<link rel="alternate" hreflang="${l.lang}" href="${url(l.path)}">`)
    .join("\n") + `\n<link rel="alternate" hreflang="x-default" href="${url("/")}">`;

  const langMenu = all
    .map((l) =>
      l.lang === t.lang
        ? `<li><span aria-current="true">${esc(l.name)}</span></li>`
        : `<li><a href="${l.path}" hreflang="${l.lang}" lang="${l.lang}">${esc(l.name)}</a></li>`)
    .join("");

  const ld = {
    "@context": "https://schema.org",
    "@graph": [
      {
        "@type": "SoftwareApplication",
        name: "Peregrine",
        applicationCategory: "UtilitiesApplication",
        operatingSystem: "Windows 10, Windows 11, macOS 12+, Linux (CLI)",
        description: t.meta.description,
        inLanguage: t.lang,
        url: url(t.path),
        downloadUrl: `${GH}/releases`,
        softwareVersion: version,
        license: "https://www.gnu.org/licenses/gpl-3.0.html",
        image: `${SITE}/img/main-dark.png`,
        offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
      },
      {
        "@type": "FAQPage",
        inLanguage: t.lang,
        mainEntity: t.faq.items.map(([q, a]) => ({
          "@type": "Question",
          name: q,
          acceptedAnswer: { "@type": "Answer", text: strip(a) },
        })),
      },
    ],
  };

  // Static first frame of the simulation (what you see before / without JavaScript).
  const segs = START_SPEEDS.map((_, i) => {
    const fill = [72, 34, 61, 18, 66, 49, 70, 27][i];
    return `<div class="seg" style="--c:${LANES[i]};inset-inline-start:${i * 12.5}%;width:12.5%"><i style="width:${fill}%"></i></div>`;
  }).join("");
  const lanes = START_SPEEDS.map((s, i) =>
    `<li style="--c:${LANES[i]}"><b>#${i + 1}</b><span class="lb"><i style="width:${Math.round((s / 40) * 100)}%"></i></span><span class="ls">${s.toFixed(1)} MB/s</span><span class="lt"></span></li>`
  ).join("");

  const cell = (v) => {
    const [k, ...rest] = v.split(":");
    return `<td class="${k}">${rest.join(":")}</td>`;
  };

  const dlRows = t.download.rows.map(([plat, detail, p1, l1, p2, l2]) => `
        <tr>
          <th scope="row">${plat}<small>${detail}</small></th>
          <td class="dl-go">
            <a class="btn ${p1 === "extension" ? "ghost" : "solid"}" href="/download/${p1}" data-os="${p1}">${l1} <span aria-hidden="true">↓</span></a>
            ${p2 ? `<a class="alt" href="/download/${p2}">${l2}</a>` : ""}
          </td>
        </tr>`).join("");

  return `<!doctype html>
<html lang="${t.lang}" dir="${t.dir}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(t.meta.title)}</title>
<meta name="description" content="${esc(t.meta.description)}">
<link rel="canonical" href="${url(t.path)}">
${alternates}
<meta name="theme-color" content="#f3f1ec" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#0e0f11" media="(prefers-color-scheme: dark)">
<meta name="color-scheme" content="light dark">
<link rel="icon" href="/img/peregrine.svg" type="image/svg+xml">
<link rel="icon" href="/img/favicon-32.png" sizes="32x32" type="image/png">
<link rel="apple-touch-icon" href="/img/icon-512.png">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Peregrine">
<meta property="og:locale" content="${t.lang === "ar" ? "ar_AR" : "en_US"}">
<meta property="og:url" content="${url(t.path)}">
<meta property="og:title" content="${esc(t.meta.ogTitle)}">
<meta property="og:description" content="${esc(t.meta.ogDescription)}">
<meta property="og:image" content="${SITE}/img/main-dark.png">
<meta name="twitter:card" content="summary_large_image">
<script type="application/ld+json">${JSON.stringify(ld)}</script>
<style>
:root{
  --paper:#f3f1ec; --paper2:#fbfaf7; --ink:#15171b; --ink2:#5d616a; --rule:#d8d3c8; --rule2:#e6e2d9;
  --signal:#2f5fe8; --hot:#e5484d; --ok:#178a57; --warn:#a36a00;
  --panel:#101216; --panel2:#181b21; --pline:#262a32; --ptext:#e8e7e3; --pdim:#8a8f99;
  --mono:ui-monospace,"Cascadia Mono","SF Mono",Menlo,Consolas,monospace;
  --sans:system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;
}
@media (prefers-color-scheme:dark){:root{
  --paper:#0e0f11; --paper2:#15171a; --ink:#ecebe6; --ink2:#9b9ea6; --rule:#2a2c31; --rule2:#1f2125;
  --signal:#7d9bff; --hot:#ff6b6f; --ok:#45c98a; --warn:#e3a43a;
  --panel:#16181c; --panel2:#1d2026; --pline:#2b2f37;
}}
*{box-sizing:border-box}
[hidden]{display:none!important}
html{-webkit-text-size-adjust:100%;scroll-behavior:smooth}
body{margin:0;background:var(--paper);color:var(--ink);font:17px/1.6 var(--sans);text-rendering:optimizeLegibility}
:lang(ar) body,:lang(ar){font-family:"Segoe UI","SF Arabic","Noto Sans Arabic",Tahoma,var(--sans)}
a{color:inherit;text-decoration-thickness:1px;text-underline-offset:3px}
img{max-width:100%;height:auto;display:block}
code,.mono{font-family:var(--mono)}
.wrap{max-width:1180px;margin:0 auto;padding-inline:20px}
.skip{position:absolute;inset-inline-start:-999px}.skip:focus{inset-inline-start:12px;top:12px;background:var(--paper2);padding:8px;z-index:9}

/* header */
.top{border-bottom:1px solid var(--rule)}
.top .wrap{display:flex;align-items:center;gap:24px;height:64px}
.brand{display:flex;align-items:center;gap:10px;font-weight:800;font-size:19px;text-decoration:none;letter-spacing:-.01em}
.brand img{width:28px;height:28px}
.top nav{display:flex;gap:22px;margin-inline-start:auto;font-size:14px}
.top nav a{text-decoration:none;color:var(--ink2);padding-block:12px}
.top nav a:hover{color:var(--ink)}
.lang{position:relative;font-size:14px}
.lang summary{list-style:none;cursor:pointer;border:1px solid var(--rule);border-radius:6px;padding:7px 12px;display:flex;gap:8px;align-items:center;user-select:none}
.lang summary::-webkit-details-marker{display:none}
.lang summary svg{width:15px;height:15px}
.lang ul{position:absolute;inset-inline-end:0;top:calc(100% + 6px);margin:0;padding:6px;list-style:none;background:var(--paper2);border:1px solid var(--rule);border-radius:8px;min-width:150px;z-index:5;box-shadow:0 10px 30px rgba(0,0,0,.08)}
.lang li a,.lang li span{display:block;padding:9px 12px;border-radius:5px;text-decoration:none}
.lang li a:hover{background:var(--rule2)}
.lang li span{color:var(--ink2)}
.lang li span::after{content:" ✓"}
@media (max-width:860px){.top nav a.opt{display:none}}
@media (max-width:520px){.top nav{display:none}.lang{margin-inline-start:auto}}

/* hero */
.hero{display:grid;grid-template-columns:1.05fr 1fr;gap:56px;align-items:center;padding-block:72px 64px}
@media (max-width:960px){.hero{grid-template-columns:1fr;gap:40px;padding-block:44px 48px}}
.kicker{font:13px/1.4 var(--mono);color:var(--ink2);margin:0 0 22px;letter-spacing:.01em}
h1{font-size:clamp(46px,7vw,94px);line-height:.92;letter-spacing:-.045em;font-weight:820;margin:0 0 26px}
:lang(ar) .kicker,:lang(ar) .label,:lang(ar) .facts,:lang(ar) .shot figcaption,:lang(ar) .tests thead th,:lang(ar) .cmp thead th,:lang(ar) .pass{font-family:inherit;letter-spacing:0}
:lang(ar) .kicker,:lang(ar) .facts{font-size:14px}
:lang(ar) h1{letter-spacing:0;line-height:1.12;font-weight:800}
.lead{font-size:clamp(17px,1.6vw,19.5px);color:var(--ink2);max-width:36em;margin:0 0 32px}
.lead::first-line{color:var(--ink)}
.cta{display:flex;flex-wrap:wrap;gap:10px 22px;align-items:center}
.btn{display:inline-flex;align-items:center;gap:10px;padding:14px 20px;border-radius:6px;font-weight:650;text-decoration:none;font-size:16px;border:1px solid var(--ink);line-height:1.2}
.btn.solid{background:var(--ink);color:var(--paper)}
.btn.solid:hover{background:var(--signal);border-color:var(--signal);color:#fff}
.btn.ghost{background:transparent}
.btn.ghost:hover{background:var(--ink);color:var(--paper)}
.link{font-weight:600;padding-block:12px}
.facts{margin:22px 0 0;font:13px var(--mono);color:var(--ink2)}

/* simulation panel */
.sim{background:var(--panel);color:var(--ptext);border:1px solid var(--pline);border-radius:12px;padding:20px 20px 16px;font:13px/1.4 var(--mono);box-shadow:0 1px 0 var(--pline) inset,0 30px 60px -30px rgba(10,12,20,.45);direction:ltr}
.sim-top{display:flex;justify-content:space-between;align-items:baseline;gap:16px}
.sim-file{color:var(--pdim);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.sim-pct{font-size:40px;font-weight:700;letter-spacing:-.03em;color:#fff;font-variant-numeric:tabular-nums;line-height:1}
.sim-sub{display:flex;gap:18px;color:var(--pdim);margin:8px 0 16px;font-variant-numeric:tabular-nums;flex-wrap:wrap}
.sim-sub b{color:var(--ptext);font-weight:600}
.map{position:relative;height:26px;background:var(--panel2);border-radius:4px;overflow:hidden}
.seg{position:absolute;top:0;bottom:0;border-inline-end:2px solid var(--panel)}
.seg i{position:absolute;inset-block:0;inset-inline-start:0;background:var(--c);opacity:.95}
.seg.hot i{background:var(--hot)!important;animation:blink .5s steps(2) infinite}
@keyframes blink{50%{opacity:.35}}
.lanes{list-style:none;margin:16px 0 0;padding:0;display:grid;gap:6px}
.lanes li{display:grid;grid-template-columns:28px 1fr 82px minmax(0,1.3fr);gap:10px;align-items:center;color:var(--pdim);font-variant-numeric:tabular-nums}
.lanes b{color:var(--c);font-weight:600}
.lb{height:4px;background:var(--panel2);border-radius:2px;overflow:hidden}
.lb i{display:block;height:100%;background:var(--c);transition:width .3s}
.ls{text-align:right;color:var(--ptext)}
.lt{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.lt.hot{color:var(--hot)}.lt.ok{color:#5fd39a}.lt.info{color:var(--ptext)}
.lanes li.hot .ls{color:var(--hot)}
.sim-foot{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-top:14px;padding-top:12px;border-top:1px solid var(--pline);min-height:40px}
.sim-msg{color:var(--pdim)}
.sim-msg.ok{color:#5fd39a}.sim-msg.hot{color:var(--hot)}
.replay{font:inherit;color:var(--ptext);background:transparent;border:1px solid var(--pline);border-radius:5px;padding:7px 12px;cursor:pointer}
.replay:hover{border-color:var(--pdim)}
@media (max-width:520px){.lanes li{grid-template-columns:24px 1fr 74px}.lt{display:none}.sim-pct{font-size:32px}}

/* sections: spec-sheet layout */
.sec{border-top:1px solid var(--rule);padding-block:64px}
.sec .wrap{display:grid;grid-template-columns:200px minmax(0,1fr);gap:40px}
.sec .wrap>*,.hero>*{min-width:0}
@media (max-width:860px){.sec{padding-block:48px}.sec .wrap{grid-template-columns:1fr;gap:14px}}
.label{font:13px var(--mono);color:var(--ink2);padding-top:8px;margin:0}
h2{font-size:clamp(30px,4vw,48px);line-height:1.05;letter-spacing:-.03em;font-weight:780;margin:0 0 18px;max-width:18em}
:lang(ar) h2{letter-spacing:0;line-height:1.25}
.body{color:var(--ink2);max-width:40em;margin:0 0 32px}
.body a,.fine a{color:var(--ink)}

.stats{display:grid;grid-template-columns:repeat(3,1fr);border-top:1px solid var(--ink)}
.stats div{padding:18px 20px 4px 0;border-inline-end:1px solid var(--rule)}
.stats div:last-child{border-inline-end:0}
.stats div+div{padding-inline-start:20px}
.stats b{display:block;font:700 clamp(40px,5vw,64px)/1 var(--mono);letter-spacing:-.04em;margin-bottom:10px}
.stats span{color:var(--ink2);font-size:15px}
@media (max-width:640px){.stats{grid-template-columns:1fr}.stats div{border-inline-end:0;border-bottom:1px solid var(--rule);padding:16px 0!important}}

.shot{margin:44px 0 0}
.shot img{border-radius:10px;border:1px solid var(--rule);box-shadow:0 30px 60px -35px rgba(10,12,20,.5)}
.shot figcaption{font:13px var(--mono);color:var(--ink2);margin-top:12px}
table{border-collapse:collapse;width:100%}
.tests{border-top:1px solid var(--ink)}
.tests th,.tests td{text-align:start;padding:16px 12px 16px 0;border-bottom:1px solid var(--rule);vertical-align:top}
.tests thead th{font:12px var(--mono);color:var(--ink2);font-weight:400;padding-block:10px}
.tests tbody th{font-weight:650;width:38%}
.tests td.r{color:var(--ink2)}
.tests td.p{width:64px;text-align:end;padding-inline-end:0}
.pass{font:600 12px var(--mono);color:var(--ok);border:1px solid currentColor;border-radius:4px;padding:2px 7px;text-transform:uppercase;letter-spacing:.04em}
.fine{font-size:14px;color:var(--ink2);margin:16px 0 0}
@media (max-width:640px){.tests thead{display:none}.tests tr{display:grid;grid-template-columns:1fr auto;border-bottom:1px solid var(--rule);padding-block:14px}.tests th,.tests td{border:0;padding:0;width:auto!important}.tests td.r{grid-column:1/-1;grid-row:2;margin-top:4px}}

.grid6{display:grid;grid-template-columns:repeat(3,1fr);border-top:1px solid var(--ink)}
.grid6 div{padding:20px 22px 22px 0;border-bottom:1px solid var(--rule)}
.grid6 div:not(:nth-child(3n)){border-inline-end:1px solid var(--rule)}
.grid6 div:not(:nth-child(3n+1)){padding-inline-start:22px}
.grid6 h3{font-size:17px;margin:0 0 6px}
.grid6 p{margin:0;color:var(--ink2);font-size:15px}
@media (max-width:860px){.grid6{grid-template-columns:1fr 1fr}.grid6 div{border-inline-end:0!important;padding-inline:0!important}.grid6 div:nth-child(odd){border-inline-end:1px solid var(--rule)!important;padding-inline-end:20px!important}.grid6 div:nth-child(even){padding-inline-start:20px!important}}
@media (max-width:560px){.grid6{grid-template-columns:1fr}.grid6 div{border-inline-end:0!important;padding-inline:0!important}}

.dl{border-top:1px solid var(--ink)}
.dl th{text-align:start;font-weight:700;font-size:19px;padding:20px 12px 20px 0;border-bottom:1px solid var(--rule)}
.dl th small{display:block;font-weight:400;font-size:14px;color:var(--ink2);margin-top:2px}
.dl td{padding:16px 0;border-bottom:1px solid var(--rule);text-align:end;white-space:nowrap}
.dl tr.you th::before{content:"●";color:var(--signal);margin-inline-end:8px;font-size:12px;vertical-align:middle}
.alt{display:inline-block;padding:12px 4px;margin-inline-start:14px;font-size:15px;color:var(--ink2)}
.alt:hover{color:var(--ink)}
@media (max-width:640px){.dl tr{display:block;border-bottom:1px solid var(--rule);padding-block:16px}.dl th,.dl td{display:block;border:0;padding:0;text-align:start;white-space:normal}.dl td{margin-top:12px}}
.ver{font-family:var(--mono);color:var(--ink)}
.note{margin-top:22px;font-size:14px;color:var(--ink2);max-width:46em}
.note a{color:var(--ink)}

.tw{overflow-x:auto;border-top:1px solid var(--ink)}
.cmp{min-width:680px;font-size:15px}
.cmp th,.cmp td{text-align:start;padding:12px 14px 12px 0;border-bottom:1px solid var(--rule);vertical-align:top}
.cmp thead th{font:12px var(--mono);color:var(--ink2);font-weight:400}
.cmp thead th.us{color:var(--ink);font-weight:700}
.cmp tbody th{font-weight:500;color:var(--ink2)}
.cmp td:nth-child(2){font-weight:650}
.y{color:var(--ink)}.y::before{content:"✓ ";color:var(--ok)}
.n{color:var(--ink2)}.n::before{content:"✕ ";color:var(--hot)}
.m{color:var(--ink2)}.m::before{content:"~ ";color:var(--warn)}

pre{direction:ltr;text-align:left;font:14px/1.75 var(--mono);background:var(--panel);color:var(--ptext);border-radius:10px;padding:20px 22px;overflow-x:auto;margin:0}
pre .c{color:var(--pdim)}pre .p{color:#5fd39a;user-select:none}

#faq details{border-bottom:1px solid var(--rule)}
#faq details:first-of-type{border-top:1px solid var(--ink)}
#faq summary{cursor:pointer;font-weight:650;font-size:18px;list-style:none;padding:18px 0;display:flex;gap:14px}
#faq summary::-webkit-details-marker{display:none}
#faq summary::before{content:"+";font-family:var(--mono);color:var(--signal);width:14px;flex:none}
#faq details[open] summary::before{content:"−"}
#faq details p{margin:0 0 20px;padding-inline-start:28px;color:var(--ink2);max-width:44em}

footer{border-top:1px solid var(--rule);padding-block:36px 56px;font-size:14px;color:var(--ink2)}
footer .wrap{display:flex;flex-wrap:wrap;gap:12px 28px;justify-content:space-between}
footer a{display:inline-block;padding-block:8px;margin-inline-end:16px}
</style>
</head>
<body>
<a class="skip" href="#main">${t.nav.skip}</a>
<header class="top">
  <div class="wrap">
    <a class="brand" href="${t.path}"><img src="/img/peregrine.svg" alt="" width="28" height="28">Peregrine</a>
    <nav aria-label="Main">
      <a class="opt" href="#how">${t.nav.how}</a>
      <a class="opt" href="#tests">${t.nav.tests}</a>
      <a href="#download">${t.nav.download}</a>
      <a class="opt" href="#compare">${t.nav.compare}</a>
      <a href="${GH}">GitHub</a>
    </nav>
    <details class="lang">
      <summary aria-label="${esc(t.nav.language)}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 2.6 3.8 5.6 3.8 9s-1.3 6.4-3.8 9c-2.5-2.6-3.8-5.6-3.8-9S9.5 5.6 12 3z"/></svg>${esc(t.name)}</summary>
      <ul>${langMenu}</ul>
    </details>
  </div>
</header>

<main id="main">
  <div class="wrap hero">
    <div>
      <p class="kicker">${t.hero.kicker}</p>
      <h1>${t.hero.title}</h1>
      <p class="lead">${t.hero.lead}</p>
      <div class="cta">
        <a class="btn solid" id="cta" href="/download/windows">${t.hero.ctaWindows} <span aria-hidden="true">↓</span></a>
        <a class="link" href="${GH}">${t.hero.ctaGithub} <span aria-hidden="true">${t.dir === "rtl" ? "←" : "→"}</span></a>
      </div>
      <p class="facts">v<span class="v">${version}</span> · ${t.hero.facts}</p>
    </div>

    <figure class="sim" aria-label="${esc(t.sim.label)}" style="margin:0">
      <div class="sim-top"><span class="sim-file">${t.sim.file}</span><span class="sim-pct">57%</span></div>
      <div class="sim-sub"><span><b class="sim-spd">186.0</b> MB/s</span><span class="sim-eta">0:12 ${t.sim.left}</span><span>8 ${t.sim.connections}</span></div>
      <div class="map">${segs}</div>
      <ol class="lanes">${lanes}</ol>
      <div class="sim-foot"><span class="sim-msg" aria-live="polite"></span><button class="replay" type="button" hidden>↻ ${t.sim.replay}</button></div>
    </figure>
  </div>

  <section class="sec" id="how">
    <div class="wrap">
      <p class="label">${t.how.label}</p>
      <div>
        <h2>${t.how.title}</h2>
        <p class="body">${t.how.body}</p>
        <div class="stats">${t.how.stats.map(([n, s]) => `<div><b>${n}</b><span>${s}</span></div>`).join("")}</div>
        <figure class="shot"><img src="/img/main-dark.png" width="2560" height="1560" loading="lazy" alt="${esc(t.how.shotAlt)}"><figcaption>${t.how.shotCap}</figcaption></figure>
      </div>
    </div>
  </section>

  <section class="sec" id="tests">
    <div class="wrap">
      <p class="label">${t.tests.label}</p>
      <div>
        <h2>${t.tests.title}</h2>
        <p class="body">${t.tests.body}</p>
        <table class="tests">
          <thead><tr><th scope="col">${t.tests.colServer}</th><th scope="col">${t.tests.colPeregrine}</th><th scope="col"></th></tr></thead>
          <tbody>${t.tests.rows.map(([a, b]) => `<tr><th scope="row">${a}</th><td class="r">${b}</td><td class="p"><span class="pass">${t.tests.pass}</span></td></tr>`).join("")}</tbody>
        </table>
        <p class="fine">${t.tests.note.replace("internal/testsrv", `<a href="${GH}/tree/main/internal/testsrv"><code>internal/testsrv</code></a>`)}</p>
      </div>
    </div>
  </section>

  <section class="sec" id="more">
    <div class="wrap">
      <p class="label">${t.more.label}</p>
      <div>
        <h2>${t.more.title}</h2>
        <div class="grid6">${t.more.items.map(([h, p]) => `<div><h3>${h}</h3><p>${p}</p></div>`).join("")}</div>
      </div>
    </div>
  </section>

  <section class="sec" id="download">
    <div class="wrap">
      <p class="label">${t.download.label}</p>
      <div>
        <h2>${t.download.title}</h2>
        <p class="body">${t.download.body} <span class="ver v">${version}</span></p>
        <table class="dl"><tbody>${dlRows}</tbody></table>
        <p class="note">${t.download.unsigned}</p>
        <p class="note"><a href="${GH}/releases">${t.download.allFiles}</a></p>
      </div>
    </div>
  </section>

  <section class="sec" id="compare">
    <div class="wrap">
      <p class="label">${t.compare.label}</p>
      <div>
        <h2>${t.compare.title}</h2>
        <p class="body">${t.compare.body}</p>
        <div class="tw"><table class="cmp">
          <thead><tr><th scope="col"></th><th scope="col" class="us">Peregrine</th><th scope="col">IDM</th><th scope="col">FDM</th><th scope="col">${t.compare.browser}</th></tr></thead>
          <tbody>${t.compare.rows.map(([h, ...c]) => `<tr><th scope="row">${h}</th>${c.map(cell).join("")}</tr>`).join("")}</tbody>
        </table></div>
        <p class="fine">${t.compare.fine}</p>
      </div>
    </div>
  </section>

  <section class="sec" id="cli">
    <div class="wrap">
      <p class="label">${t.cli.label}</p>
      <div>
        <h2>${t.cli.title}</h2>
        <p class="body">${t.cli.body}</p>
<pre><span class="c">${t.cli.c1}</span>
<span class="p">$ </span>peregrine get https://example.com/big.iso -o ~/Downloads -c 16 --limit 5M

<span class="c">${t.cli.c2}</span>
<span class="p">$ </span>peregrine add https://example.com/file.zip</pre>
      </div>
    </div>
  </section>

  <section class="sec" id="privacy">
    <div class="wrap">
      <p class="label">${t.privacy.label}</p>
      <div>
        <h2>${t.privacy.title}</h2>
        <p class="body" style="margin:0">${t.privacy.body}</p>
      </div>
    </div>
  </section>

  <section class="sec" id="faq">
    <div class="wrap">
      <p class="label">${t.faq.label}</p>
      <div>
        <h2>${t.faq.title}</h2>
        ${t.faq.items.map(([q, a]) => `<details><summary>${q}</summary><p>${a}</p></details>`).join("\n        ")}
      </div>
    </div>
  </section>
</main>

<footer>
  <div class="wrap">
    <span>${t.footer.tagline}<br>${t.footer.license}</span>
    <span><a href="${GH}">GitHub</a><a href="${GH}/releases">${t.footer.releases}</a><a href="${GH}/blob/main/docs/CHANGELOG.md">${t.footer.changelog}</a><a href="${GH}/blob/main/SECURITY.md">${t.footer.security}</a></span>
  </div>
</footer>

<script type="application/json" id="i18n">${JSON.stringify({ sim: t.sim, cta: { windows: t.hero.ctaWindows, macos: t.hero.ctaMac, linux: t.hero.ctaLinux, phone: t.hero.ctaPhone }, lanes: LANES, speeds: START_SPEEDS })}</script>
<script src="/sim.js" defer></script>
</body>
</html>
`;
}
