setTimeout(() => process.exit(9), 28_000).unref();
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Network.enable", {}, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 25; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'") ) break; } catch (e) {} }
console.log("loggedIn:", await login(ev));
const count = () => events.filter(e => e.method === "Network.requestWillBeSent" && /join\/status/.test(e.params.request.url)).length;
const fails = () => events.filter(e => e.method === "Network.loadingFailed" && /join\/status/.test(e.params && e.params.request ? e.params.request.url : "")).length;

// control：不阻断，观察轮询调用数与失败数
events.length = 0;
await ev("document.querySelector('#btn-join-top').click()");
for (let i = 0; i < 30; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
await sleep(5000);
console.log("[control] join/status requests:", count(), "loadingFailed:", fails(), "| dlg open:", await ev("document.querySelector('#dlg-join').open"));
await ev("document.querySelector('#dlg-join').close()");
await sleep(600);

// 实验：CDP 层阻断 join/status（不动 window.fetch）
await send("Network.setBlockedURLs", { urls: ["*join/status*"] }, sid);
events.length = 0;
await ev("document.querySelector('#btn-join-top').click()");
for (let i = 0; i < 30; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
await sleep(7000);
const st = await ev(`({
  open: document.querySelector('#dlg-join').open,
  text: document.querySelector('#dlg-join').textContent.replace(/\\s+/g,' ').trim(),
  toast: document.querySelector('#toast').textContent,
  joinWait: document.querySelector('#join-wait').textContent
})`);
console.log("[exp] blocked join/status -> requests:", count(), "loadingFailed:", fails());
console.log("[exp] dialog open:", st.open);
console.log("[exp] dialog text:", JSON.stringify(st.text));
console.log("[exp] toast:", JSON.stringify(st.toast), "| join-wait:", JSON.stringify(st.joinWait));
const hasErr = /失败|错误|重试|无法|连不上|断开/.test(st.text);
console.log("[exp] any error wording in dialog:", hasErr);
await shot("C7-join-poll-silent-blocked.png");
await send("Network.setBlockedURLs", { urls: [] }, sid);
process.exit(0);
