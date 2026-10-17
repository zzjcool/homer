setTimeout(() => process.exit(9), 100_000).unref();
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
console.log("[act] clicking 下发到全部机器 ...");
await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');
  const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent)); b.click(); return 1;})()`);
// 主线程可能在 fanout 期间被占住；先纯等 50s 不打扰它
await sleep(50000);
const snap = `({ hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent, role: document.querySelector('#plugin-error').getAttribute('role'), toast: document.querySelector('#toast').textContent, btnText: (()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');const b=[...r.querySelectorAll('button')].find(x=>/下发/.test(x.textContent));return b?b.textContent.trim():'?';})() })`;
for (let i = 0; i < 20; i++) {
  let s;
  try { s = await Promise.race([ev(snap), new Promise((_, rj) => setTimeout(() => rj(new Error("blocked")), 6000))]); }
  catch (e) { console.log("  ..main thread busy at probe", i); await sleep(4000); continue; }
  console.log("[probe", i + 1, "]", JSON.stringify(s));
  if (!s.hidden) break;
  await sleep(3000);
}
await shot("CC4-b-partial-visible.png");
process.exit(0);
