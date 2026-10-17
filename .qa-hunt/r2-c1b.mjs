setTimeout(() => process.exit(9), 28_000).unref();
import { connect, sleep, login } from "/root/code/homer-cli/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot } = await connect();
await send("Page.enable", {}, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 40; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev);
await ev(`(()=>{window.__prompt=[];window.prompt=(a,b)=>{window.__prompt.push(String(a).slice(0,20));return null};return true})()`);
for (const round of [2, 3]) {
  await ev("document.querySelector('#btn-join-top').click()");
  for (let i = 0; i < 40; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
  await ev("window.__prompt=[]");
  const r = await ev(`(()=>{ try { copyJoin('join-cmd','btn-copy'); return 'no-throw'; } catch(e){ return 'THROW:'+e.name+': '+e.message; } })()`);
  await sleep(900);
  const s = await ev(`({ btn: document.querySelector('#btn-copy').textContent, fallbackHidden: document.querySelector('#copy-fallback').classList.contains('hidden'), prompts: window.__prompt.slice(), selected: String(document.getSelection()).length })`);
  console.log("round" + round + ":", r, JSON.stringify(s));
  await ev("document.querySelector('#dlg-join').close()");
  await sleep(400);
}
await shot("C1-round3-copyjoin.png");
process.exit(0);
