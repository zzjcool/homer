import { attach, nav, ev, sleep, shot, waitFor, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
await waitFor(sid, "!document.querySelector('#workspace').classList.contains('hidden')", 10000);
const L=(...a)=>console.log(...a);
// hero buttons
L("hero btns:", await ev(sid,"JSON.stringify([...document.querySelectorAll('#hero button')].map(b=>b.textContent.trim()))"));
await ev(sid,"[...document.querySelectorAll('#hero button')].find(b=>b.textContent.includes('查看存储内容'))?.click()");
await waitFor(sid,"document.querySelector('#dlg-view').open",5000);
await waitFor(sid,"document.querySelector('#view-body').textContent.length>80",15000);
L("storage title:", await ev(sid,"document.querySelector('#view-title').textContent"));
L("storage body:", await ev(sid,"document.querySelector('#view-body').textContent.replace(/\\s+/g,' ').slice(0,500)"));
L("storage undefined/NaN:", await ev(sid,"/undefined|NaN/.test(document.querySelector('#view-body').textContent)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/24-storage.png");
await ev(sid,"document.querySelector('#dlg-view').close()");
await sleep(400);
// 去同步 → from hero
await ev(sid,"(()=>{const b=[...document.querySelectorAll('#hero button')].find(b=>b.textContent.includes('去同步'));if(!b)return 'none';b.click();return 'clicked'})()");
await sleep(1500);
L("after 去同步 scrollY:", await ev(sid,"window.scrollY"), "activeEl:", await ev(sid,"document.activeElement.tagName+'|'+(document.activeElement.textContent||'').slice(0,20)"));
await shot(sid,"/root/code/homer-cli/.qa-hunt/shots-B/25-gosync.png");
L("ERRS:", JSON.stringify(LOG.err.slice(0,5)));
process.exit(0);
