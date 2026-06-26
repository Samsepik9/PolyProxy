// app.js — ProxyPool dashboard: 4 tabs, async validation, pool management, logs

const $ = (s) => document.querySelector(s);
const $$ = (s) => document.querySelectorAll(s);

// --- Helpers ---
const fmtBytes = (n) => {
  if (!n || n < 0) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB'];
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
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));

let currentTab = 'connections';
let filterText = '';
let crawlResults = []; // validated results
let crawlPage = 1;
const CRAWL_PAGE_SIZE = 50;
// 优先展示的来源（顺序即优先级）
const CRAWL_PRIORITY_SOURCES = ['docip', 'goodips'];
let validatePollTimer = null;

// --- Tab switching ---
$$('.tab').forEach(b => {
  b.addEventListener('click', () => {
    $$('.tab').forEach(x => x.classList.remove('active'));
    b.classList.add('active');
    currentTab = b.dataset.tab;
    $$('.view').forEach(v => v.classList.remove('active'));
    $('#view-' + currentTab).classList.add('active');
    if (currentTab === 'logs') loadLogs();
    if (currentTab === 'pool') loadPool();
  });
});

$('#filter').addEventListener('input', e => { filterText = e.target.value.toLowerCase(); });
$('#close-all').addEventListener('click', async () => {
  if (!confirm('Close all connections?')) return;
  await fetch('/api/connections', { method: 'DELETE' });
  refresh();
});

// --- Crawl tab ---
$('#fetch-proxies').addEventListener('click', async () => {
  const btn = $('#fetch-proxies');
  const status = $('#crawl-status');
  btn.disabled = true;
  status.textContent = '⏳ ' + t('crawl.fetch') + '...';
  try {
    const r = await fetch('/api/proxies/fetch', { method: 'POST' });
    const data = await r.json();
    if (data.error) {
      status.textContent = '❌ ' + data.error;
    } else {
      status.textContent = '✅ ' + data.total + ' proxies from ' + Object.keys(data.sources).length + ' sources';
      $('#crawl-summary').innerHTML = Object.entries(data.sources)
        .map(([k,v]) => `<span class="tag">${esc(k)}: ${v}</span>`).join(' ');
      if (data.errors && data.errors.length) {
        $('#crawl-summary').innerHTML += '<br><span class="red">Errors: ' + data.errors.join(', ') + '</span>';
      }
    }
  } catch(e) {
    status.textContent = '❌ Network error: ' + e.message;
  }
  btn.disabled = false;
});

$('#validate-proxies').addEventListener('click', async () => {
  const btn = $('#validate-proxies');
  const status = $('#crawl-status');
  btn.disabled = true;
  status.textContent = '⏳ Validating...';
  $('#crawl-progress').style.display = 'block';
  $('#crawl-progress .progress-text').textContent = '0%';

  try {
    const r = await fetch('/api/proxies/validate', { method: 'POST' });
    const data = await r.json();
    if (data.error) {
      status.textContent = '❌ ' + data.error;
      btn.disabled = false;
      return;
    }
    // Poll progress
    const taskId = data.task_id;
    pollValidateProgress(taskId, status, btn);
  } catch(e) {
    status.textContent = '❌ Network error: ' + e.message;
    btn.disabled = false;
  }
});

function pollValidateProgress(taskId, statusEl, btn) {
  if (validatePollTimer) clearInterval(validatePollTimer);
  validatePollTimer = setInterval(async () => {
    try {
      const r = await fetch('/api/proxies/validate/' + taskId);
      const data = await r.json();
      const pct = data.total > 0 ? Math.round(data.done / data.total * 100) : 0;
      $('#crawl-progress .progress-fill').style.width = pct + '%';
      $('#crawl-progress .progress-text').textContent = pct + '% (' + data.done + '/' + data.total + ')';
      statusEl.textContent = '⏳ ' + data.done + '/' + data.total + ' done, ' + data.valid + ' valid';

      if (!data.running) {
        clearInterval(validatePollTimer);
        validatePollTimer = null;
        $('#crawl-progress').style.display = 'none';
        statusEl.textContent = '✅ ' + data.valid + '/' + data.total + ' valid';
        btn.disabled = false;

        // Show results table
        crawlResults = data.results || [];
        crawlPage = 1; // reset to first page on new validation
        renderCrawlResults(crawlResults);
      }
    } catch(e) {
      clearInterval(validatePollTimer);
      validatePollTimer = null;
      btn.disabled = false;
    }
  }, 1000);
}

// Sort results so priority sources come first, preserving relative order within each group.
// Returns array of { result, originalIndex } so data-index remains stable for the "add to pool" handler.
function sortCrawlForDisplay(results) {
  const indexed = results.map((r, i) => ({ r, i }));
  indexed.sort((a, b) => {
    const ap = CRAWL_PRIORITY_SOURCES.indexOf(a.r.source);
    const bp = CRAWL_PRIORITY_SOURCES.indexOf(b.r.source);
    const ar = ap === -1 ? 999 : ap;
    const br = bp === -1 ? 999 : bp;
    if (ar !== br) return ar - br;
    return a.i - b.i;
  });
  return indexed;
}

function renderCrawlResults(results) {
  if (!results.length) {
    $('#crawl-empty').style.display = 'block';
    $('#crawl-table').style.display = 'none';
    renderCrawlPagination(0, 0, 0, 0);
    return;
  }
  $('#crawl-empty').style.display = 'none';
  $('#crawl-table').style.display = 'table';

  const sorted = sortCrawlForDisplay(results);
  const totalPages = Math.max(1, Math.ceil(sorted.length / CRAWL_PAGE_SIZE));
  if (crawlPage > totalPages) crawlPage = totalPages;
  if (crawlPage < 1) crawlPage = 1;
  const start = (crawlPage - 1) * CRAWL_PAGE_SIZE;
  const end = Math.min(start + CRAWL_PAGE_SIZE, sorted.length);
  const pageItems = sorted.slice(start, end);

  $('#crawl-body').innerHTML = pageItems.map(({ r, i }) => `
    <tr>
      <td><input type="checkbox" class="crawl-check" data-index="${i}" /></td>
      <td class="selectable">${esc(r.addr)}</td>
      <td><span class="pill">${esc(r.type)}</span></td>
      <td class="selectable">${esc(r.source)}</td>
      <td class="num">${r.latency_ms}ms</td>
    </tr>
  `).join('');

  $('#crawl-status').textContent = (currentLang === 'zh' ? '✅ 共 ' + results.length + ' 个可用代理' : '✅ ' + results.length + ' valid');

  renderCrawlPagination(sorted.length, start, end, totalPages);

  $('#crawl-select-all').onclick = () => {
    const checked = $('#crawl-select-all').checked;
    $$('.crawl-check').forEach(cb => cb.checked = checked);
  };
}

function renderCrawlPagination(total, start, end, totalPages) {
  let container = $('#crawl-pagination');
  if (!container) {
    container = document.createElement('div');
    container.id = 'crawl-pagination';
    container.className = 'pagination';
    const table = $('#crawl-table');
    table.parentNode.insertBefore(container, table.nextSibling);
  }

  const isZh = currentLang === 'zh';
  const prevLabel = isZh ? '‹ 上一页' : '‹ Prev';
  const nextLabel = isZh ? '下一页 ›' : 'Next ›';
  const totalLabel = isZh ? '共' : 'Total';
  const pageLabel = isZh ? '页' : '';

  if (total === 0) {
    container.innerHTML = '';
    return;
  }

  // Smart page list: always show first, last, current, and ±1 neighbor
  const pages = new Set([1, totalPages, crawlPage, crawlPage - 1, crawlPage + 1]);
  const pageList = [...pages].filter(p => p >= 1 && p <= totalPages).sort((a, b) => a - b);

  let pageBtns = '';
  let lastShown = 0;
  for (const p of pageList) {
    if (p - lastShown > 1) pageBtns += '<span class="page-ellipsis">…</span>';
    if (p === crawlPage) {
      pageBtns += `<button class="page-btn active" data-page="${p}">${p}</button>`;
    } else {
      pageBtns += `<button class="page-btn" data-page="${p}">${p}</button>`;
    }
    lastShown = p;
  }

  const info = `${start + 1}–${end} / ${total}`;
  container.innerHTML = `
    <span class="page-info">${info} <span class="muted">(${totalLabel} ${total}${pageLabel ? ' ' + pageLabel : ''})</span></span>
    <button class="page-btn" data-page="prev" ${crawlPage === 1 ? 'disabled' : ''}>${prevLabel}</button>
    ${pageBtns}
    <button class="page-btn" data-page="next" ${crawlPage === totalPages ? 'disabled' : ''}>${nextLabel}</button>
  `;

  container.querySelectorAll('.page-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const v = btn.dataset.page;
      if (btn.disabled) return;
      if (v === 'prev') crawlPage--;
      else if (v === 'next') crawlPage++;
      else crawlPage = parseInt(v, 10);
      renderCrawlResults(crawlResults);
    });
  });
}

$('#add-selected').addEventListener('click', async () => {
  const selected = [];
  $$('.crawl-check:checked').forEach(cb => {
    const idx = parseInt(cb.dataset.index);
    if (crawlResults[idx]) selected.push(crawlResults[idx]);
  });
  if (!selected.length) {
    $('#crawl-status').textContent = '⚠️ ' + (currentLang === 'zh' ? '请先勾选代理' : 'Select proxies first');
    return;
  }
  const r = await fetch('/api/pool/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ proxies: selected.map(s => ({ addr: s.addr, type: s.type })) })
  });
  const data = await r.json();
  $('#crawl-status').textContent = '✅ ' + data.added + ' added to pool';
});

// --- Dynamic proxy ---
let dynamicPollTimer = null;
const $dynamicBtn = $('#dynamic-proxy');
const $intervalInput = $('#dynamic-interval');
const $autoPoolBtn = $('#auto-pool');

$dynamicBtn.addEventListener('click', async () => {
  const btn = $dynamicBtn;
  const status = $('#crawl-status');
  btn.disabled = true;
  try {
    // Check current state
    const state = await fetch('/api/proxies/dynamic').then(r => r.json());
    if (state.enabled) {
      // Stop
      await fetch('/api/proxies/dynamic', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'stop' })
      });
      if (dynamicPollTimer) { clearInterval(dynamicPollTimer); dynamicPollTimer = null; }
      btn.textContent = t('crawl.dynamic');
      btn.classList.remove('active-toggle');
      status.textContent = '⏹ ' + t('crawl.dynamicStopped');
    } else {
      // Start
      const interval = parseInt($intervalInput.value, 10) || 60;
      const r = await fetch('/api/proxies/dynamic', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'start', interval })
      });
      const data = await r.json();
      if (data.error) { status.textContent = '❌ ' + data.error; return; }
      btn.textContent = '⏹ ' + t('crawl.dynamicStop');
      btn.classList.add('active-toggle');
      status.textContent = '🔄 ' + t('crawl.dynamicRunning') + ' (' + data.interval + 's)';
      pollDynamicStatus();
    }
  } catch(e) {
    status.textContent = '❌ ' + e.message;
  }
  btn.disabled = false;
});

function pollDynamicStatus() {
  if (dynamicPollTimer) clearInterval(dynamicPollTimer);
  dynamicPollTimer = setInterval(async () => {
    try {
      const data = await fetch('/api/proxies/dynamic').then(r => r.json());
      if (!data.enabled) {
        clearInterval(dynamicPollTimer);
        dynamicPollTimer = null;
        $dynamicBtn.textContent = t('crawl.dynamic');
        $dynamicBtn.classList.remove('active-toggle');
        return;
      }
      if (data.last_run) {
        const lr = data.last_run;
        const info = t('crawl.cycleInfo')
          .replace('{fetched}', lr.fetched)
          .replace('{valid}', lr.valid)
          .replace('{added}', lr.added);
        $('#crawl-status').textContent = '🔄 ' + info + (data.running ? ' ⏳' : '');
      }
      // If a cycle just finished and has a task_id, show results
      if (data.task_id && !data.running) {
        // Fetch the dynamic cycle results and update the crawl results table
        const now = Date.now();
        const ts = parseInt(data.task_id.split('-')[1] || '0');
        if (now - (ts / 1000000) < 60000) { // within 1 minute
          // Refresh the crawl results display if there are any cached
        }
      }
    } catch(e) {}
  }, 2000);
}

// --- Auto pool ---
$autoPoolBtn.addEventListener('click', async () => {
  const btn = $autoPoolBtn;
  const status = $('#crawl-status');
  btn.disabled = true;
  try {
    // Check current state by getting dynamic status
    const state = await fetch('/api/proxies/dynamic').then(r => r.json());
    const newEnabled = !(state.auto_pool || false);
    const r = await fetch('/api/proxies/auto-pool', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: newEnabled })
    });
    const data = await r.json();
    if (data.auto_pool) {
      btn.textContent = '🤖 ' + t('crawl.autoPoolOn');
      btn.classList.add('active-toggle');
      status.textContent = '✅ ' + t('crawl.autoPoolEnabled');
    } else {
      btn.textContent = '🤖 ' + t('crawl.autoPoolOff');
      btn.classList.remove('active-toggle');
      status.textContent = '⏹ ' + t('crawl.autoPoolDisabled');
    }
  } catch(e) {
    status.textContent = '❌ ' + e.message;
  }
  btn.disabled = false;
});

// --- Pool tab ---
async function loadPool() {
  try {
    const [proxies, stats] = await Promise.all([
      fetch('/api/proxies').then(r => r.json()),
      fetch('/api/stats').then(r => r.json()),
    ]);
    $('#pool-strategy').value = stats.strategy || 'random';
    renderPool(proxies);
    // Restore pinned name selection
    const sel = $('#pool-specific');
    if (stats.pinned_name && sel.querySelector(`option[value="${stats.pinned_name}"]`)) {
      sel.value = stats.pinned_name;
    }
    // Show/hide specific dropdown based on strategy
    sel.style.display = stats.strategy === 'name' ? 'inline' : 'none';
  } catch(e) {}
}

function renderPool(list) {
  $('#pool-body').innerHTML = list.map(p => `
    <tr>
      <td class="selectable"><b>${esc(p.name)}</b></td>
      <td><span class="pill">${p.type}</span></td>
      <td class="selectable">${esc(p.addr || '–')}</td>
      <td class="${p.healthy ? 'green' : 'red'}">${p.healthy ? '● healthy' : '○ down'}</td>
      <td>${p.name !== 'direct' ? `<button class="del-proxy" data-name="${esc(p.name)}">✕</button>` : ''}</td>
    </tr>
  `).join('');

  $$('#pool-body .del-proxy').forEach(b => {
    b.addEventListener('click', async () => {
      const name = b.dataset.name;
      if (!confirm('Remove proxy "' + name + '" from pool?')) return;
      await fetch('/api/proxies/' + encodeURIComponent(name), { method: 'DELETE' });
      loadPool();
    });
  });

  // Update specific proxy dropdown ONLY if the option list changed (preserve selection)
  const sel = $('#pool-specific');
  const prevValue = sel.value;
  const newOptions = list.filter(p => p.name !== 'direct').map(p =>
    `<option value="${esc(p.name)}">${esc(p.name)}</option>`
  ).join('');
  const newHTML = newOptions;
  if (sel.innerHTML !== newHTML) {
    sel.innerHTML = newHTML;
    if (prevValue && sel.querySelector(`option[value="${prevValue}"]`)) {
      sel.value = prevValue;
    }
  }
}

$('#pool-strategy').addEventListener('change', async () => {
  const strategy = $('#pool-strategy').value;
  const sel = $('#pool-specific');
  sel.style.display = strategy === 'name' ? 'inline' : 'none';
  const body = { strategy };
  if (strategy === 'name') {
    body.pinned_name = sel.value || '';
  }
  const r = await fetch('/api/pool/strategy', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  const data = await r.json();
  $('#pool-status').textContent = '✅ Strategy: ' + data.strategy +
    (data.strategy === 'name' && data.pinned_name ? ' (pinned: ' + data.pinned_name + ')' : '');
});

$('#pool-specific').addEventListener('change', async () => {
  const strategy = $('#pool-strategy').value;
  if (strategy !== 'name') return; // only relevant in "name" mode
  const pinned = $('#pool-specific').value;
  const r = await fetch('/api/pool/strategy', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ strategy, pinned_name: pinned })
  });
  const data = await r.json();
  $('#pool-status').textContent = '✅ Pinned to: ' + data.pinned_name;
});

$('#pool-save').addEventListener('click', async () => {
  const r = await fetch('/api/pool/save', { method: 'POST' });
  const data = await r.json();
  $('#pool-status').textContent = data.ok ? '✅ ' + (currentLang === 'zh' ? '已保存' : 'Saved') : '❌ ' + (data.error || 'Failed');
});

// --- Logs tab ---
async function loadLogs() {
  const level = $('#log-level').value;
  try {
    const r = await fetch('/api/logs?level=' + level + '&limit=200');
    const data = await r.json();
    const container = $('#log-container');
    container.innerHTML = data.map(e => `
      <div class="log-entry log-${e.level}">
        <span class="log-time">${esc(e.time)}</span>
        <span class="log-level">[${e.level.toUpperCase()}]</span>
        <span class="log-module">[${esc(e.module)}]</span>
        <span class="log-msg selectable">${esc(e.message)}</span>
      </div>
    `).join('');
    container.scrollTop = container.scrollHeight;
  } catch(e) {}
}

$('#log-level').addEventListener('change', loadLogs);
$('#logs-refresh').addEventListener('click', loadLogs);
$('#logs-clear').addEventListener('click', () => { $('#log-container').innerHTML = ''; });

// --- Main refresh loop ---
async function refresh() {
  try {
    const [conns, stats] = await Promise.all([
      fetch('/api/connections').then(r => r.json()),
      fetch('/api/stats').then(r => r.json()),
    ]);
    renderStats(stats);
    renderConns(conns);
    if (currentTab === 'pool') loadPool();
  } catch(e) {}
}

function renderStats(s) {
  $('#stat-active').textContent = s.active;
  $('#stat-up').textContent = fmtBytes(s.upload);
  $('#stat-down').textContent = fmtBytes(s.download);
  $('#stat-strategy').textContent = s.strategy + ' (' + s.proxy_count + ')';
}

function renderConns(list) {
  const body = $('#conn-body');
  const filtered = !filterText ? list : list.filter(c =>
    (c.host||'').toLowerCase().includes(filterText) ||
    (c.target||'').toLowerCase().includes(filterText) ||
    (c.proxy||'').toLowerCase().includes(filterText)
  );
  if (!filtered.length) {
    body.innerHTML = '';
    $('#conn-empty').classList.add('show');
    return;
  }
  $('#conn-empty').classList.remove('show');
  body.innerHTML = filtered.map(c => `
    <tr>
      <td class="selectable">${fmtTime(c.start_time)}</td>
      <td><span class="pill ${c.inbound.toLowerCase()}">${c.inbound}</span></td>
      <td class="selectable">${esc(c.host)}</td>
      <td class="selectable">${esc(c.target)}</td>
      <td class="proxy selectable">${esc(c.proxy)}</td>
      <td class="selectable">${esc(c.source)}</td>
      <td class="num">${fmtBytes(c.upload)}</td>
      <td class="num">${fmtBytes(c.download)}</td>
      <td class="num">${fmtAge(c.start_time)}</td>
      <td><button class="close" data-id="${c.id}">✕</button></td>
    </tr>
  `).join('');
  body.querySelectorAll('button.close').forEach(b => {
    b.addEventListener('click', async () => {
      await fetch('/api/connections/' + b.dataset.id, { method: 'DELETE' });
      refresh();
    });
  });
}

// --- Init ---
refresh();
setInterval(refresh, 1000);
