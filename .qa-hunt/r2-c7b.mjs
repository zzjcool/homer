setTimeout(() => process.exit(9), 28_000).unref();
import { connect, sleep, login } from "/root/code/homer/.qa-hunt/repro2-lib.mjs";
const { send, ev, sid, shot, events } = await connect();
await send("Page.enable", {}, sid);
await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sid);
await send("Network.enable", {}, sid);
await send("Page.navigate", { url: "http://hub:7760/" }, sid);
for (let i = 0; i < 20; i++) { await sleep(400); try { if (await ev("document.readyState === 'complete'")) break; } catch (e) {} }
await login(ev);
await send("Network.setBlockedURLs", { urls: ["*join/status*"] }, sid);
events.length = 0;
await ev("document.querySelector('#btn-join-top').click()");
for (let i = 0; i < 20; i++) { await sleep(300); if (await ev("document.querySelector('#join-cmd').value.indexOf('hr_') === 0")) break; }
await sleep(7000);
const n = events.filter(e => e.method === "Network.requestWillBeSent" && /join\/status/.test(e.params.request.url)).length;
const nf = events.filter(e => (e.method === "Network.loadingFailed") && JSON.stringify(e.params).includes("join/status")).length;
const st = await ev(`({ open: document.querySelector('#dlg-join').open, text: document.querySelector('#dlg-join').textContent.replace(/\\s+/g,' ').trim(), toast: document.querySelector('#toast').textContent, wait: document.querySelector('#join-wait').textContent })`);
console.log("blocked: requests=", n, "loadingFailed=", nf, "open=", st.open);
console.log("text:", JSON.stringify(st.text));
console.log("toast:", JSON.stringify(st.toast), "wait:", JSON.stringify(st.wait));
console.log("hasErrorWording:", /失败|错误|重试|无法|连不上|断开/.test(st.text));
await shot("C7-b-join-poll-silent.png");
await send("Network.setBlockedURLs", { urls: [] }, sid);
process.exit(0);
