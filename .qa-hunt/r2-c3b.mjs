setTimeout(() => process.exit(9), 55_000).unref();
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev); await sleep(500);
const arm = `(()=>{
  const d = document.querySelector('.agent-card details.agent-more');
  if(!d) return 'NO-MENU';
  d.__r2mark='R2MARK'; d.open=true;
  const first = d.querySelector('.agent-more-panel button'); if(first) first.focus();
  window.__r2={samples:[]};
  window.__r2timer=setInterval(()=>{const cur=document.querySelector('.agent-card details.agent-more');const ae=document.activeElement;
    window.__r2.samples.push({ae:ae?ae.tagName:'null',mark:cur?(cur.__r2mark||'GONE'):'NOELEM',open:cur?cur.open:null,foot:document.querySelector('#refreshed-at').textContent});},500);
  return 'armed';})()`;
// ---- control：8s 内不发生 refresh（footer 不变），标记必须存活 ----
await ev(arm);
await sleep(8000);
const c = await ev(`(()=>{const s=window.__r2.samples;const f0=s[0].foot;const changed=s.some(x=>x.foot!==f0);
  return {n:s.length, footerChanged:changed, lastMark:s[s.length-1].mark, lastOpen:s[s.length-1].open, lastAe:s[s.length-1].ae}})()`);
console.log("[control-2] 8s no-poll:", JSON.stringify(c));
await ev("clearInterval(window.__r2timer)");
// ---- 实验-2：等真实轮询 ----
await ev(arm);
let fired = -1;
for (let i = 0; i < 100; i++) { await sleep(500); if (await ev("window.__r2.samples[window.__r2.samples.length-1].foot !== window.__r2.samples[0].foot")) { fired = i; break; } }
const e = await ev(`(()=>{const s=window.__r2.samples;return {before:s[0],after:s[s.length-1]}})()`);
console.log("[exp-2] poll after", (fired*0.5).toFixed(1), "s | BEFORE:", JSON.stringify(e.before), "| AFTER:", JSON.stringify(e.after));
await ev("clearInterval(window.__r2timer)");
await shot("C3-round2-after-poll.png");
process.exit(0);
