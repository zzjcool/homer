import { attach, nav, ev, sleep, shot, waitFor, LOG, cdp } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);

// --- C1 detector: navigator.clipboard presence in the QA browser context
L("C1 detector navigator.clipboard:", await ev(sid,"String(navigator.clipboard)"), "isSecureContext:", await ev(sid,"window.isSecureContext"), "origin:", await ev(sid,"location.origin"));
await ev(sid,"window.__err=[];document.querySelector('#btn-join-top').click()");
await waitFor(sid,"document.querySelector('#join-cmd').value.indexOf('hr_')>=0",10000);
await ev(sid,"document.querySelector('#btn-copy').click()");
await sleep(1200);
L("C1 __err:", await ev(sid,"JSON.stringify(window.__err)"), "btnText:", await ev(sid,"document.querySelector('#btn-copy').textContent"),
  "fallbackHidden:", await ev(sid,"document.querySelector('#copy-fallback').classList.contains('hidden')"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C1-copy-join-broken.png");
await ev(sid,"document.querySelector('#dlg-join').close()"); await sleep(500);

// --- C4: logout failure leaves server session alive, UI at login gate
await ev(sid,"window.__err=[]");
await ev(sid, `(()=>{const of=window.fetch.bind(window);window.__logoutCalls=0;
  window.fetch=(u,o)=>{if(String(u).includes('/api/auth/logout')){window.__logoutCalls++;return Promise.reject(new TypeError('Failed to fetch'));}return of(u,o)};return 'patched'})()`);
await ev(sid,"document.querySelector('#btn-logout').click()");
await sleep(1800);
L("C4 logoutCalls:", await ev(sid,"window.__logoutCalls"),
  "workspaceHidden:", await ev(sid,"document.querySelector('#workspace').classList.contains('hidden')"),
  "gateLoginShown:", await ev(sid,"!document.querySelector('#gate-login').classList.contains('hidden')"),
  "errText:", await ev(sid,"JSON.stringify(document.querySelector('#err-login')?document.querySelector('#err-login').textContent:null)"));
L("C4 server session /api/console status:", await ev(sid,"fetch('/api/console',{headers:{Accept:'application/json'}}).then(r=>r.status)"));
L("C4 window.__err:", await ev(sid,"JSON.stringify(window.__err)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/C4-logout-fake.png");
process.exit(0);
