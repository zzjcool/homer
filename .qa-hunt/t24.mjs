import { attach, nav, ev, sleep, shot, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
for(let i=0;i<40;i++){await sleep(300); if(await ev(sid,"!document.querySelector('#workspace').classList.contains('hidden')"))break;}
const L=(...a)=>console.log(...a);
await cdp("Emulation.setDeviceMetricsOverride",{width:1280,height:900,deviceScaleFactor:1,mobile:false},sid);

// ===== Real 30s poll test (no manual refresh) =====
await ev(sid,"document.querySelector('#sec-agents').scrollIntoView()");
await ev(sid,`(()=>{const d=document.querySelector('.agent-card details.agent-more');d.open=true;const b=[...d.querySelectorAll('button')][0];b.focus();
  window.__snap={open:d.open, focused:document.activeElement.textContent.trim()};return JSON.stringify(window.__snap)})()`);
L("open menu snapshot:", await ev(sid,"JSON.stringify(window.__snap)"), "| scrollY:", await ev(sid,"window.scrollY"));
await shot(sid,"/root/code/homer/.qa-hunt/shots-B/R1-menu-open-before-poll.png");
// wait for the REAL 30s poll (footer refreshed-at ticking)
const before = await ev(sid,"document.querySelector('#refreshed-at').textContent");
L("footer before:", before);
for(let i=0;i<45;i++){ await sleep(1500); const t=await ev(sid,"document.querySelector('#refreshed-at').textContent"); if(t!==before){L("poll fired at i="+i+":",t);break;} }
L("after REAL poll -> menu open:", await ev(sid,"document.querySelector('.agent-card details.agent-more').open"),
  "| activeElement:", await ev(sid,"document.activeElement.tagName+'|'+(document.activeElement.textContent||'').trim().slice(0,12)"));
await shot(sid,"/root/code/homer/.qa-hunt/shots-B/R2-menu-closed-after-REAL-poll.png");
process.exit(0);
