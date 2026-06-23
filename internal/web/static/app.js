// Minimal vanilla-JS dashboard. Polls /api/* every second and renders tables.

const $ = (s) => document.querySelector(s);
const fmtBytes = (n) => {
  if (!n || n < 0) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i === 0 ? n.toFixed(0) : n.toFixed(1)) + ' ' + u[i];
};
const fmtAge = (iso) => {
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 1000) return ms + ' ms';
  if (ms < 60000) return (ms/1000).toFixed(1) + ' s';
  return Math.floor(ms/60000) + 'm ' + Math.floor((ms%60000)/1000) + 's';
};
const fmtTime = (iso) => new Date(iso).toLocaleTimeString();

let currentTab = 'connections';
let filterText = '';

document.querySelectorAll('.tab').forEach((b) => {
  b.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach((x) => x.classList.remove('active'));
    b.classList.add('active');
    currentTab = b.dataset.tab;
    document.querySelectorAll('.view').forEach((v) => v.classList.remove('active'));
    $('#view-' + currentTab).classList.add('active');
  });
});

$('#filter').addEventListener('input', (e) => { filterText = e.target.value.toLowerCase(); });

$('#close-all').addEventListener('click', async () => {
  if (!confirm('Close all connections?')) return;
  await fetch('/api/connections', { method: 'DELETE' });
  refresh();
});

async function refresh() {
  try {
    const [conns, stats, proxies] = await Promise.all([
      fetch('/api/connections').then((r) => r.json()),
      fetch('/api/stats').then((r) => r.json()),
      fetch('/api/proxies').then((r) => r.json()),
    ]);
    renderStats(stats);
    renderConns(conns);
    renderProxies(proxies);
  } catch (e) {
    console.error('refresh failed', e);
  }
}

function renderStats(s) {
  $('#stat-active').textContent = s.active;
  $('#stat-up').textContent = fmtBytes(s.upload);
  $('#stat-down').textContent = fmtBytes(s.download);
  $('#stat-strategy').textContent = s.strategy + ' (' + s.proxy_count + ')';
}

function renderConns(list) {
  const body = $('#conn-body');
  const filtered = !filterText ? list : list.filter((c) =>
    (c.host || '').toLowerCase().includes(filterText) ||
    (c.target || '').toLowerCase().includes(filterText) ||
    (c.proxy || '').toLowerCase().includes(filterText)
  );
  if (filtered.length === 0) {
    body.innerHTML = '';
    $('#conn-empty').classList.add('show');
    return;
  }
  $('#conn-empty').classList.remove('show');
  body.innerHTML = filtered.map((c) => `
    <tr data-id="${c.id}">
      <td>${fmtTime(c.start_time)}</td>
      <td><span class="pill ${c.inbound.toLowerCase()}">${c.inbound}</span></td>
      <td>${escapeHTML(c.host)}</td>
      <td>${escapeHTML(c.target)}</td>
      <td class="proxy">${escapeHTML(c.proxy)}</td>
      <td>${escapeHTML(c.source)}</td>
      <td class="num">${fmtBytes(c.upload)}</td>
      <td class="num">${fmtBytes(c.download)}</td>
      <td class="num">${fmtAge(c.start_time)}</td>
      <td><button class="close" data-id="${c.id}">✕</button></td>
    </tr>
  `).join('');
  body.querySelectorAll('button.close').forEach((b) => {
    b.addEventListener('click', async () => {
      const id = b.dataset.id;
      await fetch('/api/connections/' + id, { method: 'DELETE' });
      refresh();
    });
  });
}

function renderProxies(list) {
  const body = $('#proxy-body');
  body.innerHTML = list.map((p) => `
    <tr>
      <td><b>${escapeHTML(p.name)}</b></td>
      <td><span class="pill">${p.type}</span></td>
      <td>${escapeHTML(p.addr || '–')}</td>
      <td class="${p.healthy ? 'green' : 'red'}">${p.healthy ? '● healthy' : '○ down'}</td>
    </tr>
  `).join('');
}

function escapeHTML(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
}

refresh();
setInterval(refresh, 1000);