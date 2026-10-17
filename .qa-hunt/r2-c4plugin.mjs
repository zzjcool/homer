setTimeout(() => process.exit(9), 45_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Network.enable", {}, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
await sleep(600);
// 打开插件管理
await ev("document.querySelector('#btn-plugins').click()");
for (let i = 0; i < 30; i++) { await sleep(400); if (await ev("document.querySelector('#dlg-plugins').open && document.querySelector('#plugins-installed .plugin-row')")) break; }
console.log("[pre] dlg open:", await ev("document.querySelector('#dlg-plugins').open"), "| rows:", await ev("document.querySelectorAll('#plugins-installed .plugin-row').length"));
console.log("[pre] plugin-error hidden:", await ev("document.querySelector('#plugin-error').classList.contains('hidden')"), "| text:", JSON.stringify(await ev("document.querySelector('#plugin-error').textContent")));
await shot("CC4-a-plugins-before.png");
// 找 pi 行的「下发到全部机器」按钮
const info = await ev(`(()=>{const rows=[...document.querySelectorAll('#plugins-installed .plugin-row')];
  const r=rows.find(x=>x.getAttribute('data-plugin-id')==='pi');
  if(!r) return {err:'NOPI', ids: rows.map(x=>x.getAttribute('data-plugin-id'))};
  const btns=[...r.querySelectorAll('button')].map(b=>b.textContent.trim());
  return {id:r.getAttribute('data-plugin-id'), btns};})()`);
console.log("[pre] pi row:", JSON.stringify(info));
// 点它
const clicked = await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');
  const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent));
  if(!b) return 'NOBTN'; b.click(); return 'clicked';})()`);
console.log("[step] click:", clicked);
for (let i = 0; i < 60; i++) {
  await sleep(1000);
  const s = await ev(`({ hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent, toast: document.querySelector('#toast').textContent })`);
  if (!s.hidden) { console.log("[RESULT] plugin-error VISIBLE after", i + 1, "s:", JSON.stringify(s.text)); break; }
  if (i % 10 === 0) console.log("  ..waiting", i, "s");
}
const fin = await ev(`({ hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent, role: document.querySelector('#plugin-error').getAttribute('role'), toast: document.querySelector('#toast').textContent })`);
console.log("[FINAL]", JSON.stringify(fin));
await shot("CC4-b-partial-raw.png");
process.exit(0);
