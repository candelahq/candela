// ── Candela Local Management UI ──
// Consumes ConnectRPC RuntimeService via JSON POST (Connect protocol).

'use strict';

const RPC_BASE = '';  // Same origin — served from candela-local.

// ── ConnectRPC JSON client ──

async function rpc(method, body = {}) {
  const resp = await fetch(`${RPC_BASE}/candela.v1.RuntimeService/${method}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });

  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ message: resp.statusText }));
    throw new Error(err.message || `RPC ${method} failed: ${resp.status}`);
  }
  return resp.json();
}

// ── DOM helpers ──

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => document.querySelectorAll(sel);

function show(el) { el.classList.remove('hidden'); }
function hide(el) { el.classList.add('hidden'); }

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '—';
  const gb = bytes / 1e9;
  if (gb >= 1) return `${gb.toFixed(1)} GB`;
  const mb = bytes / 1e6;
  return `${mb.toFixed(0)} MB`;
}

function formatUptime(seconds) {
  if (!seconds || seconds <= 0) return '—';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${Math.floor(seconds)}s`;
}

// ── State ──

let currentHealth = null;
let pollTimer = null;
let pullPollTimer = null;
const completedPulls = new Set(); // tracks models we've already refreshed for

// ── Render functions ──

function renderHealth(data) {
  const status = data.status || {};
  const st = status.status || 'stopped';
  const models = data.models || [];

  // Header status.
  $('#header-backend').textContent = `${status.backend || '—'} — ${st}`;
  const dot = $('#header-dot');
  dot.className = 'status-dot ' + st;

  // Health badge.
  const badge = $('#health-badge');
  badge.className = 'health-badge ' + st;
  $('#health-dot').className = 'badge-dot';
  $('#health-status').textContent = st.toUpperCase();

  // Details.
  $('#health-endpoint').textContent = status.endpoint || '—';
  $('#health-uptime').textContent = formatUptime(status.uptimeSeconds);
  $('#health-backend').textContent = status.backend || '—';

  // Error.
  const errEl = $('#health-error');
  if (status.error) {
    errEl.textContent = status.error;
    show(errEl);
  } else {
    hide(errEl);
  }

  // Buttons.
  if (st === 'running' || st === 'starting') {
    $('#btn-start').style.display = 'none';
    $('#btn-stop').style.display = '';
  } else {
    $('#btn-start').style.display = '';
    $('#btn-stop').style.display = 'none';
  }

  // Footer.
  if (status.checkedAt) {
    const d = new Date(status.checkedAt);
    $('#footer-checked').textContent = `Last check: ${d.toLocaleTimeString()}`;
  }

  // Render models only on initial load. After that, models are updated
  // only by explicit user actions (refresh, load, unload).
  if (currentHealth === null && models.length > 0) {
    renderModels(models);
  }

  currentHealth = status;
}

// Track models mid-load/unload so health poll doesn't reset their spinner.
const pendingModels = new Map(); // model ID -> 'loading' | 'unloading'

function renderModels(models) {
  const list = $('#models-list');

  if (!models || models.length === 0) {
    list.innerHTML = '<div class="empty-state">No models available. Pull a model to get started.</div>';
    return;
  }

  // Keep the active-pulls section intact — only replace the model rows.
  const pullsSection = list.querySelector('.active-pulls');
  const pullsHtml = pullsSection ? pullsSection.outerHTML : '';

  list.innerHTML = models.map(m => {
    const loaded = m.loaded;
    const dotClass = loaded ? 'loaded' : 'available';
    const statusText = loaded ? 'Loaded' : 'Available';
    const meta = [
      m.family || '',
      m.parameters ? `${m.parameters} params` : '',
      formatBytes(m.sizeBytes),
    ].filter(Boolean).join(' · ');

    let action;
    const pending = pendingModels.get(m.id);
    if (pending === 'loading') {
      action = `<button class="btn btn-sm btn-model" disabled><span class="spinner"></span> Loading…</button>`;
    } else if (pending === 'unloading') {
      action = `<button class="btn btn-sm btn-model" disabled><span class="spinner"></span> Unloading…</button>`;
    } else if (pending === 'deleting') {
      action = `<button class="btn btn-sm btn-model" disabled><span class="spinner"></span> Deleting…</button>`;
    } else if (loaded) {
      action = `<button class="btn btn-sm btn-model" data-action="unload" data-model-id="${escapeAttr(m.id)}">Unload</button>`;
    } else {
      action = `<button class="btn btn-sm btn-model" data-action="load" data-model-id="${escapeAttr(m.id)}">Load</button>`;
    }

    const deleteBtn = pending
      ? ''
      : `<button class="btn btn-sm btn-danger" data-action="delete" data-model-id="${escapeAttr(m.id)}" title="Delete model from disk">🗑</button>`;

    return `
      <div class="model-row">
        <div class="model-info">
          <span class="model-dot ${dotClass}"></span>
          <span class="model-name">${escapeHtml(m.id)}</span>
          <span class="model-meta">${escapeHtml(meta)} · ${statusText}</span>
        </div>
        <div class="model-actions">
          ${action}
          ${deleteBtn}
        </div>
      </div>
    `;
  }).join('');

  // Bind click handlers via event delegation (safe against XSS).
  list.querySelectorAll('[data-action]').forEach(btn => {
    btn.addEventListener('click', () => {
      const modelId = btn.getAttribute('data-model-id');
      if (btn.dataset.action === 'load') loadModel(modelId, btn);
      else if (btn.dataset.action === 'unload') unloadModel(modelId, btn);
      else if (btn.dataset.action === 'delete') deleteModel(modelId, btn);
    });
  });
}

// ── Active Pulls ──

function renderActivePulls(pulls) {
  let container = $('#models-list').querySelector('.active-pulls');
  if (!pulls || pulls.length === 0) {
    if (container) container.remove();
    // Stop fast-polling when no active pulls.
    if (pullPollTimer) {
      clearInterval(pullPollTimer);
      pullPollTimer = null;
    }
    return;
  }

  const html = pulls.map(p => {
    const pct = Math.round(p.percent);
    let statusClass = 'pull-active';
    let label = `Pulling… ${pct}%`;
    if (p.status === 'complete') {
      statusClass = 'pull-complete';
      label = 'Complete!';
    } else if (p.status === 'failed') {
      statusClass = 'pull-failed';
      label = `Failed: ${escapeHtml(p.error)}`;
    } else if (p.status === 'cancelled') {
      statusClass = 'pull-failed';
      label = 'Cancelled';
    }

    const cancelBtn = (p.status === 'pulling')
      ? `<button class="btn btn-sm btn-danger pull-cancel" data-action="cancel-pull" data-model-id="${escapeAttr(p.model)}" title="Cancel pull">✕</button>`
      : '';

    return `
      <div class="pull-row ${statusClass}">
        <div class="pull-info">
          <span class="pull-model">${escapeHtml(p.model)}</span>
          <span class="pull-label">${label}</span>
          ${cancelBtn}
        </div>
        <div class="pull-bar-track">
          <div class="pull-bar-fill" style="width: ${pct}%"></div>
        </div>
      </div>
    `;
  }).join('');

  if (!container) {
    container = document.createElement('div');
    container.className = 'active-pulls';
    $('#models-list').prepend(container);
  }
  container.innerHTML = html;

  // Bind cancel buttons.
  container.querySelectorAll('[data-action="cancel-pull"]').forEach(btn => {
    btn.addEventListener('click', () => cancelPull(btn.getAttribute('data-model-id')));
  });

  // Start fast-polling (every 2s) while pulls are active.
  if (!pullPollTimer) {
    pullPollTimer = setInterval(refreshPulls, 2000);
  }
}

async function refreshPulls() {
  try {
    const resp = await fetch('/_local/api/pulls');
    const pulls = await resp.json();
    renderActivePulls(pulls);

    // Auto-refresh model list when a pull completes.
    if (pulls) {
      let needsRefresh = false;
      for (const p of pulls) {
        if (p.status === 'complete' && !completedPulls.has(p.model)) {
          completedPulls.add(p.model);
          needsRefresh = true;
        }
      }
      if (needsRefresh) refreshModels();
      // Clean up completed entries that are no longer in the list.
      const activeModels = new Set(pulls.map(p => p.model));
      for (const m of completedPulls) {
        if (!activeModels.has(m)) completedPulls.delete(m);
      }
    }
  } catch (err) {
    console.error('refreshPulls failed:', err);
  }
}

function renderBackends(data) {
  const list = $('#backends-list');
  const backends = data.backends || [];
  const active = data.active || '';

  if (backends.length === 0) {
    list.innerHTML = '<div class="empty-state">No backends detected.</div>';
    return;
  }

  list.innerHTML = backends.map(b => {
    let badgeClass, badgeText;
    if (b.name === active) {
      badgeClass = 'active';
      badgeText = 'Active';
    } else if (b.installed) {
      badgeClass = 'installed';
      badgeText = 'Installed';
    } else {
      badgeClass = 'missing';
      badgeText = 'Not Found';
    }

    const hint = b.installed
      ? (b.binaryPath || '')
      : (b.installHint || '');

    return `
      <div class="backend-row">
        <div class="backend-info">
          <span class="backend-name">${escapeHtml(b.name)}</span>
          <span class="backend-hint">${escapeHtml(hint)}</span>
        </div>
        <span class="backend-badge ${badgeClass}">${badgeText}</span>
      </div>
    `;
  }).join('');
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escapeAttr(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// ── Actions ──

async function refreshHealth() {
  try {
    const data = await rpc('GetHealth');
    renderHealth(data);
  } catch (err) {
    console.error('GetHealth failed:', err);
    renderHealth({
      status: { status: 'error', error: err.message, endpoint: '—', backend: '—' },
      models: [],
    });
  }
}

async function refreshModels() {
  try {
    const data = await rpc('ListModels');
    renderModels(data.models || []);
  } catch (err) {
    console.error('ListModels failed:', err);
  }
}

async function refreshBackends() {
  try {
    const data = await rpc('ListBackends');
    renderBackends(data);
  } catch (err) {
    console.error('ListBackends failed:', err);
    $('#backends-list').innerHTML =
      `<div class="empty-state">Failed to load backends: ${escapeHtml(err.message)}</div>`;
  }
}

async function startRuntime() {
  const btn = $('#btn-start');
  const origText = btn.textContent;
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Starting…';

  // Immediately show "starting" state in health badge.
  const badge = $('#health-badge');
  badge.className = 'health-badge starting';
  $('#health-status').textContent = 'STARTING';

  try {
    await rpc('StartRuntime');
    // Poll health quickly until it's running or we timeout.
    for (let i = 0; i < 15; i++) {
      await new Promise(r => setTimeout(r, 1000));
      await refreshHealth();
      const status = $('#health-status').textContent;
      if (status === 'RUNNING' || status === 'ERROR') break;
    }
  } catch (err) {
    alert('Start failed: ' + err.message);
    await refreshHealth();
  } finally {
    btn.disabled = false;
    btn.textContent = origText;
  }
}

async function stopRuntime() {
  const btn = $('#btn-stop');
  const origText = btn.textContent;
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Stopping…';

  try {
    const data = await rpc('StopRuntime');
    renderHealth({ status: data.status, models: [] });
  } catch (err) {
    alert('Stop failed: ' + err.message);
  } finally {
    btn.disabled = false;
    btn.textContent = origText;
  }
}

async function loadModel(modelId, btn) {
  pendingModels.set(modelId, 'loading');
  if (btn) {
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> Loading…';
  }
  try {
    await rpc('LoadModel', { model: modelId });
  } catch (err) {
    alert('Load failed: ' + err.message);
  } finally {
    pendingModels.delete(modelId);
    await refreshModels();
  }
}

async function unloadModel(modelId, btn) {
  pendingModels.set(modelId, 'unloading');
  if (btn) {
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> Unloading…';
  }
  try {
    await rpc('UnloadModel', { model: modelId });
  } catch (err) {
    alert('Unload failed: ' + err.message);
  } finally {
    pendingModels.delete(modelId);
    await refreshModels();
  }
}

async function pullModel(modelId) {
  const statusEl = $('#pull-status');
  try {
    await rpc('PullModel', { model: modelId });
    statusEl.textContent = `Pull started for "${modelId}"…`;
    show(statusEl);
    setTimeout(() => hide(statusEl), 5000);
    // Immediately fetch pull status.
    await refreshPulls();
  } catch (err) {
    statusEl.textContent = `Pull failed: ${err.message}`;
    show(statusEl);
  }
}

async function cancelPull(modelId) {
  try {
    await rpc('CancelPull', { model: modelId });
    await refreshPulls();
  } catch (err) {
    alert('Cancel failed: ' + err.message);
  }
}

async function deleteModel(modelId, btn) {
  if (!confirm(`Delete "${modelId}" from disk? This cannot be undone.`)) return;
  pendingModels.set(modelId, 'deleting');
  if (btn) {
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span>';
  }
  try {
    await rpc('DeleteModel', { model: modelId });
  } catch (err) {
    alert('Delete failed: ' + err.message);
  } finally {
    pendingModels.delete(modelId);
    await refreshModels();
  }
}

async function resetState() {
  if (!confirm('Reset all local state? This clears preferences and pull history. Your config file is not affected.')) {
    return;
  }
  try {
    await rpc('ResetState');
    alert('State reset. Reloading…');
    location.reload();
  } catch (err) {
    alert('Reset failed: ' + err.message);
  }
}

// ── Models Catalog ──

async function renderPopularModels() {
  const grid = $('#popular-grid');
  try {
    const data = await rpc('ListCatalog');
    const models = data.models || [];
    if (models.length === 0) {
      grid.innerHTML = '<div class="empty-state">No catalog models.</div>';
      return;
    }
    grid.innerHTML = models.map(m => `
      <button class="popular-item" data-model="${escapeAttr(m.id)}" title="Click to fill pull input">
        <span class="popular-name">${escapeHtml(m.id)}</span>
        <span class="popular-desc">${escapeHtml(m.description)}</span>
        <span class="popular-size">${escapeHtml(m.sizeHint)}</span>
      </button>
    `).join('');

    grid.querySelectorAll('.popular-item').forEach(btn => {
      btn.addEventListener('click', () => {
        const input = $('#pull-input');
        input.value = btn.dataset.model;
        input.focus();
      });
    });
  } catch (err) {
    console.error('ListCatalog failed:', err);
    grid.innerHTML = '<div class="empty-state">Failed to load catalog.</div>';
  }
}

// ── Event listeners ──

$('#btn-start').addEventListener('click', startRuntime);
$('#btn-stop').addEventListener('click', stopRuntime);
$('#btn-refresh-models').addEventListener('click', refreshModels);
$('#btn-reset').addEventListener('click', resetState);

$('#pull-form').addEventListener('submit', (e) => {
  e.preventDefault();
  const input = $('#pull-input');
  const model = input.value.trim();
  if (model) {
    pullModel(model);
    input.value = '';
  }
});

// ── Initialize ──

async function init() {
  // Fire all initial data loads in parallel.
  await Promise.allSettled([
    refreshHealth(),
    refreshModels(),
    refreshBackends(),
    refreshPulls(),
    renderPopularModels(),
  ]);

  // Poll health every 5 seconds (updates badge only, not models).
  pollTimer = setInterval(refreshHealth, 5000);
}

init();
