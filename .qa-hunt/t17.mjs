import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
// C5: slow down /api/agents so refresh takes 3s, then observe button feedback
await cdp("Fetch.enable",{patterns:[{urlPattern:"*api/agents*",requestStage:"Response"}]},sid);
L("Fetch enabled (note: we won't continue -> refresh will stall? verify)");
process.exit(0);
