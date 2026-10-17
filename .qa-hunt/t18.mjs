import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
// delay /api/agents by 3s using fetch patch (slow network simulation)
await ev(sid, `(()=>{const of=window.fetch.bind(window);window.fetch=(u,o)=>{const url=String(u);if(url.includes('/api/agents'))return new Promise(r=>setTimeout(()=>of(u,o).then(r),3000));return of(u,o)};return 'patched'})()`);
L("refreshed-at before:", await ev(sid,"document.querySelector('#refreshed-at').textContent"));
await ev(sid,"document.querySelector('#btn-refresh').click()");
await sleep(300);
L("t+0.3s busyVisible:", await ev(sid,"!document.querySelector('#busy').classList.contains('hidden')"),
  "btnDisabled:", await ev(sid,"document.querySelector('#btn-refresh').disabled"),
  "btnText:", await ev(sid,"document.querySelector('#btn-refresh').textContent"),
  "ariaBusy:", await ev(sid,"document.querySelector('#btn-refresh').getAttribute('aria-busy')"),
  "refreshed-at:", await ev(sid,"document.querySelector('#refreshed-at').textContent"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C5-refresh-no-feedback.png");
// double-click concurrency: click 3x fast, count in-flight
await ev(sid, `(()=>{window.__inflight=0;const of=window.fetch.bind(window);window.fetch=(u,o)=>{const url=String(u);if(url.includes('/api/agents')){window.__inflight++;return of(u,o).finally(()=>{window.__inflight--})}return of(u,o)};return 'p'})()`);
for(let i=0;i<3;i++){ await ev(sid,"document.querySelector('#btn-refresh').click()"); await sleep(60); }
await sleep(500);
L("C5 concurrent /api/agents in flight:", await ev(sid,"window.__inflight"));
await sleep(6000);
L("after settle refreshed-at:", await ev(sid,"document.querySelector('#refreshed-at').textContent"));
process.exit(0);
