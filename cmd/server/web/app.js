(function () {
  'use strict';

  const API_BASE = '';
  function getApiKey() {
    return window.DEMO_API_KEY && window.DEMO_API_KEY !== '__DEMO_API_KEY__'
      ? window.DEMO_API_KEY
      : (sessionStorage.getItem('api_key') || '');
  }
  function getOperatorId() {
    return sessionStorage.getItem('operator_id') || '';
  }
  function setOperatorId(id) {
    sessionStorage.setItem('operator_id', id);
  }

  function headers() {
    const h = { 'Content-Type': 'application/json' };
    const key = getApiKey();
    if (key) h['Authorization'] = 'Bearer ' + key;
    return h;
  }
  function headersWithOperator(opId) {
    const h = headers();
    if (opId) h['X-Operator-ID'] = opId;
    return h;
  }

  async function api(path, opts = {}) {
    const url = (path.startsWith('http') ? path : API_BASE + path);
    const res = await fetch(url, {
      ...opts,
      headers: { ...headers(), ...(opts.headers || {}) },
    });
    const text = await res.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch (_) {}
    if (!res.ok) {
      const err = new Error(data?.message || res.statusText || 'Request failed');
      err.status = res.status;
      err.data = data;
      throw err;
    }
    return data;
  }

  function parsePath() {
    const path = window.location.pathname.replace(/\/$/, '') || '/';
    const parts = path.split('/').filter(Boolean);
    if (parts[0] === 'transfers' && parts[1]) return { view: 'transfer', id: parts[1] };
    if (parts[0] === 'submit') return { view: 'submit' };
    if (parts[0] === 'operator') return { view: 'operator' };
    return { view: 'dashboard' };
  }

  async function renderDashboard() {
    const key = getApiKey();
    if (!key) {
      return '<div class="container"><h1>Dashboard</h1><p class="error">Set API key in sessionStorage (api_key) or use demo key from server.</p></div>';
    }
    let data;
    try {
      data = await api('/api/v1/stats/dashboard');
    } catch (e) {
      return '<div class="container"><h1>Dashboard</h1><p class="error">Failed to load stats: ' + (e.data?.message || e.message) + '</p><button onclick="app.refreshRoute()">Refresh</button></div>';
    }
    const byState = data.deposits_by_state || {};
    const stateHtml = Object.entries(byState).map(([s, n]) => '<span class="state-count">' + s + ': ' + n + '</span>').join('');
    const cards = [
      { label: 'Gating correctness', value: data.gating_correctness || 'N/A' },
      { label: 'Settlement reconciliation', value: data.settlement_reconciliation || 'N/A' },
      { label: 'Vendor scenario coverage', value: data.vendor_scenario_coverage || 'N/A' },
      { label: 'Operator queue response', value: data.operator_queue_response || 'N/A' },
      { label: 'Return accuracy', value: data.return_accuracy || 'N/A' },
      { label: 'Queue count', value: String(data.queue_count ?? 0) },
    ].map(c => '<div class="card"><div class="label">' + c.label + '</div><div class="value">' + c.value + '</div></div>').join('');
    return '<div class="container"><h1>Dashboard</h1><p><button onclick="app.refreshRoute()">Refresh</button></p><h2>Deposits by state</h2><div class="state-counts">' + stateHtml + '</div><div class="dashboard-cards">' + cards + '</div></div>';
  }

  function renderSubmit() {
    const key = getApiKey();
    const msg = key ? '' : '<p class="error">No API key. Set sessionStorage.api_key or use demo key.</p>';
    return '<div class="container"><h1>Submit deposit</h1>' + msg +
      '<form id="submit-form">' +
      '<label>Account ID <input type="text" name="account_id" placeholder="PASS-10001" required></label>' +
      '<label>Amount (cents) <input type="number" name="amount_cents" placeholder="15000" required></label>' +
      '<button type="submit">Submit</button>' +
      '</form><div id="submit-msg"></div></div>';
  }

  async function loadImage(id, side) {
    const key = getApiKey();
    const res = await fetch(API_BASE + '/api/v1/deposits/' + id + '/images/' + side, {
      headers: key ? { 'Authorization': 'Bearer ' + key } : {},
    });
    if (!res.ok) return null;
    const blob = await res.blob();
    return URL.createObjectURL(blob);
  }

  async function renderOperator() {
    const key = getApiKey();
    if (!key) {
      return '<div class="container"><h1>Operator queue</h1><p class="error">API key required. Set sessionStorage.api_key.</p></div>';
    }
    let list;
    try {
      list = await api('/api/v1/operator/queue');
    } catch (e) {
      return '<div class="container"><h1>Operator queue</h1><p class="error">' + (e.data?.message || e.message) + '</p></div>';
    }
    const opId = getOperatorId();
    const sorted = (Array.isArray(list) ? list : []).slice().sort((a, b) => (b.risk_score || 0) - (a.risk_score || 0));
    const opBar = '<div class="operator-bar">Operator ID: <input type="text" id="operator-id" value="' + (opId || 'op1') + '" placeholder="op1"> <button type="button" onclick="app.saveOperatorId()">Save</button></div>';
    let html = '<div class="container"><h1>Operator queue</h1>' + opBar + '<p><button onclick="app.refreshRoute()">Refresh queue</button></p>';
    if (sorted.length === 0) {
      html += '<p class="muted">No flagged deposits.</p></div>';
      return html;
    }
    for (const t of sorted) {
      const risk = t.risk_score >= 65 ? 'high' : 'low';
      const riskLabel = (t.risk_score != null ? t.risk_score : '—') + '';
      const micr = [t.micr_routing, t.micr_account, t.check_number].filter(Boolean).join(' | ') || '—';
      html += '<div class="queue-item" data-transfer-id="' + t.transfer_id + '">';
      html += '<div class="images"><img data-id="' + t.transfer_id + '" data-side="front" alt="front"><img data-id="' + t.transfer_id + '" data-side="back" alt="back"></div>';
      html += '<div class="meta">';
      html += '<span class="risk ' + risk + '">Risk ' + riskLabel + '</span> ';
      html += '<strong>$' + ((t.amount_cents || 0) / 100).toFixed(2) + '</strong> — ' + (t.investor_account_id || '') + '<br>';
      html += '<small>MICR: ' + micr + '</small><br>';
      html += '<a href="/transfers/' + t.transfer_id + '">View detail</a>';
      html += '</div>';
      html += '<div class="actions">';
      html += '<select class="contribution-type"><option value="">—</option><option value="INDIVIDUAL">INDIVIDUAL</option><option value="EMPLOYER">EMPLOYER</option><option value="ROLLOVER">ROLLOVER</option></select>';
      html += '<button type="button" class="approve">Approve</button>';
      html += '<button type="button" class="reject">Reject</button>';
      html += '</div></div>';
    }
    html += '</div>';
    setTimeout(function () {
      document.querySelectorAll('.queue-item .images img').forEach(function (img) {
        const id = img.getAttribute('data-id');
        const side = img.getAttribute('data-side');
        loadImage(id, side).then(function (url) {
          if (url) img.src = url;
        });
      });
      document.querySelectorAll('.queue-item').forEach(function (el) {
        const id = el.getAttribute('data-transfer-id');
        el.querySelector('button.approve').onclick = function () {
          const opId = document.getElementById('operator-id').value || getOperatorId();
          if (!opId) { alert('Set Operator ID first'); return; }
          const contrib = el.querySelector('select.contribution-type').value;
          fetch(API_BASE + '/api/v1/operator/queue/' + id + '/approve', {
            method: 'POST',
            headers: headersWithOperator(opId),
            body: JSON.stringify({ operator_id: opId, contribution_type: contrib || undefined }),
          }).then(function (r) {
            if (r.ok) app.refreshRoute();
            else return r.json().then(function (d) { throw new Error(d.message || r.statusText); });
          }).catch(function (e) { alert(e.message); });
        };
        el.querySelector('button.reject').onclick = function () {
          const opId = document.getElementById('operator-id').value || getOperatorId();
          if (!opId) { alert('Set Operator ID first'); return; }
          const reason = prompt('Reject reason:', 'Rejected by operator');
          if (reason == null) return;
          fetch(API_BASE + '/api/v1/operator/queue/' + id + '/reject', {
            method: 'POST',
            headers: headersWithOperator(opId),
            body: JSON.stringify({ operator_id: opId, reason: reason || 'Rejected by operator' }),
          }).then(function (r) {
            if (r.ok) app.refreshRoute();
            else return r.json().then(function (d) { throw new Error(d.message || r.statusText); });
          }).catch(function (e) { alert(e.message); });
        };
      });
    }, 0);
    return html;
  }

  function isPlaceholderId(id) {
    if (!id || typeof id !== 'string') return true;
    if (id.indexOf('{') !== -1 || id.indexOf('}') !== -1) return true;
    if (id === 'id' || id === ':id') return true;
    return false;
  }

  async function renderTransfer(id) {
    const key = getApiKey();
    if (!key) {
      return '<div class="container"><h1>Transfer</h1><p class="error">API key required.</p></div>';
    }
    if (isPlaceholderId(id)) {
      return '<div class="container"><h1>Transfer detail</h1><p class="muted">Use a real transfer ID.</p>' +
        '<p>Open the <a href="/operator">Operator queue</a> and click &laquo; View detail &raquo; on a deposit, or use the <code>transfer_id</code> from a successful submission on <a href="/submit">Submit deposit</a>.</p>' +
        '<p><a href="/">Dashboard</a> &middot; <a href="/operator">Operator queue</a></p></div>';
    }
    let transfer, history;
    try {
      transfer = await api('/api/v1/deposits/' + encodeURIComponent(id));
      history = await api('/api/v1/deposits/' + encodeURIComponent(id) + '/history');
    } catch (e) {
      return '<div class="container"><h1>Transfer not found</h1><p class="error">' + (e.data?.message || e.message) + '</p>' +
        '<p><a href="/">Dashboard</a> &middot; <a href="/operator">Operator queue</a></p></div>';
    }
    let html = '<div class="container transfer-detail"><h1>Transfer ' + (transfer.transfer_id || id) + '</h1>';
    html += '<div class="section"><h2>Details</h2><p>Status: <strong>' + (transfer.status || '') + '</strong> | Account: ' + (transfer.investor_account_id || '') + ' | Amount: $' + ((transfer.amount_cents || 0) / 100).toFixed(2) + '</p></div>';
    html += '<div class="section"><h2>Decision trace (events)</h2><ul class="events">';
    const events = Array.isArray(history) ? history : [];
    events.forEach(function (ev) {
      const payload = ev.payload ? ('<pre>' + ev.payload.replace(/</g, '&lt;') + '</pre>') : '';
      html += '<li><span class="event-type">' + (ev.event_type || '') + '</span> <span class="actor">' + (ev.actor || '') + '</span> ' + (ev.created_at || '') + payload + '</li>';
    });
    html += '</ul></div></div>';
    return html;
  }

  function render(pathResult) {
    const app = document.getElementById('app');
    if (!app) return;
    app.innerHTML = '<p class="loading">Loading…</p>';
    const go = function (html) {
      app.innerHTML = html;
    };
    if (pathResult.view === 'dashboard') {
      renderDashboard().then(go);
    } else if (pathResult.view === 'submit') {
      go(renderSubmit());
      setTimeout(function () {
        const form = document.getElementById('submit-form');
        const msgEl = document.getElementById('submit-msg');
        if (form) {
          form.onsubmit = function (e) {
            e.preventDefault();
            const fd = new FormData(form);
            const body = { account_id: fd.get('account_id'), amount_cents: parseInt(fd.get('amount_cents'), 10) };
            msgEl.innerHTML = '';
            fetch(API_BASE + '/api/v1/deposits', {
              method: 'POST',
              headers: headers(),
              body: JSON.stringify(body),
            }).then(function (r) {
              return r.json().then(function (d) {
                if (r.ok) {
                  msgEl.className = 'form-msg success';
                  msgEl.innerHTML = 'Created: ' + (d.transfer_id || '') + ' — ' + (d.status || '');
                  app.refreshRoute && app.refreshRoute();
                } else {
                  msgEl.className = 'form-msg error';
                  msgEl.innerHTML = (d.message || d.code || r.statusText);
                }
              });
            }).catch(function (e) {
              msgEl.className = 'form-msg error';
              msgEl.innerHTML = e.message || 'Request failed';
            });
          };
        }
      }, 0);
    } else if (pathResult.view === 'operator') {
      renderOperator().then(go);
    } else if (pathResult.view === 'transfer' && pathResult.id) {
      renderTransfer(pathResult.id).then(go);
    } else {
      go('<div class="container"><h1>Not found</h1></div>');
    }
  }

  function refreshRoute() {
    render(parsePath());
  }

  function saveOperatorId() {
    const input = document.getElementById('operator-id');
    if (input) setOperatorId(input.value);
  }

  window.app = {
    refreshRoute: refreshRoute,
    saveOperatorId: saveOperatorId,
  };

  function init() {
    render(parsePath());
    window.addEventListener('popstate', function () { render(parsePath()); });
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
