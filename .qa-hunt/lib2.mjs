setTimeout(() => { console.error("TIMEOUT-GUARD-EXIT"); process.exit(9); }, 50_000).unref();
const V = await fetch("http://127.0.0.1:3000/json/version").then(r=>r.json());
const ws = new WebSocket(V.webSocketDebuggerUrl.replace("ws://0.0.0.0","ws://127.0.0.1"));
const pend = new Map(); let seq = 0;
export const LOG = { err: [], console: [] };
await new Promise((res,rej)=>{ ws.onopen=res; ws.onerror=rej; });
ws.onmessage = m => { const d = JSON.parse(m.data);
  if (d.id && pend.has(d.id)) { pend.get(d.id)(d); pend.delete(d.id); return; }
  if (d.method === "Runtime.exceptionThrown") LOG.err.push(String(d.params?.exceptionDetails?.exception?.description || d.params?.exceptionDetails?.text).split("\n")[0]);
  if (d.method === "Runtime.consoleAPICalled" && ["error","warning"].includes(d.params?.type)) LOG.console.push(d.params.type+": "+d.params.args.map(a=>a.value ?? a.description).join(" ").slice(0,200));
};
function raw(method, params={}, sid) {
  const id = ++seq;
  const p = new Promise(r => pend.set(id, r));
  const msg = { id, method, params }; if (sid) msg.sessionId = sid;
  ws.send(JSON.stringify(msg));
  return Promise.race([p, new Promise((_,rj)=>setTimeout(()=>rj(new Error("CDP timeout "+method)),12000))]);
}
export async function attach() {
  const t = await raw("Target.getTargets");
  let page = t.result.targetInfos.find(x=>x.type==="page");
  if (!page) { const c = await raw("Target.createTarget",{url:"about:blank"}); page = {targetId:c.result.targetId}; await sleep(1200); }
  const sid = (await raw("Target.attachToTarget",{targetId:page.targetId,flatten:true})).result.sessionId;
  await raw("Page.enable",{},sid); await raw("Runtime.enable",{},sid); await raw("Log.enable",{},sid);
  await raw("Page.addScriptToEvaluateOnNewDocument",{source:`
    window.__dlg=[];
    window.prompt=function(m,d){window.__dlg.push(["prompt",String(m).slice(0,80)]);return null;};
    window.confirm=function(m){window.__dlg.push(["confirm",String(m).slice(0,80)]);return true;};
    window.alert=function(m){window.__dlg.push(["alert",String(m).slice(0,80)]);};
    window.__err=[]; window.addEventListener("error",e=>window.__err.push(String(e.message)));
    window.addEventListener("unhandledrejection",e=>window.__err.push("rej:"+(e.reason&&e.reason.message)));
  `},sid);
  return sid;
}
export async function nav(sid, url) { await raw("Page.navigate",{url},sid);
  for(let i=0;i<60;i++){await sleep(300); if(await ev(sid,"document.readyState")==="complete")break;} await sleep(500); }
export async function ev(sid, expr) {
  const r = await raw("Runtime.evaluate",{expression:expr, returnByValue:true, awaitPromise:true}, sid);
  if (r.result?.exceptionDetails) return {__err:String(r.result.exceptionDetails.exception?.description||r.result.exceptionDetails.text).slice(0,300)};
  return r.result?.result?.value;
}
export async function shot(sid,path){ const r=await raw("Page.captureScreenshot",{format:"png"},sid); const fs=await import("node:fs"); fs.writeFileSync(path,Buffer.from(r.result.data,"base64")); return path; }
export async function waitFor(sid,expr,ms=10000){ const t0=Date.now(); while(Date.now()-t0<ms){const v=await ev(sid,expr); if(v)return v; await sleep(250);} return false; }
export const sleep = ms => new Promise(r=>setTimeout(r,ms));
export { raw as cdp };
