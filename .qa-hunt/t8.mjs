import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);

// C1 retry: copyJoin null-clipboard crash, with screenshot at moment of failure
await ev(sid,"document.querySelector('#btn-join-top').click()");
await waitFor(sid,"document.querySelector('#join-cmd').value.indexOf('hr_')>=0",10000);
L("C1 clipboard:", await ev(sid,"typeof navigator.clipboard"), "isSecureContext:", await ev(sid,"window.isSecureContext"));
await ev(sid,"window.__err=[]");
await ev(sid,"document.querySelector('#btn-copy').click()");
await sleep(1500);
L("C1 err:", await ev(sid,"JSON.stringify(window.__err)"));
L("C1 btn text:", await ev(sid,"document.querySelector('#btn-copy').textContent"));
L("C1 fallback hidden:", await ev(sid,"document.querySelector('#copy-fallback').classList.contains('hidden')"));
L("C1 fallback text:", await ev(sid,"document.querySelector('#copy-fallback').textContent.slice(0,80)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/13-copyjoin-broken.png");
await ev(sid,"document.querySelector('#dlg-join').close()");
await sleep(600);

// C7: logout request failure still returns to login gate (silent)
await ev(sid,"window.__err=[]");
await ev(sid, `(()=>{const of=window.fetch;window.fetch=(u,o)=>{if(String(u).includes('/api/auth/logout'))return Promise.reject(new TypeError('Failed to fetch'));return of(u,o)};return 'patched'})()`);
await ev(sid,"document.querySelector('#btn-logout').click()");
await sleep(2000);
L("C7 workspace hidden after failed logout:", await ev(sid,"document.querySelector('#workspace').classList.contains('hidden')"));
L("C7 gate-login visible:", await ev(sid,"!document.querySelector('#gate-login').classList.contains('hidden')"));
L("C7 toast:", await ev(sid,"document.querySelector('#toast').textContent"));
L("C7 err note:", await ev(sid,"document.querySelector('#gate-login').textContent.slice(0,200)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/14-logout-failed.png");

// relogin and check session still valid server-side (proves server-side session not revoked)
L("C7 server session still valid:", await ev(sid, `fetch('/api/console',{headers:{'Accept':'application/json'}}).then(r=>r.status).catch(e=>'err')`));
process.exit(0);
