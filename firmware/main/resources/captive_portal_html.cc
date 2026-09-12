#include "resources/captive_portal_html.h"

// 嵌入式 HTML — Slate · Mono Press 风格重设计版
// 用户连了 SoftAP "Slate-XXXX" 后浏览器看到此页。无外网访问,只能用系统字体回退。
// 保持与原版完全一致的接口:GET /scan、POST /submit、占位 {{SERVER_URL}} 与 {{AP_SSID}}。
// 仅样式与排版重设计,字段、状态、交互、提示文案均不变。

namespace slate {

const char* const kCaptivePortalHtml = R"HTML(<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Slate · 配网</title>
<style>
:root {
  --ink:   #14110d;
  --paper: #f5f3ed;
  --band:  #ebe7dd;
  --line:  #d8d2c4;
  --mute:  #6b665d;
  --dim:   #a39d92;
  --red:   #a8281c;
}
* { box-sizing: border-box; }
html, body {
  margin: 0; padding: 0;
  background: var(--paper);
  color: var(--ink);
  font-family: "IBM Plex Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
  font-size: 14px;
  line-height: 1.55;
  -webkit-font-smoothing: antialiased;
}
.serif { font-family: "Songti SC", "STSong", "Source Han Serif SC", Georgia, serif; }
.mono  { font-family: ui-monospace, "IBM Plex Mono", "JetBrains Mono", SFMono-Regular, Menlo, Monaco, Consolas, monospace; }

.wrap { max-width: 560px; margin: 0 auto; padding: 28px 20px 64px; }

/* Masthead */
.masthead { padding-bottom: 8px; }
.kicker {
  display: flex; justify-content: space-between; align-items: baseline;
  font-family: ui-monospace, monospace;
  font-size: 10px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--mute);
}
.brand {
  font-family: "Songti SC", "STSong", "Source Han Serif SC", Georgia, serif;
  font-size: 48px; font-weight: 900;
  letter-spacing: -0.02em;
  margin: 6px 0 4px;
  line-height: 1;
}
.brand-dot { color: var(--ink); }
.tag {
  font-family: "Songti SC", "STSong", "Source Han Serif SC", Georgia, serif;
  font-style: italic;
  font-size: 16px;
  color: var(--mute);
  margin: 0 0 10px;
}
.rule       { height: 1px; background: var(--ink); }
.rule-thick { height: 3px; background: var(--ink); margin-top: 3px; }

/* Section */
section {
  margin-top: 32px;
}
section .sechead {
  display: flex; align-items: baseline; justify-content: space-between; gap: 14px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--ink);
  margin-bottom: 14px;
}
section .sechead .num {
  font-family: "Songti SC", "STSong", Georgia, serif;
  font-weight: 700; font-size: 13px; letter-spacing: 0.02em;
  flex: 0 0 auto;
}
section .sechead h2 {
  font-family: "Songti SC", "STSong", "Source Han Serif SC", Georgia, serif;
  font-size: 22px; font-weight: 700; letter-spacing: -0.01em;
  margin: 0; flex: 1;
}
section .sechead .eyebrow {
  font-family: ui-monospace, monospace;
  font-size: 10px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--mute);
  flex: 0 0 auto;
}

/* Networks list */
ul.networks {
  list-style: none; margin: 0; padding: 0;
  border: 1px solid var(--ink);
  max-height: 220px; overflow-y: auto;
  background: var(--paper);
}
ul.networks li {
  padding: 10px 14px;
  border-bottom: 1px solid var(--line);
  cursor: pointer;
  display: flex; justify-content: space-between; align-items: baseline; gap: 12px;
}
ul.networks li:last-child { border-bottom: 0; }
ul.networks li .ssid {
  font-family: "Songti SC", "STSong", Georgia, serif;
  font-size: 15px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
ul.networks li .ssid .open {
  font-family: ui-monospace, monospace;
  font-size: 9px; letter-spacing: 0.18em; color: var(--mute);
  margin-left: 6px;
}
ul.networks li .meta {
  font-family: ui-monospace, monospace;
  font-size: 11px; color: var(--mute);
  flex: 0 0 auto;
  display: flex; gap: 8px; align-items: baseline;
}
ul.networks li .bars { color: var(--ink); }
ul.networks li:hover { background: var(--band); }
ul.networks li.sel   { background: var(--ink); color: var(--paper); }
ul.networks li.sel .ssid .open,
ul.networks li.sel .meta,
ul.networks li.sel .bars { color: var(--paper); }
ul.networks .placeholder { cursor: default; color: var(--mute); }
ul.networks .placeholder:hover { background: var(--paper); }

/* Inputs */
label {
  display: block;
  margin-top: 16px;
}
label .lbl {
  display: block;
  font-family: ui-monospace, monospace;
  font-size: 10px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--mute);
  margin-bottom: 4px;
}
input[type=text],
input[type=password],
input[type=url] {
  width: 100%;
  padding: 8px 0 6px;
  background: transparent;
  color: var(--ink);
  font-family: "Songti SC", "STSong", "Source Han Serif SC", Georgia, serif;
  font-size: 20px;
  border: 0;
  border-bottom: 2px solid var(--ink);
  border-radius: 0;
  outline: none;
  -webkit-appearance: none;
}
input::placeholder {
  color: var(--dim);
  font-style: italic;
  font-size: 16px;
}
input:focus { border-bottom-color: var(--ink); }
.hint {
  margin: 6px 0 0;
  font-family: "Songti SC", "STSong", Georgia, serif;
  font-style: italic;
  font-size: 12px; color: var(--mute);
  line-height: 1.5;
}

/* Submit / button */
button.primary {
  display: flex; align-items: center; justify-content: space-between;
  width: 100%;
  margin-top: 28px;
  padding: 0 22px;
  height: 56px;
  background: var(--ink);
  color: var(--paper);
  font-family: "IBM Plex Sans", Helvetica, Arial, sans-serif;
  font-size: 13px; font-weight: 600; letter-spacing: 0.22em; text-transform: uppercase;
  border: 1px solid var(--ink);
  border-radius: 0;
  cursor: pointer;
}
button.primary:disabled { opacity: 0.45; cursor: not-allowed; }
button.primary:hover:not(:disabled) { background: var(--paper); color: var(--ink); }
button.primary .arrow { font-family: ui-monospace, monospace; font-size: 16px; letter-spacing: 0; }

/* Status */
.status {
  margin-top: 18px;
  padding: 12px 14px;
  border: 1px solid var(--mute);
  font-family: ui-monospace, monospace;
  font-size: 12px;
  background: var(--paper);
}
.status.info { border-color: var(--mute); color: var(--mute); }
.status.ok   { border-color: var(--ink);  color: var(--ink); }
.status.err  { border-color: var(--red);  color: var(--red); }
.status.err::before { content: "[ERROR] "; }
.status.info::before { content: "[…] "; }
.status.ok::before  { content: "[OK] "; }

/* Success block (after submit) */
.success {
  margin-top: 28px;
  border: 2px solid var(--ink);
  padding: 28px 24px;
}
.success .ok-eyebrow {
  font-family: ui-monospace, monospace;
  font-size: 10px; letter-spacing: 0.2em; text-transform: uppercase; color: var(--mute);
  margin: 0 0 8px;
}
.success h3 {
  font-family: "Songti SC", "STSong", "Source Han Serif SC", Georgia, serif;
  font-size: 32px; font-weight: 700; letter-spacing: -0.02em;
  margin: 0 0 14px;
  line-height: 1.05;
}
.success p {
  margin: 0 0 10px;
  font-size: 14px;
  line-height: 1.6;
}
.success p strong {
  font-family: "Songti SC", "STSong", Georgia, serif;
  font-weight: 700;
  border-bottom: 2px solid var(--ink);
  padding-bottom: 1px;
}
.success .cd {
  margin-top: 14px;
  font-family: ui-monospace, monospace;
  font-size: 11px; letter-spacing: 0.16em; text-transform: uppercase; color: var(--mute);
}
.success .cd #cd {
  color: var(--ink);
  font-weight: 700;
}

/* Spinner */
@keyframes spin {
  0%   { content: "[ / ]"; }
  25%  { content: "[ - ]"; }
  50%  { content: "[ \\ ]"; }
  75%  { content: "[ | ]"; }
  100% { content: "[ / ]"; }
}
.spin::before { content: "[ / ]"; animation: spin 480ms steps(4) infinite; }

/* Footer */
.foot {
  margin-top: 40px;
  padding-top: 14px;
  border-top: 1px solid var(--ink);
  display: flex; justify-content: space-between; align-items: baseline;
  font-family: ui-monospace, monospace;
  font-size: 10px; letter-spacing: 0.16em; text-transform: uppercase; color: var(--mute);
}
</style>
</head>
<body>
<div class="wrap">
  <header class="masthead">
    <div class="kicker">
      <span>第 〇 卷 · 配 网 portal</span>
      <span>{{AP_SSID}}</span>
    </div>
    <h1 class="brand">Slate<span class="brand-dot">.</span></h1>
    <p class="tag">案头那一块墨水屏，先递上凭证。</p>
    <div class="rule"></div>
    <div class="rule-thick"></div>
  </header>

  <form id="f">
    <section>
      <header class="sechead">
        <span class="num serif">I.</span>
        <h2>无线网络</h2>
        <span class="eyebrow">wifi</span>
      </header>
      <ul id="nets" class="networks">
        <li class="placeholder"><span class="ssid">扫描中</span><span class="meta"><span class="spin"></span></span></li>
      </ul>
      <label>
        <span class="lbl">ssid</span>
        <input id="ssid" name="ssid" type="text" required maxlength="32" placeholder="点上方列表自动填入，或手动输入">
      </label>
      <label>
        <span class="lbl">password</span>
        <input id="password" name="password" type="password" maxlength="64" placeholder="开放网络留空即可">
      </label>
    </section>

    <section>
      <header class="sechead">
        <span class="num serif">II.</span>
        <h2>服务地址</h2>
        <span class="eyebrow">server</span>
      </header>
      <label>
        <span class="lbl">url</span>
        <input id="server_url" name="server_url" type="url" required
               value="{{SERVER_URL}}"
               placeholder="https://slate.your-domain.com">
      </label>
      <p class="hint">填运行 slate 后端的地址。本地调试用 http://&lt;LAN-IP&gt;:9494 即可。</p>
    </section>

    <button type="submit" class="primary" id="btn">
      <span>配网并连接</span>
      <span class="arrow">⏎ →</span>
    </button>
  </form>

  <div id="status"></div>

  <footer class="foot">
    <span>slate · v0.1.0</span>
    <span>192.168.4.1</span>
  </footer>
</div>

<script>
const $ = (id) => document.getElementById(id);
const status = $('status');
const setStatus = (msg, kind) => {
  status.className = 'status ' + (kind || 'info');
  status.textContent = msg;
};
const rssiBars = (r) => {
  if (r >= -55) return '████';
  if (r >= -65) return '███▒';
  if (r >= -75) return '██▒▒';
  if (r >= -85) return '█▒▒▒';
  return '▒▒▒▒';
};
let scanning = false;
async function scan() {
  if (scanning) return;
  scanning = true;
  try {
    const r = await fetch('/scan', { cache: 'no-store' });
    if (!r.ok) throw new Error('scan failed');
    const list = await r.json();
    const ul = $('nets');
    ul.innerHTML = '';
    if (!list.length) {
      ul.innerHTML = '<li class="placeholder"><span class="ssid">未扫到网络</span><span class="meta">—</span></li>';
      return;
    }
    for (const ap of list) {
      const li = document.createElement('li');
      const left = document.createElement('span');
      left.className = 'ssid';
      const ssidText = document.createTextNode(ap.ssid);
      left.appendChild(ssidText);
      if (ap.authmode === 0) {
        const open = document.createElement('span');
        open.className = 'open';
        open.textContent = 'open';
        left.appendChild(open);
      }
      const right = document.createElement('span');
      right.className = 'meta';
      const bars = document.createElement('span');
      bars.className = 'bars';
      bars.textContent = rssiBars(ap.rssi);
      const dbm = document.createElement('span');
      dbm.textContent = ap.rssi + 'dBm';
      right.appendChild(bars);
      right.appendChild(dbm);
      li.appendChild(left);
      li.appendChild(right);
      li.addEventListener('click', () => {
        $('ssid').value = ap.ssid;
        for (const n of ul.querySelectorAll('li')) n.classList.remove('sel');
        li.classList.add('sel');
      });
      ul.appendChild(li);
    }
  } catch (e) {
    $('nets').innerHTML = '<li class="placeholder"><span class="ssid">扫描失败 · 重试中</span><span class="meta"><span class="spin"></span></span></li>';
  } finally {
    scanning = false;
  }
}
scan();
const scanTimer = setInterval(scan, 5000);

function showSuccess(ssid) {
  document.querySelector('form').style.display = 'none';
  status.style.display = 'none';
  const wrap = document.querySelector('.wrap');
  const box = document.createElement('section');
  box.className = 'success';
  const eyebrow = document.createElement('p');
  eyebrow.className = 'ok-eyebrow';
  eyebrow.textContent = '✓ 配 网 成 功 · paired';
  const h3 = document.createElement('h3');
  h3.textContent = '设备即将自启。';
  const p1 = document.createElement('p');
  p1.textContent = '设备将连接到 ';
  const strong = document.createElement('strong');
  strong.textContent = ssid;
  p1.appendChild(strong);
  p1.appendChild(document.createTextNode('，并在数秒内自动重启。'));
  const p2 = document.createElement('p');
  p2.textContent = '设备屏幕也会同步显示「配网成功」与重启倒计时。';
  const p3 = document.createElement('p');
  p3.className = 'cd';
  p3.textContent = '本页将在 ';
  const cdSpan = document.createElement('span');
  cdSpan.id = 'cd';
  cdSpan.textContent = '5';
  p3.appendChild(cdSpan);
  p3.appendChild(document.createTextNode(' 秒后失联，可直接关闭。'));
  box.appendChild(eyebrow);
  box.appendChild(h3);
  box.appendChild(p1);
  box.appendChild(p2);
  box.appendChild(p3);
  wrap.insertBefore(box, document.querySelector('.foot'));
  let n = 5;
  const t = setInterval(() => {
    n--;
    const cd = document.getElementById('cd');
    if (cd) cd.textContent = n;
    if (n <= 0) clearInterval(t);
  }, 1000);
}

$('f').addEventListener('submit', async (e) => {
  e.preventDefault();
  const body = {
    ssid: $('ssid').value.trim(),
    password: $('password').value,
    server_url: $('server_url').value.trim(),
  };
  if (!body.ssid) { setStatus('SSID 不能空', 'err'); return; }
  if (!body.server_url) { setStatus('服务端 URL 不能空', 'err'); return; }
  $('btn').disabled = true;
  setStatus('正在试连 ' + body.ssid + ' …（最多 10 秒）', 'info');
  try {
    const r = await fetch('/submit', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await r.json();
    if (!r.ok || !data.success) {
      setStatus((data.error || ('HTTP ' + r.status)) + ' · 请改密码后重试', 'err');
      $('btn').disabled = false;
      return;
    }
    clearInterval(scanTimer);
    showSuccess(body.ssid);
  } catch (err) {
    setStatus('网络错误: ' + (err && err.message || '未知') + ' · 请重试', 'err');
    $('btn').disabled = false;
  }
});
</script>
</body>
</html>
)HTML";

}  // namespace slate
