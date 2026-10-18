setTimeout(() => process.exit(9), 120_000).unref();
process.on("unhandledRejection", () => {});
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev); await sleep(600);
await ev("document.querySelector('#btn-plugins').click()");
for (let i = 0; i < 25; i++) { await sleep(400); if (await ev("document.querySelector('#dlg-plugins').open && document.querySelector('#plugins-installed .plugin-row')")) break; }
console.log("[pre] rows:", await ev("document.querySelectorAll('#plugins-installed .plugin-row').length"), "| err hidden:", await ev("document.querySelector('#plugin-error').classList.contains('hidden')"));
console.log("[act] click 下发到全部机器 (real fanout, may take ~40s)");
await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');
  const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent)); b.click(); return 1;})()`);
await sleep(45000);
const snap = `({ hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent, role: document.querySelector('#plugin-error').getAttribute('role'), toast: document.querySelector('#toast').textContent, btn: (()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');const b=[...r.querySelectorAll('button')].find(x=>/下发/.test(x.textContent));return b?b.textContent.trim():'?';})() })`;
let done = null;
for (let i = 0; i < 12; i++) {
  let s = null;
  try { s = await Promise.race([ev(snap).catch(e => ({ err: e.message })), new Promise(r => setTimeout(() => r({ err: "blocked" }), 6000))]); }
  catch (e) { s = { err: e.message }; }
  console.log("[probe", i + 1, "]", JSON.stringify(s));
  if (s && s.hidden === false) { done = s; break; }
  await sleep(3000);
}
console.log("[FINAL]", JSON.stringify(done));
await shot("CC4-b-partial-visible.png");
process.exit(0);
