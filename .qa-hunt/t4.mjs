import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);

// ---- A: 30s refresh wipes open <details> menu / scroll / focus
await ev(sid, "window.scrollTo(0, 300)");
await ev(sid, `(()=>{const d=document.querySelector('.agent-card details.agent-more'); d.open=true; return !!d})()`);
await sleep(300);
L("A before refresh: details open =", await ev(sid,"document.querySelector('.agent-card details.agent-more').open"), "scrollY =", await ev(sid,"window.scrollY"));
await ev(sid, "refresh()");
await sleep(3000);
L("A after refresh(): details open =", await ev(sid,"document.querySelector('.agent-card details.agent-more') ? document.querySelector('.agent-card details.agent-more').open : 'NO-MENU'"), "scrollY =", await ev(sid,"window.scrollY"));
L("A cards after:", await ev(sid,"document.querySelectorAll('.agent-card').length"));

// ---- B: focus a button then refresh
await ev(sid, `(()=>{const b=[...document.querySelectorAll('.agent-card button')].find(b=>b.textContent.trim()==='详情'); b.focus(); return document.activeElement.textContent})()`);
await ev(sid, "refresh()");
await sleep(3000);
L("B activeElement after refresh:", await ev(sid,"document.activeElement ? (document.activeElement.tagName+'.'+document.activeElement.className+'|'+document.activeElement.textContent.slice(0,20)) : 'none'"));

// ---- C: 查看 dialog (openMachineView) content
await ev(sid, `(()=>{const c=document.querySelector('.agent-card');[...c.querySelectorAll('button')].find(b=>b.textContent.trim()==='查看').click()})()`);
await waitFor(sid,"document.querySelector('#dlg-view').open",5000);
await waitFor(sid,"document.querySelector('#view-body').textContent.length>60",15000);
L("C view-title:", await ev(sid,"document.querySelector('#view-title').textContent"));
L("C view-body:", await ev(sid,"document.querySelector('#view-body').textContent.slice(0,600)"));
L("C has undefined/NaN:", await ev(sid,"/undefined|NaN/.test(document.querySelector('#view-body').textContent)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/05-view.png");
await ev(sid,"document.querySelector('#dlg-view').close()");
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)), JSON.stringify(LOG.console.slice(0,5)));
process.exit(0);
