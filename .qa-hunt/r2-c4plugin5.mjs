process.on("unhandledRejection", e => console.log("[suppressed]", String(e).slice(0,50)));
setTimeout(() => { console.log("[TIMEOUT-GUARD] exit"); process.exit(9); }, 110_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev); await sleep(600);
await ev("document.querySelector('#btn-plugins').click()");
for (let i = 0; i < 25; i++) { await sleep(400); if (await ev("document.querySelector('#dlg-plugins').open")) break; }
// 页面侧自我记录：MutationObserver + 定时器把状态存到 localStorage（跨 CDP 阻塞也能读回）
await ev(`(()=>{ window.__pe=[]; const box=document.querySelector('#plugin-error');
  new MutationObserver(()=>{ window.__pe.push(box.textContent+'|hidden='+box.classList.contains('hidden')); try{localStorage.setItem('r2pe', JSON.stringify(window.__pe));}catch(_){} })
    .observe(box,{childList:true,characterData:true,subtree:true,attributes:true});
  setInterval(()=>{ try{localStorage.setItem('r2t', document.querySelector('#toast').textContent);}catch(_){} }, 1000);
  return 'armed';})()`);
console.log("[pre] armed; rows:", await ev("document.querySelectorAll('#plugins-installed .plugin-row').length"));
await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent)); b.click(); return 1;})()`);
console.log("[act] clicked 下发到全部机器");
// 纯等待，不做任何可能阻塞的调用
await sleep(55000);
console.log("[post] wake up, probing...");
for (let i = 0; i < 8; i++) {
  const r = await Promise.race([ev("({pe: (localStorage.getItem('r2pe')||''), t: localStorage.getItem('r2t'), hidden: document.querySelector('#plugin-error').classList.contains('hidden'), text: document.querySelector('#plugin-error').textContent})").catch(e => "ERR"), new Promise(res => setTimeout(() => res("BLOCKED"), 8000))]);
  if (r !== "BLOCKED") { console.log("[RESULT]", JSON.stringify(r)); break; }
  console.log("  ..blocked probe", i+1); await sleep(4000);
}
await shot("CC4-b-partial.png");
process.exit(0);
