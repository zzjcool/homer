import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
L("isSecureContext:", await ev(sid,"window.isSecureContext"), "origin:", await ev(sid,"location.origin"), "clipboard:", await ev(sid,"typeof navigator.clipboard"));

// ---- D: copyJoin (接入命令 → 复制) — no null check on navigator.clipboard
await ev(sid,"document.querySelector('#btn-join-top').click()");
await waitFor(sid,"document.querySelector('#dlg-join').open && document.querySelector('#join-cmd').value.indexOf('hr_')>=0",10000);
L("D join-cmd:", await ev(sid,"document.querySelector('#join-cmd').value"));
await ev(sid,"window.__dlg.length=0");
await ev(sid,"document.querySelector('#btn-copy').click()");
await sleep(1200);
L("D btn-copy text after click:", await ev(sid,"document.querySelector('#btn-copy').textContent"));
L("D copy-fallback hidden?", await ev(sid,"document.querySelector('#copy-fallback').classList.contains('hidden')"));
L("D dialogs:", await ev(sid,"JSON.stringify(window.__dlg)"));
L("D window.__err:", await ev(sid,"JSON.stringify(window.__err)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/06-copyjoin.png");
await ev(sid,"document.querySelector('#dlg-join').close()");
await sleep(500);

// ---- E: copyCommand (fallback manual install command in agents error) — check .then(ok,ask)
L("E copyCommand src check via behaviour: n/a");

// ---- F: 停用 adapter from 查看 dialog
await ev(sid, `(()=>{const c=document.querySelector('.agent-card');[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='查看').click()})()`);
await waitFor(sid,"document.querySelector('#view-body').textContent.length>60",15000);
L("F agentId:", await ev(sid,"document.querySelector('#view-body').textContent.slice(0,40)"));
L("F override buttons:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#view-body button[data-adapter-override]')].map(b=>[b.dataset.adapterOverride,b.textContent.trim()]))"));
await ev(sid, `(()=>{const b=document.querySelector('#view-body button[data-adapter-override]');if(!b)return 'none';b.click();return b.dataset.adapterOverride})()`);
await sleep(1200);
L("F dlg-confirm open:", await ev(sid,"document.querySelector('#dlg-confirm').open"));
L("F title:", await ev(sid,"document.querySelector('#confirm-title').textContent"));
L("F desc:", await ev(sid,"document.querySelector('#confirm-desc').textContent"));
L("F ok:", await ev(sid,"JSON.stringify({t:document.querySelector('#btn-confirm-ok').textContent,d:document.querySelector('#btn-confirm-ok').disabled})"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/07-disable-confirm.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,6)));
L("window.__err:", await ev(sid,"JSON.stringify(window.__err)"));
process.exit(0);
