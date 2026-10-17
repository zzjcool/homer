import { attach, nav, ev, sleep, shot, waitFor } from "./lib-b.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
// wait hero empty-center case: hub has no published center? check
console.log("hero:", await ev(sid, "document.querySelector('#hero').textContent.slice(0,300)"));
// find 下发 button in first card
const btns = await ev(sid, "Array.from(document.querySelectorAll('#agent-list button')).map((b,i)=>i+':'+b.textContent.trim())");
console.log("buttons:", JSON.stringify(btns));
process.exit(0);
