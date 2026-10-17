process.on("unhandledRejection", e => console.log("[suppressed]", String(e).slice(0,60)));
setTimeout(() => process.exit(9), 60_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev); await sleep(600);
await ev("document.querySelector('#btn-plugins').click()");
for (let i = 0; i < 25; i++) { await sleep(400); if (await ev("document.querySelector('#dlg-plugins').open")) break; }
events.length = 0;
// 安装页面侧 MutationObserver（不依赖后续 CDP 调用）
await ev(`(()=>{window.__pe=[];const box=document.querySelector('#plugin-error');
  new MutationObserver(()=>window.__pe.push(box.textContent+'|hidden='+box.classList.contains('hidden'))).observe(box,{childList:true,characterData:true,subtree:true,attributes:true});
  return 'observing';})()`);
console.log("observer:", await ev("'ok'"));
await ev(`(()=>{const r=[...document.querySelectorAll('#plugins-installed .plugin-row')].find(x=>x.getAttribute('data-plugin-id')==='pi');
  const b=[...r.querySelectorAll('button')].find(x=>/下发到全部机器/.test(x.textContent)); b.click(); return 1;})()`);
console.log("[act] clicked");
await sleep(8000);
console.log("[t+8s] trivial eval:", await Promise.race([ev("1+1").catch(e=>"ERR"), new Promise(r=>setTimeout(()=>r("BLOCKED"),5000))]));
const dlg = events.filter(e => /javascriptDialog|DialogOpening/i.test(e.method));
console.log("[t+8s] dialog events:", JSON.stringify(dlg.map(e=>e.method)));
await sleep(30000);
console.log("[t+38s] trivial eval:", await Promise.race([ev("1+1").catch(e=>"ERR"), new Promise(r=>setTimeout(()=>r("BLOCKED"),5000))]));
console.log("[t+38s] __pe:", JSON.stringify(await Promise.race([ev("window.__pe").catch(e=>"ERR"), new Promise(r=>setTimeout(()=>r("BLOCKED"),5000))])));
process.exit(0);
