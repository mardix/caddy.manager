'use strict';
/* caddy.manager — single-file vanilla JS UI */
const $app = document.getElementById('app');
const $modal = document.getElementById('modal');

// ---------- helpers ----------
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const clone = (o) => JSON.parse(JSON.stringify(o));
const rid = () => Array.from(crypto.getRandomValues(new Uint8Array(6)), (b) => b.toString(16).padStart(2, '0')).join('');
const fmtN = (n) => Number(n || 0).toLocaleString();
const fmtB = (b) => { b = Number(b || 0); const u = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0; while (b >= 1024 && i < 4) { b /= 1024; i++; } return (i ? b.toFixed(1) : b) + ' ' + u[i]; };
const pad = (n) => String(n).padStart(2, '0');
function fmtTime(ms) {
  const d = new Date(ms), now = new Date();
  const t = pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
  return d.toDateString() === now.toDateString() ? t : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) + ' ' + t.slice(0, 5);
}
function ago(sec) {
  const s = Math.max(0, Date.now() / 1000 - sec);
  if (s < 60) return 'just now';
  if (s < 3600) return Math.floor(s / 60) + ' min ago';
  if (s < 86400) return Math.floor(s / 3600) + ' h ago';
  return Math.floor(s / 86400) + ' d ago';
}
const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };

const I = {
  sparkle: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z"/></svg>',
  gear: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>',
  x: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  back: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 6l-6 6 6 6"/></svg>',
  plus: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg>',
  up: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 16V4M6 10l6-6 6 6M4 20h16"/></svg>',
  down: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4v12M6 10l6 6 6-6M4 20h16"/></svg>',
  proxy: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 12h16M14 6l6 6-6 6"/></svg>',
  redirect: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 18v-6a5 5 0 0 1 5-5h11M16 3l4 4-4 4"/></svg>',
  dead: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M6 18L18 6"/></svg>',
  check: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12l5 5 9-10"/></svg>',
  alert: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" aria-hidden="true"><path d="M12 7v6M12 17h.01"/></svg>',
};

// ---------- api ----------
async function api(path, opts = {}) {
  const o = { method: opts.method || 'GET', headers: { 'X-Requested-With': 'caddy-manager' } };
  if (opts.form) o.body = opts.form;
  else if (opts.body !== undefined) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(opts.body); }
  const r = await fetch(path, o);
  if (r.status === 401 && path !== '/api/login') { S.authed = false; closeModal(); renderLogin(); throw new Error('Please sign in'); }
  const ct = r.headers.get('content-type') || '';
  const data = ct.includes('json') ? await r.json() : await r.text();
  if (!r.ok) throw new Error((data && data.error) || 'Request failed (' + r.status + ')');
  return data;
}
const post = (p, body) => api(p, { method: 'POST', body: body || {} });

// ---------- global state ----------
const S = { authed: false, state: null, caddyfile: '', status: null, settings: null };
let P = { key: '', data: {}, loading: {} }; // per-page cache
let PROPOSAL = null;
let pollTimer = null;

async function loadState() {
  const [st, status, settings] = await Promise.all([api('/api/state'), api('/api/status'), api('/api/settings')]);
  S.state = st.state; S.caddyfile = st.caddyfile; S.status = status; S.settings = settings;
}
async function refreshStatus() { try { S.status = await api('/api/status'); } catch (e) { } }

function need(key, loader) {
  if (key in P.data || P.loading[key]) return P.data[key];
  P.loading[key] = true;
  const pageKey = P.key;
  loader().then((v) => { if (P.key !== pageKey) return; P.data[key] = v; delete P.loading[key]; rerender(); })
    .catch((e) => { if (P.key !== pageKey) return; P.data[key] = { error: e.message }; delete P.loading[key]; rerender(); });
  return undefined;
}

function toast(msg, bad) {
  const t = document.createElement('div');
  t.className = 'toast' + (bad ? ' bad' : '');
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), bad ? 6000 : 3000);
}

// ---------- routing ----------
const routeParts = () => (location.hash.replace(/^#\/?/, '').split('?')[0] || '').split('/').filter(Boolean);
window.addEventListener('hashchange', () => render());

function render() {
  if (!S.authed) return renderLogin();
  const key = location.hash || '#/';
  if (P.key !== key) { P = { key, data: {}, loading: {} }; clearInterval(pollTimer); pollTimer = null; window.scrollTo(0, 0); }
  rerender();
}

function rerender() {
  if (!S.authed) return;
  const p = routeParts();
  let html;
  if (!p.length || p[0] === 'overview') html = pageOverview();
  else if (p[0] === 'hosts') html = pageHosts();
  else if (p[0] === 'host' && p[2] === 'log') html = pageHostLog(p[1]);
  else if (p[0] === 'host') html = pageEditor(p[1]);
  else if (p[0] === 'access') html = pageAccess();
  else if (p[0] === 'logs') html = pageLogs();
  else if (p[0] === 'settings') html = pageSettings();
  else html = shell('', '<div class="card empty">Page not found. <a href="#/">Go to overview</a></div>');
  const y = window.scrollY;
  $app.innerHTML = html;
  window.scrollTo(0, y);
  afterRender();
}

let afterHooks = [];
function onAfter(fn) { afterHooks.push(fn); }
function afterRender() { const h = afterHooks; afterHooks = []; h.forEach((f) => f()); }

// ---------- login ----------
function renderLogin() {
  $app.innerHTML = `<div class="login"><form id="loginForm">
    <div class="logo" style="font-size:26px">caddy<b>.</b>manager</div>
    <label class="f">Admin password<input class="in" type="password" name="password" autocomplete="current-password" autofocus required></label>
    <div id="loginErr" class="note bad" hidden></div>
    <button class="btn primary" type="submit">Sign in</button>
    <p class="hint">The password is set with ADMIN_PASSWORD in your docker-compose file.</p>
  </form></div>`;
  document.getElementById('loginForm').onsubmit = async (e) => {
    e.preventDefault();
    const btn = e.target.querySelector('button'); btn.disabled = true;
    try {
      await api('/api/login', { method: 'POST', body: { password: e.target.password.value } });
      S.authed = true; await loadState(); render();
    } catch (err) {
      const el = document.getElementById('loginErr'); el.hidden = false; el.textContent = err.message; btn.disabled = false;
    }
  };
}

// ---------- shell ----------
function aiOn() { return !!(S.settings && S.settings.ai && S.settings.ai.enabled); }
function shell(active, body) {
  const nav = [['overview', '#/', 'Overview'], ['hosts', '#/hosts', 'Hosts'], ['access', '#/access', 'Access lists'], ['logs', '#/logs', 'Logs &amp; stats']]
    .map(([k, h, l]) => `<a href="${h}" class="${active === k ? 'on' : ''}">${l}</a>`).join('');
  const c = S.status && S.status.caddy;
  const st = c ? (c.running ? '<span class="dot up"></span>Caddy running' : '<span class="dot down"></span>Caddy stopped') : '';
  const manual = S.state && S.state.manual ? `<div class="note warn between"><span class="fill"><b>Running an uploaded Caddyfile.</b> Host and access-list changes are saved but won't take effect until you switch back to managed hosts.</span><button class="btn sm" data-a="unmanual">Switch back to managed hosts</button></div>` : '';
  return `<div class="wrap"><header class="top">
    <a class="logo" href="#/">caddy<b>.</b>manager</a>
    <nav class="nav" aria-label="Main">${nav}</nav>
    <span class="pill-status hide-sm">${st}</span>
    ${aiOn() ? `<button class="btn sm" data-a="ask"><span style="color:var(--ai)">${I.sparkle}</span>Ask <span class="kbd">⌘K</span></button>` : ''}
    <a class="icon-btn ${active === 'settings' ? 'on' : ''}" href="#/settings" aria-label="Settings">${I.gear}</a>
  </header>${manual}${body}<footer class="foot">with <span aria-label="love">❤️</span> <a href="https://github.com/mardix/caddy.manager" target="_blank" rel="noopener noreferrer">caddy.manager</a></footer></div>`;
}

// ---------- shared bits ----------
const hostName = (h) => (h.domains && h.domains[0]) || 'New host';
const hs = (id) => (S.status && S.status.hosts && S.status.hosts[id]) || { health: 'na', cert: 'none' };
function healthLabel(h, s) {
  if (!h.enabled) return ['na', 'Off'];
  if (h.type !== 'proxy') return ['na', 'Active'];
  if (s.health === 'up') return ['up', 'Up · ' + s.ms + ' ms'];
  if (s.health === 'slow') return ['slow', 'Slow · ' + s.ms + ' ms'];
  if (s.health === 'down') return ['down', 'Down'];
  return ['unknown', 'Checking…'];
}
function certTag(h, s) {
  if (h.proto === 'http') return '<span class="tag t-muted">HTTP only</span>';
  const wild = h.domains.some((d) => d.startsWith('*.'));
  if (s.cert === 'valid') return `<span class="tag ${wild ? 't-wild' : 't-ok'}">HTTPS${wild ? ' · wildcard' : ''}</span>`;
  if (s.cert === 'expiring') return `<span class="tag t-warn">Renews in ${s.certDays} d</span>`;
  if (s.cert === 'failed') return '<span class="tag t-bad">Certificate failed</span>';
  return '<span class="tag t-muted">HTTPS · pending</span>';
}
const issuerName = (i) => (/letsencrypt/.test(i || '') ? 'Let’s Encrypt' : /zerossl/.test(i || '') ? 'ZeroSSL' : i === 'local' ? 'Caddy local CA' : i === 'custom' ? 'your certificate' : esc(i || ''));
const typeIco = (t) => `<span class="typeico t-${t}" title="${t === 'dead' ? '404 host' : t}">${I[t] || I.proxy}</span>`;
function accessLabel(h) {
  if (h.type !== 'proxy') return '—';
  if (h.access.mode === 'list') { const l = S.state.accessLists.find((x) => x.id === h.access.listId); return l ? esc(l.name) : 'Missing list'; }
  if (h.access.mode === 'custom') return `Custom · ${h.access.users.length} login${h.access.users.length === 1 ? '' : 's'}${h.access.rules.length ? ', ' + h.access.rules.length + ' IP rule' + (h.access.rules.length === 1 ? '' : 's') : ''}`;
  return 'Public';
}
function destLabel(h) {
  if (h.type === 'redirect') return `${h.redirectCode} → ${esc(h.redirectTo)}${h.keepPath ? '{uri}' : ''}`;
  if (h.type === 'dead') return h.deadCode === 'abort' ? 'Close connection' : `${h.deadCode} “${esc(h.deadMsg || 'Not found')}”`;
  return esc(h.upstreams.join(', ')) + (h.paths.length ? ` <span class="muted">+${h.paths.length} path${h.paths.length > 1 ? 's' : ''}</span>` : '');
}

function bars(buckets, height) {
  const max = Math.max(1, ...buckets.map((b) => b.ok + b.err));
  return `<div class="bars" style="height:${height || 140}px" role="img" aria-label="Requests over time">${buckets.map((b) => {
    const tot = b.ok + b.err; const h = (tot / max) * 100; const eh = tot ? (b.err / tot) * h : 0;
    return `<span class="b" title="${fmtN(b.ok)} ok, ${fmtN(b.err)} errors"><i class="e" style="height:${eh}%"></i><i style="height:${h - eh}%;${tot ? '' : 'min-height:2px;background:var(--chip)'}"></i></span>`;
  }).join('')}</div>`;
}
function ticks(from, step, n) {
  const pts = [0, Math.floor(n / 4), Math.floor(n / 2), Math.floor((3 * n) / 4), n - 1];
  return `<div class="ticks">${pts.map((i) => { const d = new Date(from + i * step); return `<span>${step >= 86400000 / 4 ? d.toLocaleDateString(undefined, { weekday: 'short' }) + ' ' + pad(d.getHours()) + 'h' : pad(d.getHours()) + ':' + pad(d.getMinutes())}</span>`; }).join('')}</div>`;
}

function mdLite(text) {
  const lines = esc(text).split('\n');
  let out = '', list = '';
  const inline = (s) => s.replace(/\*\*(.+?)\*\*/g, '<b>$1</b>').replace(/`([^`]+)`/g, '<code>$1</code>');
  const flush = () => { if (list) { out += `</${list}>`; list = ''; } };
  let para = [];
  const flushP = () => { if (para.length) { out += '<p>' + inline(para.join(' ')) + '</p>'; para = []; } };
  for (const raw of lines) {
    const l = raw.trim();
    let m;
    if ((m = l.match(/^[-*•]\s+(.*)/))) { flushP(); if (list !== 'ul') { flush(); out += '<ul>'; list = 'ul'; } out += '<li>' + inline(m[1]) + '</li>'; }
    else if ((m = l.match(/^\d+[.)]\s+(.*)/))) { flushP(); if (list !== 'ol') { flush(); out += '<ol>'; list = 'ol'; } out += '<li>' + inline(m[1]) + '</li>'; }
    else if (!l) { flushP(); flush(); }
    else if ((m = l.match(/^#+\s+(.*)/))) { flushP(); flush(); out += '<p><b>' + inline(m[1]) + '</b></p>'; }
    else { flush(); para.push(l); }
  }
  flushP(); flush();
  return out;
}

// ---------- modal ----------
function openModal(title, body, opts = {}) {
  $modal.innerHTML = `<div class="modal-bg" data-a="modal-bg"><div class="modal" role="dialog" aria-modal="true" aria-label="${esc(title)}" style="${opts.width ? 'max-width:' + opts.width + 'px' : ''}">
    <div class="mh">${opts.icon || ''}<h2>${esc(title)}</h2><button class="x" data-a="close-modal" aria-label="Close">${I.x}</button></div>
    <div class="mb" id="mbody">${body}</div></div></div>`;
  const f = $modal.querySelector('[autofocus]'); if (f) f.focus();
}
const setModalBody = (html) => { const b = document.getElementById('mbody'); if (b) b.innerHTML = html; };
function closeModal() { $modal.innerHTML = ''; }

// ---------- diff ----------
function diffHTML(a, b) {
  const A = (a || '').split('\n'), B = (b || '').split('\n');
  const n = A.length, m = B.length;
  if (n * m > 6e6) return '<div class="gap">File too large to compare line by line.</div>';
  const W = m + 1, dp = new Uint32Array((n + 1) * W);
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i * W + j] = A[i] === B[j] ? dp[(i + 1) * W + j + 1] + 1 : Math.max(dp[(i + 1) * W + j], dp[i * W + j + 1]);
  const ops = []; let i = 0, j = 0;
  while (i < n && j < m) { if (A[i] === B[j]) { ops.push([' ', A[i]]); i++; j++; } else if (dp[(i + 1) * W + j] >= dp[i * W + j + 1]) { ops.push(['-', A[i++]]); } else { ops.push(['+', B[j++]]); } }
  while (i < n) ops.push(['-', A[i++]]); while (j < m) ops.push(['+', B[j++]]);
  if (!ops.some((o) => o[0] !== ' ')) return '<div class="gap">No changes to the Caddyfile.</div>';
  const keep = new Array(ops.length).fill(false);
  ops.forEach((o, k) => { if (o[0] !== ' ') for (let d = -3; d <= 3; d++) if (ops[k + d]) keep[k + d] = true; });
  let html = '', skipped = 0;
  ops.forEach((o, k) => {
    if (!keep[k]) { skipped++; return; }
    if (skipped) { html += `<div class="gap">… ${skipped} unchanged line${skipped > 1 ? 's' : ''}</div>`; skipped = 0; }
    html += `<div class="${o[0] === '+' ? 'a' : o[0] === '-' ? 'd' : ''}">${o[0]} ${esc(o[1])}</div>`;
  });
  if (skipped) html += `<div class="gap">… ${skipped} unchanged line${skipped > 1 ? 's' : ''}</div>`;
  return html;
}

// ---------- review & apply ----------
let REVIEW = null;
async function review(payload, label, after) {
  REVIEW = { payload, label, after };
  openModal(label || 'Review changes', '<div class="between"><span class="spin"></span><span class="muted">Checking the new Caddyfile…</span></div>');
  try {
    const r = await post('/api/preview', { ...payload, label });
    REVIEW.res = r;
    showReview(r);
  } catch (e) {
    REVIEW.res = { valid: false, error: e.message };
    setModalBody(`<div class="ck fail">${I.alert}<span><b>Can't apply this yet</b>${esc(e.message)}</span></div>
      <div class="between">${aiOn() ? `<button class="btn ai" data-a="explain-review">${I.sparkle}Explain</button>` : ''}<span class="fill"></span><button class="btn" data-a="close-modal">Back to editing</button></div>`);
  }
}
function showReview(r) {
  const checks = (r.checks || []).map((c) => `<div class="ck ${c.status}">${c.status === 'ok' ? I.check : I.alert}<span><b>${esc(c.name)}</b>${esc(c.detail)}</span></div>`).join('');
  setModalBody(`<div class="checks">${checks}</div>
    <div class="stack s8"><h3>Caddyfile changes</h3><div class="diff">${diffHTML(r.current, r.caddyfile)}</div></div>
    <div class="between">
      ${r.token ? '<button class="btn primary" data-a="apply">Apply &amp; reload</button>' : '<button class="btn" disabled>Fix the errors to apply</button>'}
      <button class="btn ghost" data-a="close-modal">Cancel</button>
      ${!r.valid && aiOn() ? `<button class="btn ai" data-a="explain-review">${I.sparkle}Explain</button>` : ''}
      <span class="fill"></span><span class="hint">The current Caddyfile is backed up first.</span>
    </div>`);
}
async function doApply(btn) {
  btn.disabled = true; btn.textContent = 'Applying…';
  try {
    await post('/api/apply', { token: REVIEW.res.token });
    closeModal();
    toast('Applied — Caddy reloaded');
    await loadState();
    const after = REVIEW.after; REVIEW = null;
    P.data = {}; // refresh page data
    if (after) after(); else rerender();
  } catch (e) {
    setModalBody(`<div class="ck fail">${I.alert}<span><b>Not applied</b>${esc(e.message)}</span></div>
      <div class="between">${aiOn() ? `<button class="btn ai" data-a="explain-review">${I.sparkle}Explain</button>` : ''}<span class="fill"></span><button class="btn" data-a="close-modal">Close</button></div>`);
    REVIEW.res = { valid: false, error: e.message };
  }
}
function saveHosts(hosts, label, after) { return review({ hosts }, label, after); }

// ---------- AI: explain & ask ----------
async function explain(error, hostId) {
  openModal('What went wrong', `<div class="note bad mono" style="white-space:pre-wrap">${esc(error)}</div><div class="between"><span class="spin"></span><span class="muted">Checking DNS, upstreams and recent events…</span></div>`, { icon: `<span style="color:var(--ai)">${I.sparkle}</span>` });
  try {
    const r = await post('/api/ai', { mode: 'explain', error, hostId: hostId || '' });
    setModalBody(`<div class="note bad mono" style="white-space:pre-wrap;max-height:160px;overflow:auto">${esc(error)}</div><div class="ans">${mdLite(r.text)}<span class="basis">${esc(r.basis)}${S.settings.ai.maskIps ? ' · IPs masked' : ''} · nothing was changed</span></div>`);
  } catch (e) { setModalBody(`<div class="note bad">${esc(e.message)}</div>`); }
}

function openAsk() {
  if (!aiOn()) { location.hash = '#/settings'; return; }
  const sugg = ['Add a host for photos.example.com that forwards to host.docker.internal:2342', 'Which hosts are public with no access list?', 'Redirect example.org to https://example.com and keep the path'];
  openModal('Ask', `<form id="askForm" class="line"><label class="sr" for="askIn">Ask</label><input id="askIn" class="in" autofocus placeholder="Describe a change, or ask about your setup…" style="flex:1 1 300px"><button class="btn ai" type="submit">Ask</button></form>
    <div class="chips">${sugg.map((s) => `<button class="sug" data-a="ask-sug">${esc(s)}</button>`).join('')}</div><div id="askOut"></div>`, { icon: `<span style="color:var(--ai)">${I.sparkle}</span>`, width: 760 });
  document.getElementById('askForm').onsubmit = (e) => { e.preventDefault(); runAsk(document.getElementById('askIn').value); };
}
async function runAsk(q) {
  q = (q || '').trim(); if (!q) return;
  const out = document.getElementById('askOut');
  out.innerHTML = '<div class="between"><span class="spin"></span><span class="muted">Thinking…</span></div>';
  try {
    const r = await post('/api/ai', { mode: 'ask', question: q });
    let html = `<div class="ans">${mdLite(r.text || '')}<span class="basis">${esc(r.basis)} · nothing changes until you apply</span></div>`;
    if (r.proposal) {
      PROPOSAL = r.proposal;
      html += `<div class="stack s8"><h3>Proposed change${r.proposal.id ? ' to ' + esc(hostName(r.proposal)) : ' — new host'}</h3>${(r.changes || []).map((c) => `<div class="note mono">${esc(c)}</div>`).join('')}</div>
        <div class="between"><button class="btn primary" data-a="open-proposal">Open in editor</button><span class="hint">Review it there, then apply.</span></div>`;
    }
    out.innerHTML = html;
  } catch (e) { out.innerHTML = `<div class="note bad">${esc(e.message)}</div>`; }
}

// AI panel for logs (host or all)
function aiPanel(key, hostId, range) {
  if (!aiOn()) return `<div class="aibox"><div class="hd">${I.sparkle}<h2>Ask about ${hostId ? 'this log' : 'your logs'}</h2></div><div class="bd"><p class="hint">Turn on the AI assistant in <a href="#/settings">Settings</a> to ask questions about traffic and errors.</p></div></div>`;
  const th = (P.data['thread:' + key] = P.data['thread:' + key] || []);
  const sugg = hostId ? ['What errors happened and why?', 'Who is hitting this site the most?', 'Is anything slow?', 'Any signs of attacks?'] : ['What happened in this period?', 'Which IPs look like attacks?', 'Which host has the most errors?', 'Which host uses the most bandwidth?'];
  const asked = new Set(th.map((m) => m.q));
  return `<div class="aibox"><div class="hd">${I.sparkle}<h2>Ask about ${hostId ? 'this log' : 'your logs'}</h2><span style="font-size:12px">${{ '1h': 'Last hour', '24h': 'Last 24 hours', '7d': 'Last 7 days' }[range] || ''}</span></div>
    <div class="bd">${th.map((m) => `<div class="q">${esc(m.q)}</div>${m.loading ? '<div class="ans"><span class="spin"></span></div>' : m.error ? `<div class="note bad">${esc(m.error)}</div>` : `<div class="ans">${mdLite(m.a)}<span class="basis">${esc(m.basis)}${S.settings.ai.maskIps ? ' · IPs masked' : ''}</span></div>`}`).join('')}
    <div class="chips">${sugg.filter((s) => !asked.has(s)).map((s) => `<button class="sug" data-a="ai-sug" data-k="${key}" data-h="${esc(hostId || '')}" data-r="${range}">${esc(s)}</button>`).join('')}</div>
    <form class="line" data-aiform="${key}" data-h="${esc(hostId || '')}" data-r="${range}"><label class="sr" for="aiq-${key}">Question</label><input id="aiq-${key}" class="in" placeholder="Ask about traffic, errors, clients…" style="border-color:var(--ai-line)"><button class="btn ai" type="submit">Ask</button></form>
    <p class="hint">Uses only ${hostId ? 'this host’s' : 'the filtered'} log for the selected range. Password hashes are never sent${S.settings.ai.maskIps ? '; IPs are masked' : ''}.</p></div></div>`;
}
async function askLogs(key, hostId, range, q) {
  q = (q || '').trim(); if (!q) return;
  const th = P.data['thread:' + key] = P.data['thread:' + key] || [];
  const m = { q, loading: true }; th.push(m); rerender();
  try { const r = await post('/api/ai', { mode: 'logs', hostId, range, question: q }); m.a = r.text; m.basis = r.basis; }
  catch (e) { m.error = e.message; }
  m.loading = false; rerender();
}

// ---------- pages: overview ----------
function pageOverview() {
  const stats = need('stats', () => api('/api/stats?range=24h'));
  const backups = need('backups', () => api('/api/backups'));
  if (!pollTimer) pollTimer = setInterval(async () => { if ($modal.innerHTML) return; await refreshStatus(); delete P.data.stats; rerender(); }, 20000);
  const hosts = S.state.hosts;
  const c = S.status.caddy;
  const proxies = hosts.filter((h) => h.enabled && h.type === 'proxy');
  const healthy = proxies.filter((h) => ['up', 'slow'].includes(hs(h.id).health)).length;
  const att = [];
  if (!c.running) att.push({ k: 'bad', t: 'Caddy is not running', d: c.error || 'The Caddy process stopped.', act: '<button class="btn sm" data-a="restart-caddy">Restart Caddy</button>', ex: 'Caddy process stopped: ' + (c.error || '') });
  for (const h of hosts.filter((x) => x.enabled)) {
    const s = hs(h.id);
    if (h.type === 'proxy' && s.health === 'down') att.push({ k: 'bad', t: `${hostName(h)} is down`, d: `${h.upstreams.join(', ')} isn't answering${s.error ? ' — ' + s.error : ''}`, act: `<a class="btn sm" href="#/host/${h.id}">Open host</a>`, ex: `Upstream for ${hostName(h)} (${h.upstreams.join(', ')}) is not reachable: ${s.error || 'connection failed'}`, hid: h.id });
    if (h.type === 'proxy' && s.health === 'slow') att.push({ k: 'warn', t: `${hostName(h)} is slow`, d: `Upstream took ${s.ms} ms to accept a connection`, act: `<a class="btn sm" href="#/host/${h.id}/log">View log</a>` });
    if (s.cert === 'failed') att.push({ k: 'bad', t: `Certificate problem on ${hostName(h)}`, d: s.certError || 'Caddy could not get a certificate', act: `<a class="btn sm" href="#/host/${h.id}">Open host</a>`, ex: s.certError || 'Certificate could not be obtained for ' + hostName(h), hid: h.id });
    if (s.cert === 'expiring') att.push({ k: 'warn', t: `Certificate for ${hostName(h)} expires in ${s.certDays} days`, d: 'Caddy renews certificates well before they expire, so renewal may be failing.', act: `<a class="btn sm" href="#/host/${h.id}">Open host</a>`, ex: `Certificate for ${hostName(h)} expires in ${s.certDays} days and has not been renewed`, hid: h.id });
  }
  const attHTML = att.length ? att.map((a, i) => `<div class="att ${a.k}"><span class="dot ${a.k === 'bad' ? 'down' : 'slow'}"></span><div style="flex:1 1 260px;min-width:0"><div class="t">${esc(a.t)}</div><div class="d">${esc(a.d)}</div></div>${a.act}${a.ex && aiOn() ? `<button class="btn sm ai" data-a="explain" data-err="${esc(a.ex)}" data-h="${a.hid || ''}">${I.sparkle}Explain</button>` : ''}</div>`).join('')
    : '<div class="att info"><span class="dot up"></span><div class="t">All good — nothing needs your attention.</div></div>';
  let traffic = '<div class="muted">Loading…</div>';
  if (stats && !stats.error) traffic = `<div class="between" style="margin-bottom:14px"><h2 class="fill">Traffic · last 24 hours</h2>
      <span class="muted">Requests <b style="color:var(--ink)">${fmtN(stats.total)}</b></span><span class="muted">5xx <b style="color:var(--ink)">${fmtN(stats.err5)}</b></span><span class="muted">Data <b style="color:var(--ink)">${fmtB(stats.bytes)}</b></span></div>
      ${bars(stats.buckets.map((b) => ({ ok: b.ok, err: b.err })))}${ticks(stats.from, stats.step, stats.buckets.length)}`;
  const tiles = hosts.length ? hosts.map((h) => { const s = hs(h.id); const [cls, lbl] = healthLabel(h, s); const per = stats && stats.perHost ? stats.perHost.find((x) => x.id === h.id) : null;
    return `<a class="tile" href="#/host/${h.id}"><div class="between"><span class="pill-status" style="color:var(--soft)"><span class="dot ${cls}"></span>${lbl}</span><span class="fill"></span>${h.type === 'proxy' ? certTag(h, s) : `<span class="tag t-${h.type}">${h.type === 'dead' ? '404' : 'Redirect'}</span>`}</div>
      <div><div class="dname">${esc(hostName(h))}</div><div class="mono muted" style="margin-top:2px;overflow-wrap:anywhere">${h.type === 'proxy' ? '→ ' + esc(h.upstreams[0] || '') : destLabel(h)}</div></div>
      <div class="muted" style="font-size:12.5px">${per ? fmtN(per.req) + ' req · 24h' : ''}</div></a>`; }).join('')
    : `<div class="card empty">No hosts yet. <a href="#/host/new">Add your first host</a></div>`;
  const certCounts = { valid: 0, expiring: 0, failed: 0, pending: 0, http: 0 };
  hosts.filter((h) => h.enabled).forEach((h) => { const s = hs(h.id).cert; if (s in certCounts) certCounts[s]++; });
  const events = (S.status.events || []).slice(-8).reverse();
  return shell('overview', `<p class="lead">A quick look at Caddy, your hosts, traffic and anything that needs fixing.</p>
    <section class="hero" aria-label="Status"><div style="flex:1 1 360px" class="stack s8">
      <span class="pill-status" style="color:#D6D3D1"><span class="dot ${c.running ? 'up' : 'down'}"></span>Caddy is ${c.running ? 'running' : 'stopped'}</span>
      <h1>${proxies.length ? `${healthy} of ${proxies.length} host${proxies.length > 1 ? 's' : ''} healthy` : hosts.length ? `${hosts.filter((h) => h.enabled).length} host${hosts.length > 1 ? 's' : ''} active` : 'Welcome'}</h1>
      <span class="sub">${c.running ? 'Started ' + ago(c.since) : ''}${S.state.manual ? ' · uploaded Caddyfile' : ''}</span></div>
      <div class="between"><a class="btn primary" href="#/host/new">${I.plus}Add host</a><button class="btn" data-a="restart-caddy">Restart Caddy</button></div></section>
    <div class="row"><div class="grow stack">
      <section class="card stack s8" aria-label="Needs attention"><h2>Needs attention</h2>${attHTML}</section>
      <section class="card" aria-label="Traffic">${traffic}</section>
      <section class="stack s8"><div class="between"><h2 class="fill">Hosts</h2><a href="#/hosts" style="font-size:13px;font-weight:600">Manage hosts</a></div><div class="tiles">${tiles}</div></section>
    </div>
    <aside class="side stack">
      <section class="card stack s8"><h2>Certificates</h2>
        ${[['valid', 'Valid', 'up'], ['expiring', 'Expiring soon', 'slow'], ['failed', 'Failed', 'down'], ['pending', 'Waiting to be issued', 'na'], ['http', 'HTTP only', 'na']].filter(([k]) => certCounts[k] || k === 'valid').map(([k, l, d]) => `<div class="between"><span class="dot ${d}"></span><span class="fill">${l}</span><b>${certCounts[k]}</b></div>`).join('')}
        <p class="hint">Caddy issues and renews certificates automatically.</p></section>
      <section class="card stack s8"><h2>Recent activity</h2>${events.length ? events.map((e) => `<div class="ev"><span class="dot ${e.level === 'error' ? 'down' : e.level === 'warn' ? 'slow' : 'up'}"></span><div style="min-width:0"><div style="word-break:break-word">${esc(e.msg)}${e.identifier ? ' · ' + esc(e.identifier) : ''}</div>${e.error ? `<div class="muted mono" style="font-size:12px;word-break:break-word">${esc(e.error.slice(0, 220))}</div>` : ''}<div class="muted" style="font-size:12px">${ago(e.ts)}</div></div></div>`).join('') : '<p class="hint">Nothing yet.</p>'}
        <a href="#/logs" style="font-size:13px;font-weight:600">Open logs</a></section>
      <section class="card stack s8"><h2>Caddyfile</h2><p class="hint">Backed up automatically before every change.</p>
        <div class="between"><a class="btn sm" href="/api/caddyfile" download>${I.down}Download</a><a class="btn sm" href="#/hosts?view=file">View</a></div>
        ${Array.isArray(backups) ? backups.slice(0, 5).map((b) => `<div class="between" style="font-size:13px;flex-wrap:nowrap"><span class="fill" style="min-width:0">${esc(new Date(b.ts * 1000).toLocaleString())}<br><span class="muted">${esc(b.label)}</span></span><button class="btn sm ghost" data-a="restore" data-id="${esc(b.id)}" style="color:var(--accent)">Restore</button></div>`).join('') || '<p class="hint">No backups yet.</p>' : ''}
      </section>
    </aside></div>`);
}

// ---------- pages: hosts ----------
function pageHosts() {
  const q = new URLSearchParams(location.hash.split('?')[1] || '');
  P.data.view = P.data.view || (q.get('view') === 'file' ? 'file' : 'list');
  P.data.filter = P.data.filter || 'all';
  P.data.q = P.data.q || '';
  const hosts = S.state.hosts;
  const count = (t) => hosts.filter((h) => t === 'all' || h.type === t).length;
  const view = P.data.view;
  let body;
  if (view === 'file') {
    const lines = S.caddyfile.split('\n');
    body = `<div class="between"><span class="fill muted">${S.state.manual ? 'This is the Caddyfile you uploaded.' : 'Generated from your hosts. Upload a file to run your own Caddyfile instead.'}</span>
      <a class="btn" href="/api/caddyfile" download>${I.down}Download</a>
      <label class="btn">${I.up}Upload Caddyfile<input type="file" id="cfUpload" class="sr"></label></div>
      <div class="codefile"><pre class="ln" aria-hidden="true">${lines.map((_, i) => i + 1).join('\n')}</pre><pre>${esc(S.caddyfile)}</pre></div>`;
  } else {
    body = `<div class="between"><div class="chips fill" role="group" aria-label="Filter by type">${[['all', 'All'], ['proxy', 'Proxy'], ['redirect', 'Redirect'], ['dead', '404']].map(([k, l]) => `<button class="chip ${P.data.filter === k ? 'on' : ''}" data-a="hfilter" data-k="${k}" aria-pressed="${P.data.filter === k}">${l}<span>${count(k)}</span></button>`).join('')}</div>
      <label class="sr" for="hq">Search hosts</label><input id="hq" class="in" type="search" placeholder="Search domains or upstreams" value="${esc(P.data.q)}" style="max-width:300px"></div>
      <div class="card flush"><div class="tbl-wrap"><table style="min-width:860px"><thead><tr><th>Source</th><th>Destination</th><th>SSL</th><th>Access</th><th>Status</th><th>On</th><th><span class="sr">Actions</span></th></tr></thead><tbody id="hostsBody">${hostRows()}</tbody></table></div></div>
      <p class="hint">Turning a host off removes it from the Caddyfile but keeps its settings.</p>`;
  }
  onAfter(() => {
    const qi = document.getElementById('hq');
    if (qi) qi.oninput = () => { P.data.q = qi.value; document.getElementById('hostsBody').innerHTML = hostRows(); };
    const up = document.getElementById('cfUpload');
    if (up) up.onchange = async () => { const f = up.files[0]; if (!f) return; const text = await f.text(); review({ caddyfile: text }, 'Upload Caddyfile', () => { P.data.view = 'file'; rerender(); }); };
  });
  return shell('hosts', `<div class="between"><div class="fill"><h1>Hosts</h1><p class="lead">Every domain Caddy serves: where it forwards, its certificate, who can open it and whether it’s up.</p></div>
    <div class="seg" role="group" aria-label="View"><button class="${view === 'list' ? 'on' : ''}" data-a="hview" data-k="list">List</button><button class="${view === 'file' ? 'on' : ''}" data-a="hview" data-k="file">Caddyfile</button></div>
    <a class="btn dark" href="#/host/new">${I.plus}Add host</a></div>${body}`);
}
function hostRows() {
  const ql = P.data.q.trim().toLowerCase();
  const rows = S.state.hosts.filter((h) => (P.data.filter === 'all' || h.type === P.data.filter) && (!ql || (h.domains.join(' ') + ' ' + h.upstreams.join(' ') + ' ' + h.redirectTo).toLowerCase().includes(ql)));
  if (!rows.length) return `<tr><td colspan="7" class="empty">${S.state.hosts.length ? 'No hosts match.' : 'No hosts yet — <a href="#/host/new">add your first host</a>.'}</td></tr>`;
  return rows.map((h) => { const s = hs(h.id); const [cls, lbl] = healthLabel(h, s);
    return `<tr class="hover" style="${h.enabled ? '' : 'opacity:.6'}">
      <td><div class="between" style="flex-wrap:nowrap">${typeIco(h.type)}<div style="min-width:0"><a href="#/host/${h.id}" style="color:var(--ink);font-weight:600;text-decoration:none;white-space:nowrap">${esc(hostName(h))}</a><div class="muted" style="font-size:12.5px">${{ proxy: 'Proxy', redirect: 'Redirect', dead: '404 host' }[h.type]}${h.domains.length > 1 ? ' · +' + (h.domains.length - 1) + ' domain' + (h.domains.length > 2 ? 's' : '') : ''}</div></div></div></td>
      <td class="mono" style="color:var(--soft);overflow-wrap:anywhere;max-width:280px">${destLabel(h)}</td>
      <td>${h.enabled ? certTag(h, s) : '<span class="tag t-muted">—</span>'}</td>
      <td style="font-size:13px;color:var(--soft)">${accessLabel(h)}</td>
      <td><span class="pill-status" style="color:var(--soft);white-space:nowrap"><span class="dot ${cls}"></span>${lbl}</span></td>
      <td><label class="chk" style="padding:0;align-items:center"><span class="sr">Enable ${esc(hostName(h))}</span><input type="checkbox" ${h.enabled ? 'checked' : ''} data-a="toggle-host" data-id="${h.id}"></label></td>
      <td style="text-align:right;white-space:nowrap"><a class="btn sm ghost" href="#/host/${h.id}/log">Log</a><a class="btn sm ghost" href="#/host/${h.id}" style="color:var(--accent)">Edit</a></td></tr>`; }).join('');
}

// ---------- pages: editor ----------
function newHost() {
  return { id: rid(), type: 'proxy', enabled: true, domains: [], upstreams: [''], lb: 'round_robin', healthPath: '', redirectTo: 'https://', redirectCode: 301, keepPath: true, deadCode: '404', deadMsg: 'Nothing here.', paths: [], access: { mode: 'none', listId: '', users: [], rules: [], satisfy: 'any' }, proto: 'https', cert: 'auto', forceHttps: true, hsts: false, exploits: true, gzip: true, cache: false, maintenance: false, headers: [], raw: '' };
}
function edModel(id) {
  if (P.data.ed) return P.data.ed;
  let h;
  if (PROPOSAL && ((id === 'new' && !PROPOSAL.id) || PROPOSAL.id === id)) { h = Object.assign(newHost(), clone(PROPOSAL)); if (!h.id) h.id = rid(); PROPOSAL = null; P.data.fromAI = true; }
  else if (id === 'new') h = newHost();
  else { const x = S.state.hosts.find((x) => x.id === id); if (!x) return null; h = Object.assign(newHost(), clone(x)); }
  if (!h.upstreams.length) h.upstreams = [''];
  h.domainsText = h.domains.join(', ');
  h.access.users = h.access.users.map((u) => ({ name: u.name, password: '', saved: !!u.name && id !== 'new' }));
  P.data.ed = h; P.data.tab = 'details'; P.data.isNew = !S.state.hosts.some((x) => x.id === h.id);
  return h;
}
function edHost() {
  const e = P.data.ed; const h = clone(e);
  h.domains = e.domainsText.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean);
  h.upstreams = e.upstreams.map((s) => s.trim()).filter(Boolean);
  h.access.users = e.access.users.filter((u) => u.name.trim()).map((u) => ({ name: u.name.trim(), password: u.password || undefined }));
  h.access.rules = e.access.rules.filter((r) => r.cidr.trim()).map((r) => ({ act: r.act, cidr: r.cidr.trim() }));
  h.paths = e.paths.filter((p) => p.path.trim() || p.to.trim());
  h.headers = e.headers.filter((x) => x.name.trim());
  h.redirectCode = Number(h.redirectCode);
  delete h.domainsText;
  return h;
}
const renderSnippet = debounce(async () => {
  const el = document.getElementById('snippet'); if (!el || !P.data.ed) return;
  try { const r = await post('/api/render', { host: edHost() }); P.data.snip = r.text; const el2 = document.getElementById('snippet'); if (el2) el2.textContent = r.text; } catch (e) { }
}, 250);

function pageEditor(id) {
  const e = edModel(id);
  if (!e) return shell('hosts', '<div class="card empty">This host doesn’t exist anymore. <a href="#/hosts">Back to hosts</a></div>');
  const t = P.data.tab;
  const proxy = e.type === 'proxy';
  const types = [['proxy', 'Proxy', 'Forward to a service'], ['redirect', 'Redirect', 'Send visitors to another URL'], ['dead', '404 host', 'Park a domain, return an error']];
  const tabs = proxy ? [['details', 'Details'], ['paths', 'Paths', e.paths.length], ['access', 'Access', e.access.mode === 'none' ? 0 : e.access.mode === 'list' ? 1 : e.access.users.length + e.access.rules.length], ['ssl', 'SSL'], ['advanced', 'Advanced']] : [['details', 'Details'], ['ssl', 'SSL'], ['advanced', 'Advanced']];
  const tab = tabs.some((x) => x[0] === t) ? t : 'details';
  const opt = (k, label, hint, locked) => `<label class="chk ${locked ? 'dis' : ''}"><input type="checkbox" ${locked || e[k] ? 'checked' : ''} ${locked ? 'disabled' : `data-f="${k}"`}><span>${label}<small>${hint}</small></span></label>`;
  const rm = (a, i, l) => `<button type="button" class="x" data-a="${a}" data-i="${i}" aria-label="${l}">${I.x}</button>`;
  let panel = '';
  if (tab === 'details') {
    panel = `<label class="f">Domain names<input class="in" data-f="domainsText" value="${esc(e.domainsText)}" placeholder="app.example.com, www.app.example.com" ${P.data.isNew ? 'autofocus' : ''}>
      <span class="hint">Separate several with commas. Use <span class="mono">*.example.com</span> for a wildcard (needs the DNS challenge under SSL).</span></label>`;
    if (proxy) panel += `<div class="stack s8"><span class="f">Forward to</span>${e.upstreams.map((u, i) => `<div class="line"><label class="sr" for="up${i}">Upstream</label><input id="up${i}" class="in mono" data-f="upstreams.${i}" value="${esc(u)}" placeholder="host.docker.internal:3000">${e.upstreams.length > 1 ? rm('up-del', i, 'Remove upstream') : ''}</div>`).join('')}
      <button type="button" class="add" data-a="up-add">+ Add another upstream (load balance)</button>
      ${e.upstreams.length > 1 ? `<div class="line note"><label class="f" style="flex:1 1 200px">Balancing<select class="in" data-f="lb">${[['round_robin', 'Round robin'], ['least_conn', 'Fewest connections'], ['ip_hash', 'Same visitor → same server'], ['first', 'First available (failover)']].map(([v, l]) => `<option value="${v}" ${e.lb === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
        <label class="f" style="flex:1 1 200px">Health check path<input class="in mono" data-f="healthPath" value="${esc(e.healthPath)}" placeholder="/health (optional)"></label></div>` : ''}
      <p class="hint">Just host:port — scheme, headers and WebSockets are handled automatically. Services on the Docker host: <span class="mono">host.docker.internal:PORT</span>.</p></div>`;
    if (e.type === 'redirect') panel += `<label class="f">Redirect to<input class="in mono" data-f="redirectTo" value="${esc(e.redirectTo)}" placeholder="https://example.com"></label>
      <div class="line"><label class="f" style="flex:1 1 240px">Type<select class="in" data-f="redirectCode">${[[301, '301 · Permanent'], [302, '302 · Temporary'], [307, '307 · Temporary, keep method'], [308, '308 · Permanent, keep method']].map(([v, l]) => `<option value="${v}" ${Number(e.redirectCode) === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
      <label class="chk" style="flex:1 1 240px"><input type="checkbox" data-f="keepPath" ${e.keepPath ? 'checked' : ''}><span>Keep path and query<small>/page?x=1 stays on the new URL</small></span></label></div>`;
    if (e.type === 'dead') panel += `<p class="note">Park a domain or catch stray subdomains. Nothing is forwarded.</p>
      <label class="f">Respond with<select class="in" data-f="deadCode">${[['404', '404 · Not found'], ['410', '410 · Gone for good'], ['abort', 'Close the connection (no response)']].map(([v, l]) => `<option value="${v}" ${e.deadCode === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
      ${e.deadCode !== 'abort' ? `<label class="f">Message<input class="in" data-f="deadMsg" value="${esc(e.deadMsg)}"></label>` : ''}`;
    if (proxy) panel += `<fieldset style="border:0;margin:0;padding:0"><legend class="f" style="margin-bottom:2px">Options</legend><div class="opts">
      ${opt('exploits', 'Block common exploits', 'Stops .env, .git, wp-login probes')}${opt('gzip', 'Compression', 'zstd + gzip')}${opt('cache', 'Cache static assets', 'CSS, JS, images for 7 days')}${opt('ws', 'WebSockets', 'Always on', true)}</div></fieldset>`;
  }
  if (tab === 'paths') panel = `<div><h2>Path routing</h2><p class="hint" style="margin-top:4px">Send parts of this site to other services. Everything else goes to the main upstream.</p></div>
    <div class="stack s8">${e.paths.map((p, i) => `<div class="line note"><label class="sr" for="pp${i}">Path</label><input id="pp${i}" class="in mono" data-f="paths.${i}.path" value="${esc(p.path)}" placeholder="/api" style="flex:1 1 120px"><span aria-hidden="true">${I.proxy}</span><label class="sr" for="pt${i}">Forward to</label><input id="pt${i}" class="in mono" data-f="paths.${i}.to" value="${esc(p.to)}" placeholder="host.docker.internal:4000" style="flex:2 1 200px">
      <label class="chk" style="padding:0;align-items:center"><input type="checkbox" data-f="paths.${i}.strip" ${p.strip ? 'checked' : ''}>Strip path</label>${rm('path-del', i, 'Remove path')}</div>`).join('')}
      <div class="note mono" style="background:transparent;border:1px dashed var(--line2)">everything else → ${esc(e.upstreams.filter(Boolean).join(', ') || '(main upstream)')}</div></div>
    <button type="button" class="add" data-a="path-add">+ Add path</button>
    <p class="hint"><b>Strip path</b> removes the prefix: <span class="mono">/api/users</span> arrives as <span class="mono">/users</span>.</p>`;
  if (tab === 'access') {
    const lists = S.state.accessLists;
    panel = `<div class="choice">${[['none', 'Public', 'Anyone can open it'], ['list', 'Use an access list', 'Shared logins and IP rules'], ['custom', 'Custom for this host', 'Set logins and IPs here']].map(([k, l, d]) => `<button type="button" class="${e.access.mode === k ? 'on' : ''}" data-a="acc-mode" data-k="${k}" aria-pressed="${e.access.mode === k}"><b>${l}</b><span>${d}</span></button>`).join('')}</div>`;
    if (e.access.mode === 'list') panel += lists.length ? `<label class="f" style="max-width:360px">Access list<select class="in" data-f="access.listId"><option value="">Choose…</option>${lists.map((l) => `<option value="${l.id}" ${e.access.listId === l.id ? 'selected' : ''}>${esc(l.name)}</option>`).join('')}</select></label><a href="#/access" style="font-size:13px;font-weight:600">Edit access lists</a>` : `<p class="note">No access lists yet. <a href="#/access">Create one</a> first.</p>`;
    if (e.access.mode === 'custom') panel += usersRulesEditor(e.access, 'access.');
  }
  if (tab === 'ssl') {
    const https = e.proto === 'https';
    const s = hs(e.id);
    panel = `<div class="choice">${[['https', 'HTTPS', 'Encrypted, certificate managed for you'], ['http', 'HTTP only', 'Plain routing, no certificate']].map(([k, l, d]) => `<button type="button" class="${e.proto === k ? 'on' : ''}" data-a="proto" data-k="${k}" aria-pressed="${e.proto === k}"><b>${l}</b><span>${d}</span></button>`).join('')}</div>`;
    if (https) {
      if (!P.data.isNew) panel += `<div class="note ${s.cert === 'valid' ? 'ok' : s.cert === 'failed' ? 'bad' : s.cert === 'expiring' ? 'warn' : ''} between"><span class="fill">${s.cert === 'valid' ? `Certificate valid · ${issuerName(s.certIssuer)}${s.certDays >= 1 ? ' · ' + s.certDays + ' days left' : ' · renewed automatically'}` : s.cert === 'expiring' ? `Expires in ${s.certDays} days` : s.cert === 'failed' ? 'Certificate failed: ' + esc(s.certError) : 'No certificate yet — Caddy gets one on the first request after applying.'}</span>${s.cert === 'failed' && aiOn() ? `<button class="btn sm ai" data-a="explain" data-err="${esc(s.certError)}" data-h="${e.id}">${I.sparkle}Explain</button>` : ''}</div>`;
      panel += `<div class="stack s8"><span class="f">Certificate</span><div class="choice">${[['auto', 'Automatic', 'Let’s Encrypt / ZeroSSL'], ['dns', 'DNS challenge', 'Wildcards, hosts behind a firewall'], ['internal', 'Self-signed', 'Caddy’s local CA, for LAN'], ['custom', 'Upload my own', '.crt and .key files']].map(([k, l, d]) => `<button type="button" class="${e.cert === k ? 'on' : ''}" data-a="cert" data-k="${k}" aria-pressed="${e.cert === k}"><b>${l}</b><span>${d}</span></button>`).join('')}</div></div>`;
      if (e.cert === 'dns') panel += `<p class="note ${S.settings.hasDnsToken ? 'ai' : 'warn'}">Uses Cloudflare. ${S.settings.hasDnsToken ? 'Your API token is saved in Settings.' : 'Add your Cloudflare API token in <a href="#/settings">Settings</a> first.'}</p>`;
      if (e.cert === 'custom') panel += `<form id="certForm" class="line note"><label class="f" style="flex:1 1 200px">Certificate (.crt / .pem)<input type="file" name="cert" required></label><label class="f" style="flex:1 1 200px">Private key (.key)<input type="file" name="key" required></label><button class="btn sm" type="submit">Upload files</button></form><p class="hint">Upload the files first, then apply.</p>`;
      if (proxy || true) panel += `<div class="opts"><label class="chk"><input type="checkbox" data-f="forceHttps" ${e.forceHttps ? 'checked' : ''}><span>Force HTTPS<small>Redirect http:// to https://</small></span></label><label class="chk"><input type="checkbox" data-f="hsts" ${e.hsts ? 'checked' : ''}><span>HSTS<small>Browsers always use HTTPS</small></span></label></div>
        <p class="hint">HTTP/2 and HTTP/3 are on automatically.</p>`;
    } else panel += '<p class="note warn">Traffic to this host isn’t encrypted. Fine on a home network — avoid it for logins.</p>';
  }
  if (tab === 'advanced') {
    if (proxy) panel += `<label class="chk note ${e.maintenance ? 'warn' : ''}" style="padding:12px 14px"><input type="checkbox" data-f="maintenance" ${e.maintenance ? 'checked' : ''}><span><b>Maintenance mode</b><small>Show a “Back soon” page (503) instead of forwarding.</small></span></label>
      <div class="stack s8"><h2>Custom headers</h2>${e.headers.map((h, i) => `<div class="line"><label class="sr" for="hk${i}">Direction</label><select id="hk${i}" class="in" data-f="headers.${i}.kind" style="flex:0 0 150px"><option value="res" ${h.kind === 'res' ? 'selected' : ''}>To browser</option><option value="req" ${h.kind === 'req' ? 'selected' : ''}>To upstream</option></select>
        <label class="sr" for="hn${i}">Name</label><input id="hn${i}" class="in mono" data-f="headers.${i}.name" value="${esc(h.name)}" placeholder="X-Frame-Options"><label class="sr" for="hv${i}">Value</label><input id="hv${i}" class="in mono" data-f="headers.${i}.value" value="${esc(h.value)}" placeholder="SAMEORIGIN" style="flex:2 1 200px">${rm('hdr-del', i, 'Remove header')}</div>`).join('')}
        <button type="button" class="add" data-a="hdr-add">+ Add header</button></div>`;
    panel += `<label class="f">Custom Caddy directives<textarea class="in mono" rows="5" data-f="raw" placeholder="request_body {\n\tmax_size 50MB\n}">${esc(e.raw)}</textarea><span class="hint">Added inside this host’s block. Checked by Caddy before it’s applied.</span></label>`;
  }
  const s = hs(e.id); const [cls, lbl] = healthLabel(e, s);
  onAfter(() => {
    renderSnippet();
    const cf = document.getElementById('certForm');
    if (cf) cf.onsubmit = async (ev) => { ev.preventDefault(); try { await api('/api/certs/' + e.id, { method: 'POST', form: new FormData(cf) }); toast('Certificate files uploaded'); } catch (err) { toast(err.message, true); } };
    const f = $app.querySelector('[autofocus]'); if (f && P.data.isNew && !P.data.focused) { f.focus(); P.data.focused = true; }
  });
  return shell('hosts', `
    <div class="between"><div style="flex:1 1 400px;min-width:0"><a href="#/hosts" class="between muted" style="text-decoration:none;font-size:13px;font-weight:600;gap:4px;display:inline-flex">${I.back}All hosts</a>
      <h1>${P.data.isNew ? 'New host' : esc(hostName(e))}</h1><p class="lead">Choose what this host does, then set its domains, routing, access and HTTPS. Nothing changes until you review and apply.</p>${P.data.fromAI ? '<p class="note ai" style="margin-top:8px">Filled in by the assistant — check it, then review &amp; apply.</p>' : ''}</div>
      ${!P.data.isNew ? `<div class="seg"><a class="on" href="#/host/${e.id}">Settings</a><a href="#/host/${e.id}/log">Log</a></div>` : ''}
      <label class="chk" style="padding:0 14px;background:var(--card);border-radius:12px;align-items:center"><input type="checkbox" data-f="enabled" ${e.enabled ? 'checked' : ''}>Enabled</label>
      <button class="btn primary" data-a="save-host">Review &amp; apply</button></div>
    <section class="choice" aria-label="Host type" style="grid-template-columns:repeat(auto-fit,minmax(220px,1fr))">${types.map(([k, l, d]) => `<button type="button" class="${e.type === k ? 'on' : ''}" data-a="htype" data-k="${k}" aria-pressed="${e.type === k}" style="display:flex;gap:12px;align-items:flex-start;background:${e.type === k ? 'var(--card)' : 'transparent'}">${typeIco(k)}<span style="margin:0"><b>${l}</b><span>${d}</span></span></button>`).join('')}</section>
    <div class="row"><main class="grow card flush"><div class="tabs" role="tablist">${tabs.map(([k, l, n]) => `<button role="tab" class="${tab === k ? 'on' : ''}" aria-selected="${tab === k}" data-a="tab" data-k="${k}">${l}${n ? `<span class="tag t-muted">${n}</span>` : ''}</button>`).join('')}</div>
      <div class="stack" style="padding:22px">${panel}</div></main>
      <aside class="side stack">
        <section class="code" aria-label="Caddyfile preview" style="padding:0"><div class="between" style="padding:16px 18px 0;color:#D6D3D1;font-family:Inter,sans-serif;font-size:13px;font-weight:600"><span class="fill">This host in the Caddyfile</span><span style="font-weight:400;color:#A8A29E">live</span></div><pre id="snippet" style="margin:0;padding:12px 18px 18px;white-space:pre;overflow-x:auto;font:inherit">${esc(P.data.snip || '…')}</pre></section>
        ${!P.data.isNew ? `<section class="card stack s8" style="font-size:13.5px"><div class="between"><span class="dot ${cls}"></span><span class="fill">Upstream check</span><b>${lbl}</b></div>${s.error ? `<div class="muted mono" style="font-size:12px">${esc(s.error)}</div>` : ''}<div class="between"><button class="btn sm" data-a="check-now">Check now</button><a href="#/host/${e.id}/log" style="font-size:13px;font-weight:600">Open this host’s log</a></div></section>
        <button class="btn danger" data-a="delete-host" style="align-self:flex-start">Delete host</button>` : ''}
      </aside></div>`);
}
function usersRulesEditor(a, prefix) {
  return `<div class="stack s8"><h2>Logins</h2>${a.users.map((u, i) => `<div class="line"><label class="sr" for="un${i}">Username</label><input id="un${i}" class="in" data-f="${prefix}users.${i}.name" value="${esc(u.name)}" placeholder="username" autocomplete="off"><label class="sr" for="pw${i}">Password</label><input id="pw${i}" class="in" type="password" data-f="${prefix}users.${i}.password" value="${esc(u.password)}" placeholder="${u.saved ? '•••••••• saved — type to change' : 'Password'}" autocomplete="new-password" style="flex:1 1 200px"><button type="button" class="x" data-a="user-del" data-i="${i}" aria-label="Remove login">${I.x}</button></div>`).join('')}
    <button type="button" class="add" data-a="user-add">+ Add login</button><p class="hint">Passwords are hashed with bcrypt before they’re saved.</p></div>
    <div class="stack s8"><h2>IP allow / deny</h2>${a.rules.map((r, i) => `<div class="line"><label class="sr" for="ra${i}">Rule</label><select id="ra${i}" class="in" data-f="${prefix}rules.${i}.act" style="flex:0 0 120px"><option value="allow" ${r.act === 'allow' ? 'selected' : ''}>Allow</option><option value="deny" ${r.act === 'deny' ? 'selected' : ''}>Deny</option></select><label class="sr" for="rc${i}">IP or range</label><input id="rc${i}" class="in mono" data-f="${prefix}rules.${i}.cidr" value="${esc(r.cidr)}" placeholder="192.168.1.0/24" style="flex:1 1 220px"><button type="button" class="x" data-a="rule-del" data-i="${i}" aria-label="Remove rule">${I.x}</button></div>`).join('')}
      <div class="line"><button type="button" class="add" data-a="rule-add">+ Add rule</button><span class="hint">Quick add:</span><button type="button" class="chip" style="background:var(--chip)" data-a="rule-quick" data-v="private_ranges">Local network</button><button type="button" class="chip" style="background:var(--chip)" data-a="rule-quick" data-v="100.64.0.0/10">Tailscale</button></div>
      <p class="hint">Once you add an allow rule, every other IP is refused.</p></div>
    <label class="f" style="max-width:460px">When both are set, require<select class="in" data-f="${prefix}satisfy"><option value="any" ${a.satisfy === 'any' ? 'selected' : ''}>Either — an allowed IP skips the login</option><option value="all" ${a.satisfy === 'all' ? 'selected' : ''}>Both — allowed IP and a login</option></select></label>`;
}

// ---------- pages: host log ----------
function pageHostLog(id) {
  const h = S.state.hosts.find((x) => x.id === id);
  if (!h) return shell('hosts', '<div class="card empty">This host doesn’t exist. <a href="#/hosts">Back to hosts</a></div>');
  return logPage(h);
}
function pageLogs() { return logPage(null); }
function logPage(h) {
  const d = P.data;
  d.range = d.range || '24h'; d.status = d.status || 'all'; d.q = d.q || ''; d.hostSel = h ? h.id : (d.hostSel || '');
  if (d.live === undefined) d.live = !!h;
  const hid = d.hostSel;
  const qs = `range=${d.range}&host=${encodeURIComponent(hid)}`;
  const stats = need('stats', () => api('/api/stats?' + qs));
  const logs = need('logs', () => api(`/api/logs?${qs}&status=${d.status}&q=${encodeURIComponent(d.q)}&limit=400`));
  if (d.live && !pollTimer) pollTimer = setInterval(() => { const ae = document.activeElement; if ($modal.innerHTML || (ae && ['INPUT', 'TEXTAREA', 'SELECT'].includes(ae.tagName))) return; delete P.data.logs; delete P.data.stats; need('stats', () => api('/api/stats?' + `range=${P.data.range}&host=${encodeURIComponent(P.data.hostSel)}`)); need('logs', () => api(`/api/logs?range=${P.data.range}&host=${encodeURIComponent(P.data.hostSel)}&status=${P.data.status}&q=${encodeURIComponent(P.data.q)}&limit=400`)); }, 5000);
  if (!d.live && pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  const s = h ? hs(h.id) : null;
  const ranges = `<div class="seg" role="group" aria-label="Range">${[['1h', '1 h'], ['24h', '24 h'], ['7d', '7 d']].map(([k, l]) => `<button class="${d.range === k ? 'on' : ''}" data-a="lrange" data-k="${k}">${l}</button>`).join('')}</div>`;
  const statTiles = stats && !stats.error ? `<section class="grid" style="grid-template-columns:repeat(auto-fit,minmax(150px,1fr))">${[['Requests', fmtN(stats.total)], ['Errors (5xx)', fmtN(stats.err5), stats.err5 ? 'var(--bad-ink)' : ''], ['Client errors (4xx)', fmtN(stats.err4)], ['Median response', stats.medianMs + ' ms'], ['Data served', fmtB(stats.bytes)]].map(([l, v, c]) => `<div class="stat"><div class="l">${l}</div><div class="v" style="${c ? 'color:' + c : ''}">${v}</div></div>`).join('')}</section>` : `<div class="card muted">${stats && stats.error ? esc(stats.error) : 'Loading…'}</div>`;
  const chart = stats && !stats.error ? `<section class="card"><div class="between" style="margin-bottom:12px"><h2 class="fill" style="font-size:15px;font-family:Inter,sans-serif">Requests</h2><span class="pill-status"><span class="dot" style="background:#FDBA74"></span>OK</span><span class="pill-status"><span class="dot down"></span>5xx</span></div>${bars(stats.buckets, 120)}${ticks(stats.from, stats.step, stats.buckets.length)}</section>` : '';
  const codes = stats && !stats.error && !h ? `<section class="card stack s8"><h2>Responses</h2>${(() => { const t = Math.max(1, stats.total); const c = stats.codes; const col = { 2: '#22C55E', 3: '#60A5FA', 4: '#F59E0B', 5: '#DC2626' }; return `<div class="split">${[2, 3, 4, 5].map((k) => c[k] ? `<span style="width:${(c[k] / t) * 100}%;background:${col[k]}"></span>` : '').join('')}</div>` + [[2, 'OK'], [3, 'Redirect'], [4, 'Client error'], [5, 'Server error']].map(([k, l]) => `<div class="between" style="font-size:14px"><span class="dot" style="background:${col[k]};border-radius:3px"></span><span class="fill">${k}xx ${l}</span><b>${((c[k] / t) * 100).toFixed(1)}%</b></div>`).join(''); })()}</section>` : '';
  const perHost = stats && !stats.error && !h ? `<section class="card flush"><h2 style="padding:16px 20px">By host</h2><div class="tbl-wrap"><table style="min-width:560px"><thead><tr><th>Host</th><th style="text-align:right">Requests</th><th style="text-align:right">5xx</th><th style="text-align:right">Avg response</th><th style="text-align:right">Data</th></tr></thead><tbody>${stats.perHost.length ? stats.perHost.map((p) => `<tr class="hover"><td><a href="#/host/${p.id}/log" style="color:var(--ink);font-weight:600">${esc(p.domain)}</a></td><td class="mono" style="text-align:right">${fmtN(p.req)}</td><td class="mono" style="text-align:right;${p.err ? 'color:var(--bad-ink)' : ''}">${fmtN(p.err)}</td><td class="mono" style="text-align:right">${p.avgMs} ms</td><td class="mono" style="text-align:right">${fmtB(p.bytes)}</td></tr>`).join('') : '<tr><td colspan="5" class="empty">No hosts yet.</td></tr>'}</tbody></table></div></section>` : '';
  const logTable = `<section class="card flush"><div class="between" style="padding:12px 16px;border-bottom:1px solid var(--line)"><h2 class="fill" style="font-size:16px">Requests</h2>
      <div class="seg" role="group" aria-label="Status">${[['all', 'All'], ['2', '2xx'], ['3', '3xx'], ['4', '4xx'], ['5', '5xx']].map(([k, l]) => `<button class="${d.status === k ? 'on' : ''}" data-a="lstatus" data-k="${k}" style="min-height:34px;padding:0 10px;font-size:13px">${l}</button>`).join('')}</div>
      <label class="sr" for="lq">Filter by path or IP</label><input id="lq" class="in" type="search" value="${esc(d.q)}" placeholder="Path or IP" style="width:170px;height:40px">
      <label class="chk" style="padding:0;align-items:center;min-height:40px;font-weight:600;font-size:13px"><input type="checkbox" data-a="llive" ${d.live ? 'checked' : ''}>Live</label>
      <a class="btn sm ghost" style="color:var(--accent)" href="/api/logs?${qs}&status=${d.status}&q=${encodeURIComponent(d.q)}&limit=2000&download=1">Download</a></div>
    <div class="tbl-wrap"><table class="logtbl" style="min-width:${h ? 620 : 760}px"><thead><tr><th>Time</th><th>Status</th>${h ? '' : '<th>Host</th>'}<th>Request</th><th>Client</th><th style="text-align:right">Time</th></tr></thead><tbody>${logs && !logs.error ? (logs.lines.length ? logs.lines.map((l) => `<tr class="hover"><td style="color:var(--muted);white-space:nowrap">${fmtTime(l.ts)}</td><td class="code${String(l.status)[0]}" style="font-weight:600">${l.status}</td>${h ? '' : `<td style="white-space:nowrap">${esc(l.host)}</td>`}<td style="word-break:break-all">${esc(l.method)} ${esc(l.uri)}</td><td style="color:var(--muted);white-space:nowrap">${esc(l.ip)}</td><td style="text-align:right;color:var(--muted);white-space:nowrap">${l.ms} ms</td></tr>`).join('') : `<tr><td colspan="6" class="empty" style="font-family:Inter,sans-serif">No requests ${d.status !== 'all' || d.q ? 'match these filters' : 'in this period yet'}.</td></tr>`) : `<tr><td colspan="6" class="empty" style="font-family:Inter,sans-serif">${logs && logs.error ? esc(logs.error) : 'Loading…'}</td></tr>`}</tbody></table></div></section>`;
  onAfter(() => {
    const qi = document.getElementById('lq');
    if (qi) qi.onchange = () => { P.data.q = qi.value; delete P.data.logs; rerender(); };
    const hsel = document.getElementById('lhost');
    if (hsel) hsel.onchange = () => { P.data.hostSel = hsel.value; delete P.data.logs; delete P.data.stats; rerender(); };
  });
  if (h) {
    const [cls, lbl] = healthLabel(h, s);
    return shell('hosts', `<div class="between"><div style="flex:1 1 400px;min-width:0"><a href="#/hosts" class="muted" style="text-decoration:none;font-size:13px;font-weight:600;display:inline-flex;align-items:center;gap:4px">${I.back}All hosts</a>
      <h1>${esc(hostName(h))}</h1><p class="lead">Requests to this host: traffic, errors and response times, with an assistant to help you read them.</p><div class="between muted" style="font-size:13px;margin-top:4px"><span class="pill-status"><span class="dot ${cls}"></span>${lbl}</span><span class="mono">${destLabel(h)}</span></div></div>
      <div class="seg"><a href="#/host/${h.id}">Settings</a><a class="on" href="#/host/${h.id}/log">Log</a></div>${ranges}</div>
      ${statTiles}<div class="row"><div class="grow stack">${chart}${logTable}</div><aside class="side" style="max-width:460px">${aiPanel('host-' + h.id, h.id, d.range)}</aside></div>`);
  }
  return shell('logs', `<div class="between"><div class="fill"><h1>Logs &amp; stats</h1><p class="lead">Traffic and requests across all your hosts. Filter by host, status or time, and ask questions about what happened.</p></div><label class="sr" for="lhost">Host</label><select id="lhost" class="in" style="max-width:260px"><option value="">All hosts</option>${S.state.hosts.map((x) => `<option value="${x.id}" ${hid === x.id ? 'selected' : ''}>${esc(hostName(x))}</option>`).join('')}</select>${ranges}</div>
    ${statTiles}<div class="row"><div class="grow stack">${chart}</div><div class="side">${codes}</div></div>
    ${aiPanel('all-' + (hid || 'x'), hid, d.range)}${perHost}${logTable}`);
}

// ---------- pages: access lists ----------
function pageAccess() {
  const d = P.data;
  if (!d.lists) { d.lists = clone(S.state.accessLists).map((l) => ({ ...l, users: l.users.map((u) => ({ name: u.name, password: '', saved: true })) })); d.sel = 0; }
  const lists = d.lists;
  const cur = lists[d.sel];
  const usedBy = (l) => S.state.hosts.filter((h) => h.type === 'proxy' && h.access.mode === 'list' && h.access.listId === l.id);
  const dirty = JSON.stringify(lists.map((l) => ({ ...l, users: l.users.map((u) => ({ name: u.name, p: u.password })) }))) !== JSON.stringify(S.state.accessLists.map((l) => ({ ...l, users: l.users.map((u) => ({ name: u.name, p: '' })) })));
  onAfter(() => renderListSnippet());
  return shell('access', `<div class="between"><div class="fill"><h1>Access lists</h1><p class="lead">Logins and IP rules you set once and reuse on any host.</p></div><button class="btn dark" data-a="list-new">${I.plus}New access list</button></div>
    <div class="row"><aside class="side stack s8" style="max-width:360px">${lists.length ? lists.map((l, i) => `<button class="tile" data-a="list-sel" data-i="${i}" style="text-align:left;cursor:pointer;${i === d.sel ? 'border-color:var(--accent)' : 'background:transparent;border-color:transparent'}"><span class="dname">${esc(l.name || 'Untitled')}</span><span class="muted" style="font-size:12.5px">${l.users.length} login${l.users.length === 1 ? '' : 's'} · ${l.rules.length} IP rule${l.rules.length === 1 ? '' : 's'} · used by ${usedBy(l).length} host${usedBy(l).length === 1 ? '' : 's'}</span></button>`).join('') : '<div class="card empty">No access lists yet.</div>'}</aside>
    ${cur ? `<main class="grow row" style="align-items:flex-start"><section class="card stack" style="flex:1 1 420px;min-width:0">
      <label class="f">Name<input class="in" data-lf="name" value="${esc(cur.name)}" style="font-weight:600;font-size:16px"></label>
      ${usersRulesEditor(cur, '').replace(/data-f="/g, 'data-lf="')}
      <label class="chk"><input type="checkbox" data-lf="pass" ${cur.pass ? 'checked' : ''}><span>Pass the login on to the upstream<small>Sends the username as X-Remote-User</small></span></label>
      <div class="between" style="border-top:1px solid var(--line);padding-top:14px"><button class="btn primary" data-a="list-save" ${dirty ? '' : 'disabled'}>Review &amp; apply</button><span class="fill"></span><button class="btn danger" data-a="list-del">Delete list</button></div></section>
      <div class="stack" style="flex:1 1 300px;min-width:0"><section class="card stack s8"><h2>Used by</h2>${usedBy(cur).map((h) => `<a href="#/host/${h.id}" class="note" style="text-decoration:none;color:var(--ink);font-weight:600">${esc(hostName(h))}</a>`).join('') || '<p class="hint">Not used yet. Pick it in a host’s Access tab.</p>'}</section>
      <section class="code" style="padding:0"><div style="padding:16px 18px 0;color:#D6D3D1;font-family:Inter,sans-serif;font-size:13px;font-weight:600">In the Caddyfile (reusable snippet)</div><pre id="lsnippet" style="margin:0;padding:12px 18px 18px;white-space:pre;overflow-x:auto;font:inherit">${esc(P.data.lsnip || '…')}</pre></section></div></main>` : ''}</div>`);
}
const renderListSnippet = debounce(async () => {
  const d = P.data, el = document.getElementById('lsnippet'); if (!el || !d.lists || !d.lists[d.sel]) return;
  const l = d.lists[d.sel];
  try { const r = await post('/api/render', { list: { ...l, users: l.users.filter((u) => u.name).map((u) => ({ name: u.name })), rules: l.rules.filter((r) => r.cidr) } }); P.data.lsnip = r.text; const e2 = document.getElementById('lsnippet'); if (e2) e2.textContent = r.text; } catch (e) { }
}, 250);
function listsForSave() {
  return P.data.lists.map((l) => ({ id: l.id, name: l.name, pass: !!l.pass, satisfy: l.satisfy || 'any', users: l.users.filter((u) => u.name.trim()).map((u) => ({ name: u.name.trim(), password: u.password || undefined })), rules: l.rules.filter((r) => r.cidr.trim()).map((r) => ({ act: r.act, cidr: r.cidr.trim() })) }));
}

// ---------- pages: settings ----------
function pageSettings() {
  const d = P.data;
  if (!d.set) d.set = { email: S.settings.email || '', dnsToken: '', ai: { enabled: false, provider: 'anthropic', baseUrl: '', apiKey: '', model: '', maskIps: false, ...clone(S.settings.ai || {}), apiKey: '' } };
  const a = d.set.ai;
  const prov = [['anthropic', 'Anthropic', 'Claude models · API key'], ['openai', 'OpenAI', 'GPT models · API key'], ['compat', 'OpenAI-compatible', 'OpenRouter, LM Studio, vLLM…'], ['ollama', 'Ollama', 'Runs on your machine']];
  const needsKey = a.provider !== 'ollama';
  const needsUrl = a.provider === 'compat' || a.provider === 'ollama';
  const urlPh = { compat: 'https://openrouter.ai/api/v1', ollama: 'http://host.docker.internal:11434/v1' }[a.provider] || '';
  return shell('settings', `<div><h1>Settings</h1><p class="lead">Connect an AI assistant, set your certificate email and DNS token, and manage Caddy.</p></div>
    <div class="row"><div class="grow stack">
    <section class="card stack"><div class="between"><span style="color:var(--ai)">${I.sparkle}</span><div class="fill"><h2>AI assistant</h2><p class="hint">Explains errors, answers questions about your logs and drafts host changes. It only suggests — every change goes through Review.</p></div>
      <label class="chk" style="padding:0 14px;background:${a.enabled ? 'var(--ai-bg)' : 'var(--tint)'};border-radius:12px;align-items:center;font-weight:600"><input type="checkbox" data-sf="ai.enabled" ${a.enabled ? 'checked' : ''}>${a.enabled ? 'On' : 'Off'}</label></div>
      <div class="stack" style="${a.enabled ? '' : 'opacity:.5'}">
      <div class="choice ai">${prov.map(([k, l, dd]) => `<button type="button" class="${a.provider === k ? 'on' : ''}" data-a="prov" data-k="${k}" aria-pressed="${a.provider === k}"><b>${l}</b><span>${dd}</span></button>`).join('')}</div>
      <div class="grid" style="grid-template-columns:repeat(auto-fit,minmax(260px,1fr))">
        ${needsKey ? `<label class="f">API key<input class="in" type="password" data-sf="ai.apiKey" value="${esc(a.apiKey)}" placeholder="${S.settings.ai && S.settings.ai.hasKey ? '•••••••• saved — type to replace' : a.provider === 'compat' ? 'Optional for local servers' : 'Paste your key'}" autocomplete="off"><span class="hint">Stored on this server only.</span></label>` : ''}
        ${needsUrl ? `<label class="f">Base URL<input class="in mono" data-sf="ai.baseUrl" value="${esc(a.baseUrl)}" placeholder="${urlPh}"></label>` : ''}
        <label class="f">Model<input class="in mono" data-sf="ai.model" value="${esc(a.model)}" placeholder="Model ID"><span class="hint">Type the model ID your provider uses.</span></label>
      </div>
      <label class="chk"><input type="checkbox" data-sf="ai.maskIps" ${a.maskIps ? 'checked' : ''}><span>Mask IP addresses<small>Replaced with ip-1, ip-2 … before anything is sent. Password hashes and tokens are never sent.</small></span></label>
      <div class="between"><button class="btn" data-a="ai-test">Test connection</button><span id="aiTest" class="hint"></span></div></div>
    </section>
    <section class="card stack"><h2>Certificates &amp; DNS</h2>
      <label class="f">Email for certificate notices (optional)<input class="in" type="email" data-sf="email" value="${esc(d.set.email)}" placeholder="you@example.com"></label>
      <label class="f">Cloudflare API token — for wildcard certificates and the DNS challenge<input class="in" type="password" data-sf="dnsToken" value="${esc(d.set.dnsToken)}" placeholder="${S.settings.hasDnsToken ? '•••••••• saved — type to replace' : 'Token with Zone:DNS:Edit permission'}" autocomplete="off"></label>
      ${S.settings.hasDnsToken ? '<button class="btn sm ghost" data-a="clear-dns" style="align-self:flex-start;color:#B91C1C">Remove saved token</button>' : ''}
    </section>
    <div class="between"><button class="btn primary" data-a="settings-save">Save settings</button></div>
    </div>
    <aside class="side stack"><section class="card stack s8"><h2>Caddy</h2><p class="hint">${S.status.caddy.running ? 'Running.' : 'Stopped: ' + esc(S.status.caddy.error)}</p><button class="btn" data-a="restart-caddy" style="align-self:flex-start">Restart Caddy</button></section>
      <section class="card stack s8"><h2>Account</h2><p class="hint">Single admin. Change the password with ADMIN_PASSWORD in docker-compose and restart the container.</p><button class="btn" data-a="logout" style="align-self:flex-start">Sign out</button></section></aside></div>`);
}

// ---------- bindings ----------
function setPath(obj, path, val) {
  const ks = path.split('.'); let o = obj;
  for (let i = 0; i < ks.length - 1; i++) o = o[ks[i]];
  o[ks[ks.length - 1]] = val;
}
const valOf = (el) => el.type === 'checkbox' ? el.checked : el.value;

document.addEventListener('input', (ev) => {
  const el = ev.target;
  if (el.dataset.f && P.data.ed) {
    setPath(P.data.ed, el.dataset.f, valOf(el));
    if (['type', 'deadCode', 'access.listId'].includes(el.dataset.f)) rerender(); else renderSnippet();
  } else if (el.dataset.lf && P.data.lists) {
    setPath(P.data.lists[P.data.sel], el.dataset.lf, valOf(el));
    renderListSnippet();
    const b = $app.querySelector('[data-a="list-save"]'); if (b) b.disabled = false;
  } else if (el.dataset.sf && P.data.set) {
    setPath(P.data.set, el.dataset.sf, valOf(el));
    if (el.type === 'checkbox') rerender();
  }
});
document.addEventListener('change', (ev) => {
  const el = ev.target;
  if (el.tagName === 'SELECT' && el.dataset.f && P.data.ed) { setPath(P.data.ed, el.dataset.f, el.value); rerender(); }
});
document.addEventListener('submit', (ev) => {
  const f = ev.target.closest('[data-aiform]'); if (!f) return;
  ev.preventDefault();
  const inp = f.querySelector('input');
  askLogs(f.dataset.aiform, f.dataset.h, f.dataset.r, inp.value);
});
document.addEventListener('keydown', (ev) => {
  if ((ev.metaKey || ev.ctrlKey) && ev.key.toLowerCase() === 'k' && S.authed) { ev.preventDefault(); openAsk(); }
  if (ev.key === 'Escape' && $modal.innerHTML) closeModal();
});

const ACT = {
  'close-modal': () => closeModal(),
  'modal-bg': (el, ev) => { if (ev.target === el) closeModal(); },
  ask: () => openAsk(),
  'ask-sug': (el) => { document.getElementById('askIn').value = el.textContent; runAsk(el.textContent); },
  'open-proposal': () => { const p = PROPOSAL; closeModal(); P.key = ''; location.hash = p.id ? '#/host/' + p.id : '#/host/new'; if (location.hash === (p.id ? '#/host/' + p.id : '#/host/new')) render(); },
  explain: (el) => explain(el.dataset.err, el.dataset.h),
  'explain-review': () => { const r = REVIEW && REVIEW.res; const hid = REVIEW && REVIEW.payload && REVIEW.payload.hosts && P.data.ed ? P.data.ed.id : ''; explain((r && r.error) || 'Unknown error', hid); },
  apply: (el) => doApply(el),
  restore: async (el) => {
    openModal('Restore backup', '<span class="spin"></span>');
    try { const r = await post('/api/backups/restore', { id: el.dataset.id }); REVIEW = { payload: {}, label: r.label, after: null, res: r }; showReview(r); } catch (e) { setModalBody(`<div class="note bad">${esc(e.message)}</div>`); }
  },
  'restart-caddy': async (el) => { el.disabled = true; try { await post('/api/caddy/restart'); toast('Caddy restarted'); await refreshStatus(); } catch (e) { toast(e.message, true); } rerender(); },
  unmanual: () => review({ managed: true }, 'Switch back to managed hosts'),
  logout: async () => { await post('/api/logout'); S.authed = false; renderLogin(); },
  // hosts list
  hfilter: (el) => { P.data.filter = el.dataset.k; rerender(); },
  hview: (el) => { P.data.view = el.dataset.k; rerender(); },
  'toggle-host': (el, ev) => {
    ev.preventDefault();
    const hosts = clone(S.state.hosts); const h = hosts.find((x) => x.id === el.dataset.id); h.enabled = !h.enabled;
    saveHosts(hosts, `${h.enabled ? 'Turn on' : 'Turn off'} ${hostName(h)}`);
  },
  // editor
  htype: (el) => { P.data.ed.type = el.dataset.k; rerender(); },
  tab: (el) => { P.data.tab = el.dataset.k; rerender(); },
  'up-add': () => { P.data.ed.upstreams.push(''); rerender(); },
  'up-del': (el) => { P.data.ed.upstreams.splice(+el.dataset.i, 1); rerender(); },
  'path-add': () => { P.data.ed.paths.push({ path: '/', to: '', strip: true }); rerender(); },
  'path-del': (el) => { P.data.ed.paths.splice(+el.dataset.i, 1); rerender(); },
  'hdr-add': () => { P.data.ed.headers.push({ kind: 'res', name: '', value: '' }); rerender(); },
  'hdr-del': (el) => { P.data.ed.headers.splice(+el.dataset.i, 1); rerender(); },
  'acc-mode': (el) => { P.data.ed.access.mode = el.dataset.k; rerender(); },
  proto: (el) => { P.data.ed.proto = el.dataset.k; rerender(); },
  cert: (el) => { P.data.ed.cert = el.dataset.k; rerender(); },
  'user-add': () => { accTarget().users.push({ name: '', password: '', saved: false }); rerender(); },
  'user-del': (el) => { accTarget().users.splice(+el.dataset.i, 1); rerender(); },
  'rule-add': () => { accTarget().rules.push({ act: 'allow', cidr: '' }); rerender(); },
  'rule-quick': (el) => { accTarget().rules.push({ act: 'allow', cidr: el.dataset.v }); rerender(); },
  'rule-del': (el) => { accTarget().rules.splice(+el.dataset.i, 1); rerender(); },
  'check-now': async (el) => { el.disabled = true; await post('/api/check'); await refreshStatus(); rerender(); },
  'save-host': () => {
    const h = edHost();
    const hosts = clone(S.state.hosts); const i = hosts.findIndex((x) => x.id === h.id);
    if (i >= 0) hosts[i] = h; else hosts.push(h);
    // keep other hosts' users unchanged (server keeps their hashes)
    saveHosts(hosts, `${i >= 0 ? 'Update' : 'Add'} ${hostName(h)}`, () => { location.hash = '#/host/' + h.id; P.key = ''; render(); });
  },
  'delete-host': () => {
    const e = P.data.ed;
    if (!confirm(`Delete ${hostName(e)}? It will be removed from the Caddyfile.`)) return;
    saveHosts(S.state.hosts.filter((x) => x.id !== e.id), `Delete ${hostName(e)}`, () => { location.hash = '#/hosts'; });
  },
  // logs
  lrange: (el) => { P.data.range = el.dataset.k; delete P.data.stats; delete P.data.logs; rerender(); },
  lstatus: (el) => { P.data.status = el.dataset.k; delete P.data.logs; rerender(); },
  llive: (el) => { P.data.live = el.checked; rerender(); },
  'ai-sug': (el) => askLogs(el.dataset.k, el.dataset.h, el.dataset.r, el.textContent),
  // access lists
  'list-sel': (el) => { P.data.sel = +el.dataset.i; rerender(); },
  'list-new': () => { P.data.lists.push({ id: rid(), name: 'New list', users: [], rules: [], satisfy: 'any', pass: false }); P.data.sel = P.data.lists.length - 1; rerender(); },
  'list-save': () => review({ accessLists: listsForSave() }, 'Update access lists', () => { delete P.data.lists; rerender(); }),
  'list-del': () => {
    const l = P.data.lists[P.data.sel];
    const used = S.state.hosts.filter((h) => h.access.mode === 'list' && h.access.listId === l.id);
    if (used.length) { toast(`Used by ${used.map(hostName).join(', ')} — change those hosts first`, true); return; }
    if (!S.state.accessLists.some((x) => x.id === l.id)) { P.data.lists.splice(P.data.sel, 1); P.data.sel = 0; rerender(); return; }
    if (!confirm(`Delete access list “${l.name}”?`)) return;
    P.data.lists.splice(P.data.sel, 1); P.data.sel = 0;
    review({ accessLists: listsForSave() }, `Delete access list ${l.name}`, () => { delete P.data.lists; rerender(); });
  },
  // settings
  prov: (el) => { P.data.set.ai.provider = el.dataset.k; P.data.set.ai.baseUrl = ''; rerender(); },
  'clear-dns': async () => { if (!confirm('Remove the saved Cloudflare token? Caddy will restart.')) return; try { await post('/api/settings', { clearDnsToken: true }); S.settings = await api('/api/settings'); toast('Token removed'); } catch (e) { toast(e.message, true); } rerender(); },
  'settings-save': async (el) => {
    el.disabled = true;
    const d = P.data.set;
    try {
      await post('/api/settings', { email: d.email, dnsToken: d.dnsToken, ai: d.ai });
      S.settings = await api('/api/settings'); await refreshStatus();
      delete P.data.set; toast('Settings saved');
    } catch (e) { toast(e.message, true); }
    rerender();
  },
  'ai-test': async (el) => {
    const out = document.getElementById('aiTest'); el.disabled = true; out.textContent = 'Saving and testing…'; out.style.color = '';
    const d = P.data.set;
    try {
      await post('/api/settings', { email: d.email, dnsToken: d.dnsToken, ai: d.ai });
      S.settings = await api('/api/settings');
      const r = await post('/api/settings/test');
      out.textContent = `Connected · replied “${r.reply.slice(0, 40)}” in ${r.ms} ms`; out.style.color = 'var(--ok-ink)';
    } catch (e) { out.textContent = e.message; out.style.color = 'var(--bad-ink)'; }
    el.disabled = false;
  },
};
function accTarget() { return P.data.ed ? P.data.ed.access : P.data.lists[P.data.sel]; }

document.addEventListener('click', (ev) => {
  const el = ev.target.closest('[data-a]');
  if (!el) return;
  const fn = ACT[el.dataset.a];
  if (!fn) return;
  if (el.tagName === 'INPUT' && el.type === 'checkbox' && el.dataset.a !== 'toggle-host') { fn(el, ev); return; }
  if (el.tagName === 'A') return; // plain links
  fn(el, ev);
});

// ---------- boot ----------
(async function boot() {
  try { await api('/api/me'); S.authed = true; await loadState(); render(); }
  catch (e) { if (!S.authed) renderLogin(); else $app.innerHTML = `<div class="wrap"><div class="note bad">${esc(e.message)}</div></div>`; }
})();
