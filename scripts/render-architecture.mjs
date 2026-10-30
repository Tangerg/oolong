import { access, readFile, writeFile } from "node:fs/promises";

// Diagram facts live in JSON. This script owns only their presentation and verifies
// the generated page so edits cannot leave a second, stale architecture behind.
const root = new URL("../", import.meta.url);
const source = JSON.parse(await readFile(new URL("docs/diagrams/oolong.architecture.json", root), "utf8"));
const destination = new URL("docs/public/diagrams/oolong-architecture.html", root);
const escape = (value) => String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
const link = (path) => `https://github.com/Tangerg/oolong/tree/main/${escape(path)}`;
for (const item of [...source.layers, ...source.consumers]) {
  await access(new URL(item.path, root));
}
const layers = source.layers.map((item, index) => `<article class="layer layer-${index}">
  <div class="eyebrow">${escape(item.kind)}</div><h2>${escape(item.name)} <a href="${link(item.path)}">${escape(item.path)}</a></h2>
  <p class="summary">${escape(item.summary)}</p><p>${escape(item.contract)}</p>
</article>${index < source.layers.length - 1 ? '<div class="arrow" aria-label="composes the lower layer"><svg viewBox="0 0 20 28" aria-hidden="true"><path d="M10 0v23m-6-6 6 6 6-6"/></svg><span>composes</span></div>' : ''}`).join("\n");
const consumers = source.consumers.map((item) => `<article class="consumer"><h3>${escape(item.name)}</h3><a href="${link(item.path)}">${escape(item.path)}</a><p class="summary">${escape(item.summary)}</p><p>${escape(item.contract)}</p></article>`).join("\n");
const owners = source.owners.map((item) => `<tr><td>${escape(item.fact)}</td><td><code>${escape(item.owner)}</code></td><td>${escape(item.projection)}</td></tr>`).join("\n");
const progress = source.progress.map((item) => `<li><h3>${escape(item.name)}</h3><p>${escape(item.detail)}</p></li>`).join("\n");
const html = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="generator" content="scripts/render-architecture.mjs"><title>${escape(source.title)}</title>
<style>
:root{color-scheme:light dark;--bg:#f7f8fc;--fg:#152139;--muted:#506075;--line:#cad3e1;--panel:#fff;--accent:#745313;--core:#165e73;--base:#4256a6;--kit:#7e427d}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.6 system-ui,sans-serif}main{max-width:1160px;margin:auto;padding:54px 32px 64px}header{max-width:880px;margin-bottom:34px}.eyebrow{color:var(--accent);font-size:12px;font-weight:750;letter-spacing:.14em;text-transform:uppercase}h1{font-size:clamp(30px,4vw,46px);line-height:1.15;margin:12px 0 18px;letter-spacing:-.035em}h2{font-size:23px;margin:0 0 8px;line-height:1.3}h3{font-size:17px;margin:0 0 7px}p{margin:8px 0;color:var(--muted)}a{color:var(--core);text-underline-offset:3px}.architecture{display:grid;grid-template-columns:minmax(0,1.3fr) minmax(260px,1fr);gap:28px;align-items:start}.layer,.consumer{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:21px 24px}.layer{border-left:5px solid var(--core)}.layer-0{border-left-color:var(--kit)}.layer-1{border-left-color:var(--base)}.layer h2{display:flex;justify-content:space-between;align-items:baseline;gap:18px}.layer a,.consumer>a{font:13px ui-monospace,monospace}.summary{color:var(--fg);font-weight:650}.arrow{height:42px;display:flex;align-items:center;justify-content:center;gap:12px;color:var(--muted);font-size:12px}.arrow svg{width:15px;height:28px;fill:none;stroke:var(--muted);stroke-width:1.5}.consumers{display:grid;gap:16px}.consumer{padding:18px 21px}.consumer p{font-size:14px}.section{margin-top:44px}.section>p{max-width:850px}.table-wrap{overflow-x:auto;border:1px solid var(--line);border-radius:12px;margin-top:18px}table{width:100%;border-collapse:collapse;background:var(--panel);font-size:14px}th,td{padding:14px 16px;text-align:left;vertical-align:top}th{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}tr+tr,tbody tr:first-child{border-top:1px solid var(--line)}code{font:13px ui-monospace,monospace;white-space:nowrap}.progress{display:grid;grid-template-columns:repeat(auto-fit,minmax(175px,1fr));gap:18px;list-style-position:inside;padding:0;margin-top:22px}.progress li{padding:18px;border:1px solid var(--line);border-radius:12px;background:var(--panel);color:var(--accent)}.progress h3{font-size:15px;margin-top:7px;color:var(--fg)}.progress p{font-size:13px}footer{margin-top:36px;padding-top:22px;border-top:1px solid var(--line);font-size:13px;color:var(--muted)}@media(prefers-color-scheme:dark){:root{--bg:#0c1423;--fg:#edf2fa;--muted:#abb8ce;--line:#344055;--panel:#151f30;--accent:#dfb96b;--core:#84cdd7;--base:#a5b6ff;--kit:#dca2d6}}@media(max-width:760px){main{padding:30px 20px}.architecture{grid-template-columns:1fr}.layer h2{display:block}.layer a{display:block;margin-top:5px}.consumers{grid-template-columns:1fr}}
</style></head>
<body><main><header><div class="eyebrow">Oolong architecture</div><h1>${escape(source.title)}</h1><p>${escape(source.description)}</p></header>
<section class="architecture" aria-label="Package composition"><div>${layers}</div><aside class="consumers">${consumers}</aside></section>
<section class="section"><h2>Every advancing fact has one owner</h2><p>Projections can encode, cache and render a fact. They cannot originate a competing transition. Directory names follow the same boundaries as the contracts.</p><div class="table-wrap"><table><thead><tr><th>Fact</th><th>Owner</th><th>Projection</th></tr></thead><tbody>${owners}</tbody></table></div></section>
<section class="section"><h2>Completion has distinct meanings</h2><p>A successful earlier stage does not claim that later work happened. Source identity, publication responsibility and physical terminal progress keep separate observable outcomes.</p><ol class="progress">${progress}</ol></section>
<footer>Generated from <a href="https://github.com/Tangerg/oolong/blob/main/docs/diagrams/oolong.architecture.json">the architecture data</a>. Read the <a href="../core/architecture">core architecture guide</a> for preconditions, refusal semantics and limits.</footer>
</main></body></html>
`;
if (process.argv.includes("--check")) {
  const current = await readFile(destination, "utf8");
  if (current !== html) {
    throw new Error("Architecture page is stale; run node scripts/render-architecture.mjs");
  }
  console.log("Architecture diagram matches its source");
} else {
  await writeFile(destination, html);
  console.log("Updated architecture diagram");
}
