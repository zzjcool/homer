import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
await ev(sid, "document.querySelector('#btn-plugins').click()");
await waitFor(sid,"document.querySelector('#dlg-plugins').open",6000);
await sleep(1500);
L("P1 plugin dialog text:", await ev(sid,"document.querySelector('#dlg-plugins').textContent.replace(/\\s+/g,' ').slice(0,500)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/20-plugins.png");
await ev(sid,"document.querySelector('#dlg-plugins').close()");
await sleep(400);
// tool upgrade button
L("T tool chips:", await ev(sid,"JSON.stringify([...document.querySelectorAll('.agent-card .tool-row button, .agent-card [data-tool]')].map(b=>b.textContent.trim()))"));
L("T 全部升级应用 present:", await ev(sid,"[...document.querySelectorAll('#agent-list button')].map(b=>b.textContent.trim()).filter(t=>t.includes('升级')||t.includes('更新'))"));
process.exit(0);
