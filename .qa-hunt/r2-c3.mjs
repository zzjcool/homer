setTimeout(() => process.exit(9), 55_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(500);

// 我的判据：元素身份（打标记后是否存活）而不是 .open 布尔值 —— open=false 可能被全局 pointerdown 造成，
// 标记消失则只能来自 DOM 被重建。焦点用页面内 setInterval 采样，CDP 全程不发焦点相关调用。
await ev(`(()=>{
  const d = document.querySelector('.agent-card details.agent-more');
  if(!d) return 'NO-MENU';
  d.__r2mark = 'R2MARK';
  d.setAttribute('data-r2mark','R2MARK');
  d.open = true;
  const first = d.querySelector('.agent-more-panel button');
  if (first) first.focus();
  window.__r2 = { samples: [], events: [] };
  document.addEventListener('focusout', () => window.__r2.events.push('focusout:' + (document.activeElement && document.activeElement.tagName)), true);
  window.__r2timer = setInterval(() => {
    const ae = document.activeElement;
    const cur = document.querySelector('.agent-card details.agent-more');
    window.__r2.samples.push({
      t: Date.now(),
      ae: ae ? ae.tagName + '|' + (ae.textContent||'').trim().slice(0,6) : 'null',
      mark: cur ? (cur.__r2mark || 'GONE') : 'NOELEM',
      open: cur ? cur.open : null,
      foot: (document.querySelector('#refreshed-at')||{}).textContent
    });
  }, 500);
  return 'armed';
})()`);
console.log("armed:", await ev("window.__r2 ? 'yes' : 'no'"), "| pre sample:", JSON.stringify(await ev("({ae:document.activeElement.tagName+'|'+(document.activeElement.textContent||'').trim().slice(0,6), mark:(document.querySelector('.agent-card details.agent-more')||{__r2mark:'NOELEM'}).__r2mark, open:(document.querySelector('.agent-card details.agent-more')||{}).open, foot:document.querySelector('#refreshed-at').textContent})")));
await shot("C3-a-menu-open-before-poll.png");

// 等真实 30s 轮询（footer 时间戳变化 = refresh() 真的跑过）
const t0 = Date.now();
let fired = -1;
for (let i = 0; i < 100; i++) {
  await sleep(500);
  if (await ev("window.__r2.samples[window.__r2.samples.length-1].foot !== window.__r2.samples[0].foot")) { fired = i; break; }
}
console.log("poll fired after", ((fired * 500) / 1000).toFixed(1), "s (budget", ((Date.now()-t0)/1000).toFixed(1), "s)");
const res = await ev(`(()=>{
  const s = window.__r2.samples;
  const before = s[0], after = s[s.length-1];
  return { before, after, changedIdx: s.findIndex(x => x.foot !== s[0].foot), events: window.__r2.events.slice(0,6), n: s.length };
})()`);
console.log("BEFORE:", JSON.stringify(res.before));
console.log("AFTER :", JSON.stringify(res.after));
console.log("focus events:", JSON.stringify(res.events));
await ev("clearInterval(window.__r2timer)");
await shot("C3-b-after-real-poll.png");
process.exit(0);
