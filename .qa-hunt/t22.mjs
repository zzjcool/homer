import { attach, nav, ev, sleep, LOG } from "./lib2.mjs";
const sid = await attach();
await nav(sid, "http://hub:7760/");
await ev(sid, "document.querySelector('#in-login-pw').value='qa-admin-password'");
await ev(sid, "document.querySelector('#btn-login').click()");
for(let i=0;i<40;i++){await sleep(300); if(await ev(sid,"!document.querySelector('#workspace').classList.contains('hidden')"))break;}
const L=(...a)=>console.log(...a);
await ev(sid,"window.__dlg=[];window.__err=[]");
// copyCommand in non-secure context -> should fall back to prompt (no toast, prompt shown)
L("copyCommand:", await ev(sid,`(()=>{copyCommand('curl x'); return JSON.stringify(window.__dlg)})()`));
// copyAgentId -> fallback prompt + toast
L("copyAgentId:", await ev(sid,`(()=>{copyAgentId('abc'); return JSON.stringify(window.__dlg)})()`));
L("toast after copyAgentId:", await ev(sid,"document.querySelector('#toast').textContent"));
// copyJoin -> crash
L("copyJoin:", await ev(sid,`(()=>{try{copyJoin('join-cmd','btn-copy')}catch(e){return 'sync-throw:'+e.message}return 'no-sync-throw'})()`));
await sleep(800);
L("__err:", await ev(sid,"JSON.stringify(window.__err)"));
process.exit(0);
