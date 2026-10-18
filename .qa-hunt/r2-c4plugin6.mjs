process.on("unhandledRejection", e => console.log("[suppressed]", String(e).slice(0,50)));
setTimeout(() => { console.log("[GUARD] exit"); process.exit(9); }, 115_000).unref();
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
// 真实 JS dialog 由 CDP 接受（比打桩 window.confirm 更贴近真实用户：点「确定」）
const onDialog = msg => { if (msg.method === "Page.javascriptDialogOpening") { console.log("[dialog]", JSON.stringify(String(msg.params.message).slice(0,70))); send("Page.handleJavaScriptDialog", { accept: true }, sid).catch(()=>{}); } };
const origPush = events.push.bind(events);
events.push = m => { origPush(m); onDialog(m); return events.length; };
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev); await sleep(600);
await ev("document.querySelector('#btn-plugins').click()");
for (let i = 0; i < 25; i++) { await sleep(400); if (await ev("document.querySelector('#dlg-plugins').open")) break; }
console.log("[pre] rows:", await ev("document.querySelectorAll('#plugins-installed .plugin-row').length"), "| err hidden:", await ev("document.querySelector('#plugin-error').classList.contains('hidden')"));
await shot("CC4-a-before.png");
await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent)); b.click(); return 1;})()`);
console.log("[act] clicked 下发到全部机器 (CDP auto-accepts confirm)");
let got = null;
for (let i = 0; i < 30; i++) {
  await sleep(2000);
  const r = await Promise.race([ev("({hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent, role: document.querySelector('#plugin-error').getAttribute('role'), toast: document.querySelector('#toast').textContent})").catch(e=>({err:String(e).slice(0,40)})), new Promise(res=>setTimeout(()=>res({blocked:true}),9000))]);
  if (r.blocked) { console.log("  ..probe blocked", i); continue; }
  if (r.err) { console.log("  ..probe err", i, r.err); continue; }
  console.log("[probe " + (i*2+2) + "s]", JSON.stringify(r));
  if (r.hidden === false) { got = r; break; }
}
console.log("[FINAL]", JSON.stringify(got));
await shot("CC4-b-partial-visible.png");
process.exit(0);
