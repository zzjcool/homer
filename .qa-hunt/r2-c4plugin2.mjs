setTimeout(() => process.exit(9), 45_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev); await sleep(600);
await ev("document.querySelector('#btn-plugins').click()");
for (let i = 0; i < 25; i++) { await sleep(400); if (await ev("document.querySelector('#dlg-plugins').open && document.querySelector('#plugins-installed .plugin-row')")) break; }
console.log("[pre] rows:", await ev("document.querySelectorAll('#plugins-installed .plugin-row').length"), "| err hidden:", await ev("document.querySelector('#plugin-error').classList.contains('hidden')"));
await ev("window.__r2shot=0");
// 直接发请求 + 把 UI 的 result 分支喂进去？不行（不许改源码）。
// 改为：点按钮（异步），然后**轮询 plugin-error，且每轮用短超时 ev**，避免 awaitPromise 阻塞。
await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');
  const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent)); b.click(); return 1;})()`);
const snap = `({ hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent, toast: document.querySelector('#toast').textContent, btn: (()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');const b=[...r.querySelectorAll('button')].find(x=>/下发/.test(x.textContent));return b?b.textContent.trim()+'|disabled='+b.disabled:'?';})() })`;
let seen = null;
for (let i = 0; i < 34; i++) {
  await sleep(1000);
  let s;
  try { s = await Promise.race([ev(snap), new Promise((_, rj) => setTimeout(() => rj(new Error("slow")), 3000))]); }
  catch (e) { console.log("  ..probe", i, "busy/blocked"); continue; }
  if (!s.hidden) { seen = s; console.log("[RESULT] VISIBLE at", i + 1, "s:", JSON.stringify(s.text)); break; }
  if (i % 5 === 0) console.log("  ..waiting", i + 1, "s | btn:", s.btn, "| toast:", JSON.stringify(s.toast));
}
if (!seen) { try { seen = await ev(snap); } catch (e) { seen = { err: e.message }; } }
console.log("[FINAL]", JSON.stringify(seen));
await shot("CC4-b-partial.png");
process.exit(0);
