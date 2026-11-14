// Renders one language version of the landing page ("field guide" design).
// Strings come from src/i18n/*.json. They are written by us (trusted) and may contain
// inline HTML; attribute values are escaped.

const SITE = "https://peregrinedownloader.app";
const GH = "https://github.com/Salim8519/Peregrine";

const esc = (s) =>
  String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const strip = (s) => String(s).replace(/<[^>]+>/g, "");
const url = (p) => SITE + (p === "/" ? "/" : p);

// Earthy lane colours for Plate 2 (light, then night).
const LANES = ["#9c4a23", "#2f5d62", "#b07d1a", "#5b6b2e", "#7a3b5e", "#3f5873", "#a8613a", "#4c7a5a"];
const START_SPEEDS = [36, 15, 28, 8, 31, 21, 34, 13];

const falcon = (labels) => `
<svg class="falcon" viewBox="0 0 600 360" role="img" aria-hidden="true">
  <g transform="translate(300 40) scale(.62) translate(-256 -84)" stroke-width="2.4">
    <path class="f-body" d="M256 84c30 0 50 24 50 54 0 70-30 150-50 232-20-82-50-162-50-232 0-30 20-54 50-54z"/>
    <path class="f-wing" d="M70 132c78 10 136 56 170 128l16 110C196 288 140 210 70 132z"/>
    <path class="f-wing" d="M442 132c-78 10-136 56-170 128l-16 110c60-82 116-160 186-238z"/>
    <path class="f-line" d="M120 170c40 20 80 50 110 92M392 170c-40 20-80 50-110 92M150 200c30 18 56 40 80 70M362 200c-30 18-56 40-80 70M236 150c6 40 12 80 20 120M276 150c-6 40-12 80-20 120"/>
    <circle class="f-eye" cx="242" cy="118" r="6"/><circle class="f-eye" cx="270" cy="118" r="6"/>
    <rect class="f-tray" x="164" y="410" width="184" height="30" rx="15"/>
  </g>
  <g class="f-lead"><path d="M215 92 L95 62"/><path d="M382 130 L505 100"/><path d="M305 232 L455 285"/><path d="M300 305 L140 322"/></g>
  <path class="f-arrow" d="M300 300 v40 M291 330 l9 12 9-12"/>
  <g class="f-label"><text x="95" y="52">${labels[0]}</text><text x="505" y="90">${labels[1]}</text><text x="455" y="306">${labels[2]}</text><text x="140" y="346">${labels[3]}</text></g>
</svg>`;

export function render(t, all, version) {
  const rtl = t.dir === "rtl";
  const alternates =
    all.map((l) => `<link rel="alternate" hreflang="${l.lang}" href="${url(l.path)}">`).join("\n") +
    `\n<link rel="alternate" hreflang="x-default" href="${url("/")}">`;
  const others = all.filter((l) => l.lang !== t.lang);
  const langSwitch =
    others.length === 1
      ? `<a class="lang" href="${others[0].path}" hreflang="${others[0].lang}" lang="${others[0].lang}">${esc(others[0].name)}</a>`
      : `<details class="lang"><summary>${esc(t.name)}</summary><ul>${others
          .map((l) => `<li><a href="${l.path}" hreflang="${l.lang}" lang="${l.lang}">${esc(l.name)}</a></li>`)
          .join("")}</ul></details>`;

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
        mainEntity: t.faq.items.map(([q, a]) => ({ "@type": "Question", name: q, acceptedAnswer: { "@type": "Answer", text: strip(a) } })),
      },
    ],
  };

  const segs = START_SPEEDS.map((_, i) => {
    const fill = [72, 34, 61, 18, 66, 49, 70, 27][i];
    return `<div class="seg" style="--c:${LANES[i]};inset-inline-start:${i * 12.5}%;width:12.5%"><i style="width:${fill}%"></i></div>`;
  }).join("");
  const lanes = START_SPEEDS.map(
    (s, i) =>
      `<li style="--c:${LANES[i]}"><b>#${i + 1}</b><span class="lb"><i style="width:${Math.round((s / 40) * 100)}%"></i></span><span class="ls">${s.toFixed(1)} MB/s</span><span class="lt"></span></li>`
  ).join("");
  const cell = (v) => {
    const [k, ...rest] = v.split(":");
    return `<td class="${k}">${rest.join(":")}</td>`;
  };
  const arrow = rtl ? "←" : "→";

  return `<!doctype html>
<html lang="${t.lang}" dir="${t.dir}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(t.meta.title)}</title>
<meta name="description" content="${esc(t.meta.description)}">
<link rel="canonical" href="${url(t.path)}">
${alternates}
<meta name="theme-color" content="#efe8d6" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#1c1914" media="(prefers-color-scheme: dark)">
<meta name="color-scheme" content="light dark">
${rtl ? '<link rel="preload" href="/fonts/NotoNaskhArabic-arabic.woff2" as="font" type="font/woff2" crossorigin>' : ""}
<link rel="icon" href="/img/peregrine.svg" type="image/svg+xml">
<link rel="icon" href="/img/favicon-32.png" sizes="32x32" type="image/png">
<link rel="apple-touch-icon" href="/img/icon-512.png">
<meta property="og:type" content="website">
<meta property="og:site_name" content="Peregrine">
<meta property="og:locale" content="${rtl ? "ar_AR" : "en_US"}">
<meta property="og:url" content="${url(t.path)}">
<meta property="og:title" content="${esc(t.meta.ogTitle)}">
<meta property="og:description" content="${esc(t.meta.ogDescription)}">
<meta property="og:image" content="${SITE}/img/main-dark.png">
<meta name="twitter:card" content="summary_large_image">
<script type="application/ld+json">${JSON.stringify(ld)}</script>
<style>
@font-face{font-family:"Noto Naskh Arabic";font-style:normal;font-weight:400 700;font-display:swap;
  src:url(/fonts/NotoNaskhArabic-arabic.woff2) format("woff2");
  unicode-range:U+0600-06FF,U+0750-077F,U+0870-08FF,U+200C-200E,U+2010-2011,U+204F,U+2E41,U+FB50-FDFF,U+FE70-FEFC}
:root{
  --p:#efe8d6;--p2:#e6dcc4;--p3:#dccfb2;--k:#2a2419;--g:#6e634f;--rule:#2a2419;--soft:#c9bc9f;
  --rust:#9c4a23;--sea:#2f5d62;--ok:#4d6b2e;--hot:#b3361f;
  --serif:"Noto Naskh Arabic",Georgia,"Iowan Old Style","Palatino Linotype",Palatino,"Times New Roman",serif;
  --mono:ui-monospace,"Cascadia Mono","SF Mono",Menlo,Consolas,monospace;
}
@media (prefers-color-scheme:dark){:root{
  --p:#1c1914;--p2:#25211a;--p3:#2e291f;--k:#ebe2cc;--g:#a79b82;--rule:#6d6350;--soft:#4a4335;
  --rust:#e0916a;--sea:#86b9be;--ok:#a9c27c;--hot:#f07a5f;
}}
*{box-sizing:border-box}
[hidden]{display:none!important}
html{-webkit-text-size-adjust:100%}
body{margin:0;background:var(--p);color:var(--k);font:18px/1.75 var(--serif)}
:lang(ar) body{line-height:1.95}
a{color:inherit;text-underline-offset:3px;text-decoration-thickness:1px}
.w{max-width:980px;margin-inline:auto;padding-inline:20px}
.skip{position:absolute;inset-inline-start:-999px}.skip:focus{inset-inline-start:12px;top:12px;background:var(--p2);padding:8px;z-index:9}
.sc{font-variant:small-caps;letter-spacing:.04em}
:lang(ar) .sc{font-variant:normal;letter-spacing:0}

.top{display:flex;align-items:center;gap:16px;min-height:56px;border-bottom:1px solid var(--rule);font-size:15px;color:var(--g)}
.top .book{flex:1;min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.top .book a{text-decoration:none;color:var(--k);font-weight:700;display:inline-block;padding-block:10px}
.top nav{display:flex;gap:16px;align-items:center}
.top nav a{text-decoration:none;padding-block:10px}
.lang{border:1px solid var(--g);padding:3px 12px!important;color:var(--k);text-decoration:none;white-space:nowrap}
details.lang{position:relative;padding:0}
details.lang summary{cursor:pointer;list-style:none;padding:3px 12px}
details.lang ul{position:absolute;inset-inline-end:0;top:100%;margin:4px 0 0;padding:4px;list-style:none;background:var(--p2);border:1px solid var(--rule);z-index:5}
details.lang a{display:block;padding:6px 12px;text-decoration:none}
@media (max-width:560px){.top .ed{display:none}.top nav .dlnav{display:none}}

.entry{text-align:center;padding-block:30px 8px}
.no{font-size:15px;color:var(--rust);margin:0}
h1{font-weight:700;font-size:clamp(42px,10vw,84px);line-height:1.15;margin:4px 0 2px}
:lang(en) h1{font-weight:400;letter-spacing:-.01em}
.sci{font:italic 20px Georgia,"Times New Roman",serif;color:var(--g);direction:ltr;margin:0}
.plate{margin:20px auto 0;max-width:640px;border:1px solid var(--rule);background:var(--p2);padding:12px}
.plate figcaption{font-size:15px;color:var(--g);font-style:italic;margin-top:6px}
.falcon{display:block;width:100%;height:auto;stroke:var(--k);fill:none;stroke-linecap:round;stroke-linejoin:round}
.f-body{fill:var(--p)}.f-wing{fill:var(--p3)}.f-line{stroke-width:1.4}
.f-eye{fill:var(--k);stroke:none}.f-tray{fill:var(--rust);stroke:var(--rust)}
.f-lead path{stroke:var(--g);stroke-width:1;stroke-dasharray:3 4}
.f-arrow{stroke:var(--rust);stroke-width:2.2}
.f-label text{fill:var(--k);stroke:none;font:17px var(--serif);text-anchor:middle}
.lead{font-size:clamp(19px,2.4vw,21px);max-width:31em;margin:22px auto 0}
.cta{display:flex;flex-wrap:wrap;gap:10px;justify-content:center;margin-top:22px}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:10px;min-height:52px;padding:0 26px;border:1px solid var(--k);text-decoration:none;font-weight:700;font-size:18px}
.btn.solid{background:var(--k);color:var(--p)}
.btn.solid:hover{background:var(--rust);border-color:var(--rust);color:#fff}
.btn.line:hover{background:var(--p2)}
.facts{font-size:14px;color:var(--g);margin-top:14px}
@media (max-width:560px){.cta .btn{flex:1 1 100%}}

section{border-top:1px solid var(--rule);padding-block:34px}
h2{font-weight:700;font-size:clamp(28px,6vw,40px);line-height:1.3;margin:0 0 10px}
:lang(en) h2{font-weight:400}
.body{color:var(--g);max-width:38em;margin:0 0 18px}

.notes{display:grid;border-top:1px solid var(--rule)}
.note{display:grid;grid-template-columns:120px minmax(0,1fr);gap:14px;border-bottom:1px solid var(--rule);padding-block:16px}
.note h3{font-size:16px;color:var(--rust);margin:0;padding-top:3px;font-weight:700}
:lang(en) .note h3{font-variant:small-caps;letter-spacing:.05em;font-size:18px;font-weight:400}
.note p{margin:0}
.note small{display:block;color:var(--g);font-size:16px;margin-top:4px}
@media (max-width:560px){.note{grid-template-columns:1fr;gap:2px}}
@media (min-width:820px){.notes{grid-template-columns:1fr 1fr}.note:nth-child(odd){border-inline-end:1px solid var(--rule);padding-inline-end:20px}.note:nth-child(even){padding-inline-start:20px}}

.sim{margin:0;border:1px solid var(--rule);background:var(--p2);padding:16px;direction:ltr;font-variant-numeric:tabular-nums}
.sim-top{display:flex;justify-content:space-between;align-items:baseline;gap:14px}
.sim-file{color:var(--g);font:14px var(--mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.sim-pct{font:700 40px/1 Georgia,serif}
.sim-sub{display:flex;flex-wrap:wrap;gap:6px 18px;color:var(--g);font-size:15px;margin:6px 0 12px}
.sim-sub b{color:var(--k)}
.map{position:relative;height:24px;background:var(--p);border:1px solid var(--soft)}
.seg{position:absolute;top:0;bottom:0;border-inline-end:2px solid var(--p2)}
.seg i{position:absolute;inset-block:0;inset-inline-start:0;background:var(--c)}
.seg.hot i{background:var(--hot)!important;animation:blink .5s steps(2) infinite}
@keyframes blink{50%{opacity:.35}}
.lanes{list-style:none;margin:12px 0 0;padding:0;display:grid;gap:4px;font-size:15px}
.lanes li{display:grid;grid-template-columns:30px minmax(0,1fr) 84px minmax(0,1.3fr);gap:10px;align-items:center;color:var(--g)}
.lanes b{color:var(--c)}
@media (prefers-color-scheme:dark){.lanes b{filter:brightness(1.6)}.seg i{filter:brightness(1.25)}}
.lb{height:3px;background:var(--soft)}
.lb i{display:block;height:100%;background:var(--c);transition:width .3s}
.ls{text-align:right;color:var(--k)}
.lt{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-style:italic}
:lang(ar) .lt,:lang(ar) .sim-msg{direction:rtl;text-align:right;font-style:normal}
.lt.hot,.lanes li.hot .ls{color:var(--hot)}.lt.ok{color:var(--ok)}.lt.info{color:var(--k)}
.sim-foot{display:flex;justify-content:space-between;align-items:center;gap:12px;margin-top:12px;padding-top:10px;border-top:1px dotted var(--rule);min-height:42px}
.sim-msg{color:var(--g);font-style:italic}.sim-msg.ok{color:var(--ok)}.sim-msg.hot{color:var(--hot)}
.replay{font:inherit;font-size:15px;color:var(--k);background:none;border:1px solid var(--g);padding:5px 12px;cursor:pointer}
.simcap{font-size:15px;color:var(--g);font-style:italic;margin-top:8px}
@media (max-width:560px){.lanes li{grid-template-columns:26px minmax(0,1fr) 76px}.lt{display:none}.sim-pct{font-size:32px}}

.log{list-style:none;margin:0;padding:0;border-top:1px solid var(--rule)}
.log li{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:4px 20px;border-bottom:1px solid var(--rule);padding-block:12px}
.log li span{color:var(--g)}
.log li span::before{content:attr(data-o);color:var(--ok);font-weight:700;margin-inline-end:8px}
:lang(en) .log li span::before{font-variant:small-caps;letter-spacing:.05em}
@media (max-width:640px){.log li{grid-template-columns:1fr}}
.fine{font-size:15px;color:var(--g);margin:12px 0 0}
.fine code{font-size:14px}

.spp{display:grid;gap:12px}
.sp{border:1px solid var(--rule);padding:14px 16px;background:var(--p2)}
.sp b{font-size:19px}
.sp i{font:italic 15px Georgia,serif;color:var(--g);margin-inline-start:8px}
.sp ul{margin:6px 0 0;padding-inline-start:20px;color:var(--g);font-size:16px}
.sp.me{background:var(--k);color:var(--p);border-color:var(--k)}.sp.me i,.sp.me ul{color:var(--p3)}
@media (prefers-color-scheme:dark){.sp.me i,.sp.me ul{color:#4a4335}}
@media (min-width:820px){.spp{grid-template-columns:repeat(3,1fr)}}
.full{margin-top:14px;border-top:1px solid var(--rule)}
.full summary{cursor:pointer;padding-block:12px;font-weight:700;list-style:none}
.full summary::before{content:"+ ";color:var(--rust)}
.full[open] summary::before{content:"− "}
.tw{overflow-x:auto}
table{border-collapse:collapse;width:100%;min-width:620px;font-size:16px}
th,td{text-align:start;padding:9px 12px 9px 0;border-bottom:1px solid var(--soft);vertical-align:top}
thead th{font-size:14px;color:var(--g);font-weight:400}
tbody th{font-weight:400;color:var(--g)}
td:nth-child(2){font-weight:700}
.y::before{content:"✓ ";color:var(--ok)}.n{color:var(--g)}.n::before{content:"✕ ";color:var(--hot)}.m{color:var(--g)}.m::before{content:"~ ";color:var(--rust)}

pre{direction:ltr;text-align:left;font:15px/1.8 var(--mono);background:var(--p2);border:1px solid var(--rule);padding:16px 18px;overflow-x:auto;margin:0}
pre .c{color:var(--g)}pre .pr{color:var(--rust);user-select:none}

.faq details{border-bottom:1px solid var(--rule)}
.faq details:first-of-type{border-top:1px solid var(--rule)}
.faq summary{cursor:pointer;list-style:none;padding-block:14px;font-weight:700;display:flex;gap:12px}
.faq summary::-webkit-details-marker{display:none}
.faq summary::before{content:"+";color:var(--rust);width:12px;flex:none}
.faq details[open] summary::before{content:"−"}
.faq details p{margin:0 0 16px;padding-inline-start:24px;color:var(--g)}

.dl{list-style:none;margin:0;padding:0;border-top:1px solid var(--rule)}
.dl li{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:2px 14px;align-items:center;border-bottom:1px dotted var(--rule);padding-block:12px}
.dl li.you{background:linear-gradient(to left,transparent,var(--p2) 30%,var(--p2) 70%,transparent)}
.dl .name{font-size:21px}
.dl .name small{display:block;font-size:15px;color:var(--g)}
.dl .go{display:inline-flex;align-items:center;gap:8px;min-height:46px;padding:0 18px;border:1px solid var(--k);text-decoration:none;font-weight:700;white-space:nowrap}
.dl .go:hover{background:var(--k);color:var(--p)}
.dl .alt{grid-column:1/-1;font-size:15px;color:var(--g)}
.dl .alt a{padding-block:6px;display:inline-block}
.note2{font-size:15px;color:var(--g);margin-top:14px;max-width:44em}
.ver{font-family:Georgia,serif;color:var(--k);font-weight:700}

footer{border-top:1px solid var(--rule);padding-block:24px 44px;text-align:center;font-size:15px;color:var(--g);font-style:italic}
footer nav{font-style:normal;margin-top:8px}
footer nav a{display:inline-block;padding:6px 8px}
</style>
</head>
<body>
<a class="skip" href="#main">${t.nav.skip}</a>
<div class="w">
<header class="top">
  <span class="book"><a href="${t.path}">Peregrine</a> · ${t.nav.book}<span class="ed"> · ${t.nav.edition} <span class="v">${version}</span></span></span>
  <nav aria-label="Main"><a class="dlnav" href="#find">${t.nav.download}</a><a href="${GH}">GitHub</a>${langSwitch}</nav>
</header>

<main id="main">
  <div class="entry">
    <p class="no">${t.entry.no}</p>
    <h1>${t.entry.title}</h1>
    <p class="sci">Falco peregrinus digitalis</p>
    <figure class="plate">${falcon(t.entry.labels)}<figcaption>${t.entry.plate}</figcaption></figure>
    <p class="lead">${t.entry.lead}</p>
    <div class="cta">
      <a class="btn solid" id="cta" href="/download/windows">${t.entry.ctaWindows} <span aria-hidden="true">↓</span></a>
      <a class="btn line" href="${GH}">${t.entry.ctaGithub}</a>
    </div>
    <p class="facts">v<span class="v">${version}</span> · ${t.entry.facts}</p>
  </div>

  <section id="notes">
    <h2>${t.notes.title}</h2>
    <div class="notes">${t.notes.items.map(([h, p, s]) => `<div class="note"><h3>${h}</h3><p>${p}<small>${s}</small></p></div>`).join("")}</div>
  </section>

  <section id="dive">
    <h2>${t.sim.title}</h2>
    <figure class="sim" aria-label="${esc(t.sim.label)}">
      <div class="sim-top"><span class="sim-file">${t.sim.file}</span><span class="sim-pct">57%</span></div>
      <div class="sim-sub"><span><b class="sim-spd">186.0</b> MB/s</span><span class="sim-eta">0:12 ${t.sim.left}</span><span>8 ${t.sim.connections}</span></div>
      <div class="map">${segs}</div>
      <ol class="lanes">${lanes}</ol>
      <div class="sim-foot"><span class="sim-msg" aria-live="polite"></span><button class="replay" type="button" hidden>↻ ${t.sim.replay}</button></div>
    </figure>
    <p class="simcap">${t.sim.caption}</p>
  </section>

  <section id="lab">
    <h2>${t.lab.title}</h2>
    <p class="body">${t.lab.body}</p>
    <ul class="log">${t.lab.rows.map(([a, b]) => `<li><b>${a}</b><span data-o="${esc(t.lab.observed)}:">${b}</span></li>`).join("")}</ul>
    <p class="fine">${t.lab.note.replace("internal/testsrv", `<a href="${GH}/tree/main/internal/testsrv"><code>internal/testsrv</code></a>`)}</p>
  </section>

  <section id="species">
    <h2>${t.species.title}</h2>
    <p class="body">${t.species.body}</p>
    <div class="spp">${t.species.cards.map(([n, s, li], i) => `<div class="sp${i === 0 ? " me" : ""}"><b>${n}</b><i>${s}</i><ul>${li.map((x) => `<li>${x}</li>`).join("")}</ul></div>`).join("")}</div>
    <details class="full"><summary>${t.species.full}</summary>
      <div class="tw"><table>
        <thead><tr><th scope="col"></th><th scope="col">Peregrine</th><th scope="col">IDM</th><th scope="col">FDM</th><th scope="col">${t.species.browser}</th></tr></thead>
        <tbody>${t.species.rows.map(([h, ...c]) => `<tr><th scope="row">${h}</th>${c.map(cell).join("")}</tr>`).join("")}</tbody>
      </table></div>
      <p class="fine">${t.species.fine}</p>
    </details>
  </section>

  <section id="call">
    <h2>${t.call.title}</h2>
    <p class="body">${t.call.body}</p>
<pre><span class="c">${t.call.c1}</span>
<span class="pr">$ </span>peregrine get https://example.com/big.iso -o ~/Downloads -c 16 --limit 5M

<span class="c">${t.call.c2}</span>
<span class="pr">$ </span>peregrine add https://example.com/file.zip</pre>
  </section>

  <section id="find">
    <h2>${t.range.title}</h2>
    <p class="body">${t.range.body} <span class="ver v">${version}</span></p>
    <ul class="dl">${t.range.rows
      .map(
        ([n, d, p1, p2, l2]) =>
          `<li><span class="name">${n}<small>${d}</small></span><a class="go" href="/download/${p1}" data-os="${p1}">${t.range.go} <span aria-hidden="true">↓</span></a>${
            p2 ? `<span class="alt"><a href="/download/${p2}">${l2} ${arrow}</a></span>` : ""
          }</li>`
      )
      .join("")}</ul>
    <p class="note2">${t.range.unsigned}</p>
    <p class="note2"><a href="${GH}/releases">${t.range.all}</a></p>
  </section>

  <section id="faq" class="faq">
    <h2>${t.faq.title}</h2>
    ${t.faq.items.map(([q, a]) => `<details><summary>${q}</summary><p>${a}</p></details>`).join("\n    ")}
  </section>
</main>

<footer>
  ${t.footer.species}<br>${t.footer.named}
  <nav><a href="${GH}">GitHub</a> · <a href="${GH}/releases">${t.footer.releases}</a> · <a href="${GH}/blob/main/docs/CHANGELOG.md">${t.footer.changelog}</a> · <a href="${GH}/blob/main/SECURITY.md">${t.footer.security}</a> · <a href="${GH}/blob/main/docs/SECURITY-MODEL.md">${t.footer.model}</a></nav>
</footer>
</div>

<script type="application/json" id="i18n">${JSON.stringify({ sim: t.sim, cta: { windows: t.entry.ctaWindows, macos: t.entry.ctaMac, linux: t.entry.ctaLinux, phone: t.entry.ctaPhone }, lanes: LANES, speeds: START_SPEEDS })}</script>
<script src="/sim.js" defer></script>
</body>
</html>
`;
}
