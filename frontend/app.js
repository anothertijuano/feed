/* feed. — client for the Go backend. All visuals live in styles.css. */
'use strict';

const FEED = document.getElementById('feed');
const SENTINEL = document.getElementById('sentinel');
const TOAST = document.getElementById('toast');
const TOP_REFRESH = document.getElementById('top-refresh');
const PTR = document.getElementById('ptr');
const PTR_LABEL = document.getElementById('ptr-label');
const PTR_SPINNER = document.getElementById('ptr-spinner');

const SAVED_LIST = document.getElementById('saved-list');
const SAVED_EMPTY = document.getElementById('saved-empty');
const SUBS_LIST = document.getElementById('subs-list');
const SUBS_EMPTY = document.getElementById('subs-empty');
const SUBS_FORM = document.getElementById('sub-form');
const SUBS_URL = document.getElementById('sub-url');
const SUBS_REFRESH = document.getElementById('subs-refresh');
const SETTINGS_FORM = document.getElementById('settings-form');
const MEMOS_URL = document.getElementById('memos-url');
const MEMOS_TOKEN = document.getElementById('memos-token');
const MEMO_STATUS = document.getElementById('memo-status');
const PUSH_STATUS = document.getElementById('push-status');
const PUSH_ENABLE = document.getElementById('push-enable');
const PUSH_DISABLE = document.getElementById('push-disable');

const PAGE = 20;

const ICONS = {
  up: ['M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z'],
  down: ['M12 5v14', 'M19 12l-7 7-7-7'],
  save: ['M19 21l-7-4-7 4V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2v16z'],
};

const iconSVG = name =>
  `<svg viewBox="0 0 24 24" aria-hidden="true">` +
  ICONS[name].map(d => `<path d="${d}"/>`).join('') +
  `</svg>`;

/* ---------- api ---------- */

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      const body = await res.json();
      if (body && body.error) msg = body.error;
    } catch (err) {
      /* non-JSON error body */
    }
    throw new Error(msg);
  }
  return res.json();
}

const getJSON = path => api(path);
const postJSON = (path, body) => api(path, { method: 'POST', body: JSON.stringify(body) });
const deleteJSON = path => api(path, { method: 'DELETE' });

/* ---------- card rendering ---------- */

const esc = s =>
  String(s).replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])
  );

function domainOf(url) {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch (err) {
    return '';
  }
}

function hueOf(str) {
  let h = 0;
  for (let i = 0; i < str.length; i++) h = (h * 31 + str.charCodeAt(i)) % 360;
  return h;
}

function actionButton(action, label, on) {
  return (
    `<button type="button" class="act${on ? ' on' : ''}" data-action="${action}"` +
    ` aria-label="${label}" aria-pressed="${on}">${iconSVG(action)}</button>`
  );
}

function mediaHTML(item) {
  return (item.media || [])
    .map(
      m => `
    <div class="media${m.contain ? ' contain' : ''}">
      <img src="${esc(m.src)}" alt="${esc(item.title)}" loading="lazy" decoding="async">
      <a class="media-fallback" href="${esc(m.src)}" target="_blank" rel="noopener noreferrer">View media</a>
    </div>`
    )
    .join('');
}

function buildCard(item) {
  const domain = domainOf(item.link || '');
  const linkAttrs = item.link
    ? `href="${esc(item.link)}" target="_blank" rel="noopener noreferrer"`
    : '';

  const card = document.createElement('article');
  card.className = 'card';
  card.dataset.key = item.id;
  card.innerHTML = `
    <header class="card-head">
      <a class="avatar" ${linkAttrs} style="--h:${hueOf(domain || item.id)}"
         aria-hidden="true" tabindex="-1">${esc((domain || '?').charAt(0).toUpperCase())}</a>
      <a class="source" ${linkAttrs}>${esc(item.sourceName || domain || 'source unknown')}</a>
    </header>
    ${mediaHTML(item)}
    <div class="actions">
      ${actionButton('up', 'Upvote', item.vote === 1)}
      ${actionButton('down', 'Downvote', item.vote === -1)}
      <span class="spacer"></span>
      ${actionButton('save', 'Save', item.saved)}
    </div>
    <div class="card-body">
      <h2 class="title"><a class="title-link" ${linkAttrs}>${esc(item.title)}</a></h2>
      ${item.paragraphs && item.paragraphs.length ? `<div class="text">${item.paragraphs.map(p => `<p>${esc(p)}</p>`).join('')}</div>` : ''}
    </div>`;

  return card;
}

function attachMedia(card) {
  card.querySelectorAll('.media').forEach(media => {
    const img = media.querySelector('img');
    const fallback = media.querySelector('.media-fallback');

    const settle = () => {
      media.classList.add('loaded');
      if (img.naturalWidth === 0) {
        media.classList.add('broken');
        fallback.hidden = false;
      }
    };

    if (img.complete) {
      settle();
    } else {
      img.addEventListener('load', () => media.classList.add('loaded'));
      img.addEventListener('error', () => {
        media.classList.add('loaded', 'broken');
        fallback.hidden = false;
      });
    }
  });
}

function emptyMsg(text) {
  const p = document.createElement('p');
  p.className = 'empty';
  p.textContent = text;
  return p;
}

/* ---------- interactions ---------- */

function setBtn(btn, on) {
  btn.classList.toggle('on', on);
  btn.setAttribute('aria-pressed', String(on));
}

async function sendVote(card, action) {
  const key = card.dataset.key;
  try {
    if (action === 'down') {
      await postJSON('/api/interactions', { key, kind: 'vote', value: -1 });
      card.remove();
      showToast('Content removed');
      return;
    }
    const btn = card.querySelector(`[data-action="${action}"]`);
    const on = btn.classList.contains('on');
    await postJSON('/api/interactions', { key, kind: 'vote', value: on ? 0 : action === 'up' ? 1 : -1 });
    setBtn(btn, !on);
    if (action === 'up') setBtn(card.querySelector('[data-action="down"]'), false);
  } catch (err) {
    showToast('Could not reach backend');
  }
}

async function toggleSave(card) {
  const key = card.dataset.key;
  const btn = card.querySelector('[data-action="save"]');
  const saved = btn.classList.contains('on');
  try {
    await postJSON('/api/interactions', { key, kind: 'save', value: !saved });
    setBtn(btn, !saved);
    showToast(saved ? 'Removed from saved' : 'Saved to your feed');
    if (saved && currentView === 'saved') {
      card.remove();
      updateSavedEmpty();
    }
  } catch (err) {
    showToast('Could not reach backend');
  }
}

document.addEventListener('click', event => {
  const btn = event.target.closest('.act');
  if (!btn) return;
  const card = btn.closest('.card');
  if (!card) return;
  const action = btn.dataset.action;
  if (action === 'save') toggleSave(card);
  else sendVote(card, action);
});

/* Double-tap / double-click a card to like it (Instagram-style). */
document.addEventListener('dblclick', event => {
  if (event.target.closest('a, button, .text')) return;
  const card = event.target.closest('.card');
  if (!card) return;

  spawnBurst(card, event);
  const up = card.querySelector('[data-action="up"]');
  const prev = up.classList.contains('on');
  setBtn(up, !prev);
  setBtn(card.querySelector('[data-action="down"]'), false);
  postJSON('/api/interactions', {
    key: card.dataset.key,
    kind: 'vote',
    value: prev ? 0 : 1,
  }).catch(() => {
    setBtn(up, prev);
    showToast('Could not reach backend');
  });
});

function spawnBurst(card, event) {
  const host = card.querySelector('.media') || card;
  const rect = host.getBoundingClientRect();

  const burst = document.createElement('div');
  burst.className = 'burst';
  burst.style.left = `${event.clientX - rect.left}px`;
  burst.style.top = `${event.clientY - rect.top}px`;
  burst.innerHTML = iconSVG('up');

  host.appendChild(burst);
  burst.addEventListener('animationend', () => burst.remove());
}

/* ---------- feed ---------- */

let feedOffset = 0;
let feedTotal = Infinity;
let feedLoading = false;
let feedLoaded = false;
let feedNotice = null;

function setFeedNotice(el) {
  if (feedNotice) feedNotice.remove();
  feedNotice = el || null;
  if (el) FEED.insertBefore(el, SENTINEL);
}

async function loadFeed(reset) {
  if (feedLoading) return;
  if (reset) {
    feedOffset = 0;
    feedTotal = Infinity;
    feedLoaded = false;
    setFeedNotice(null);
    FEED.querySelectorAll('.card').forEach(el => el.remove());
  }

  feedLoading = true;
  try {
    const data = await getJSON(`/api/feed?limit=${PAGE}&offset=${feedOffset}`);
    feedTotal = data.total;
    const items = data.items || [];
    for (const it of items) {
      const card = buildCard(it);
      attachMedia(card);
      FEED.insertBefore(card, SENTINEL);
    }
    feedOffset += items.length;
    feedLoaded = true;

    if (items.length === 0 && feedOffset === 0) {
      setFeedNotice(emptyMsg('Your feed is empty — add some RSS subscriptions in the Subs tab.'));
    } else if (feedOffset >= feedTotal) {
      setFeedNotice(emptyMsg('You are all caught up.'));
    }
  } catch (err) {
    setFeedNotice(emptyMsg('Could not load the feed — is the backend running?'));
  } finally {
    feedLoading = false;
  }
}

const observer = new IntersectionObserver(entries => {
  if (entries.some(e => e.isIntersecting) && feedLoaded && !feedLoading && feedOffset < feedTotal) {
    loadFeed(false);
  }
}, { rootMargin: '900px 0px' });
observer.observe(SENTINEL);

/* ---------- pull to refresh ---------- */

const PTR_THRESHOLD = 64;
let ptrStartY = null;
let ptrPulling = false;
let ptrRefreshing = false;

/* Fetch every source now, then rebuild the feed with whatever is new. */
async function beginRefresh() {
  if (ptrRefreshing) return;
  ptrRefreshing = true;
  PTR.classList.add('spinning');
  PTR.classList.remove('release', 'pulling');
  PTR.style.height = '56px';
  PTR_LABEL.textContent = 'Refreshing…';

  try {
    const res = await postJSON('/api/refresh', {});
    await loadFeed(true);
    if (currentView === 'feed') window.scrollTo({ top: 0, behavior: 'smooth' });
    const n = res && res.new ? res.new : 0;
    showToast(n > 0 ? `${n} new item${n === 1 ? '' : 's'}` : 'Feed is up to date');
  } catch (err) {
    showToast('Refresh failed');
    try {
      await loadFeed(true);
    } catch (e) {
      /* keep the current list */
    }
  } finally {
    ptrRefreshing = false;
    PTR.classList.remove('spinning');
    PTR.style.height = '0px';
    PTR_LABEL.textContent = 'Pull to refresh';
    PTR_SPINNER.style.transform = '';
  }
}

TOP_REFRESH.addEventListener('click', async () => {
  await beginRefresh();
  if (currentView === 'subs') loadSubs();
});

document.addEventListener('touchstart', e => {
  if (ptrRefreshing || currentView !== 'feed' || window.scrollY > 0) {
    ptrPulling = false;
    return;
  }
  ptrStartY = e.touches[0].clientY;
  ptrPulling = true;
  PTR.classList.add('pulling');
}, { passive: true });

document.addEventListener('touchmove', e => {
  if (!ptrPulling || ptrRefreshing) return;
  const dy = e.touches[0].clientY - ptrStartY;
  if (dy <= 0) {
    PTR.style.height = '0px';
    PTR_SPINNER.style.transform = '';
    return;
  }
  if (window.scrollY > 0) return;
  e.preventDefault();
  const h = Math.min(dy * 0.5, 96);
  PTR.style.height = `${h}px`;
  PTR_SPINNER.style.transform = `rotate(${h * 3}deg)`;
  const ready = h >= PTR_THRESHOLD;
  PTR.classList.toggle('release', ready);
  PTR_LABEL.textContent = ready ? 'Release to refresh' : 'Pull to refresh';
}, { passive: false });

function ptrEnd() {
  if (!ptrPulling) return;
  ptrPulling = false;
  PTR.classList.remove('pulling');
  const h = parseFloat(PTR.style.height) || 0;
  if (h >= PTR_THRESHOLD && !ptrRefreshing) {
    beginRefresh();
  } else if (!ptrRefreshing) {
    PTR.style.height = '0px';
    PTR_LABEL.textContent = 'Pull to refresh';
  }
}

document.addEventListener('touchend', ptrEnd);
document.addEventListener('touchcancel', ptrEnd);

/* ---------- saved ---------- */

async function loadSaved() {
  try {
    const data = await getJSON('/api/saved');
    SAVED_LIST.replaceChildren();
    for (const it of data.items || []) {
      const card = buildCard(it);
      attachMedia(card);
      SAVED_LIST.appendChild(card);
    }
    updateSavedEmpty();
  } catch (err) {
    SAVED_EMPTY.textContent = 'Could not load saved items.';
    SAVED_EMPTY.hidden = false;
  }
}

function updateSavedEmpty() {
  SAVED_EMPTY.textContent = 'Nothing saved yet. Tap the bookmark on any card.';
  SAVED_EMPTY.hidden = SAVED_LIST.children.length > 0;
}

/* ---------- subscriptions ---------- */

function timeAgo(iso) {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (s < 60) return 'just now';
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

function subRow(sub) {
  const row = document.createElement('div');
  row.className = 'sub';
  const bits = [];
  if (sub.itemCount) bits.push(`${sub.itemCount} items`);
  if (sub.lastFetchedAt) bits.push(`fetched ${timeAgo(sub.lastFetchedAt)}`);
  else bits.push('never fetched');
  const meta = sub.lastError ? `error: ${sub.lastError}` : bits.join(' · ');
  row.innerHTML = `
    <div class="sub-info">
      <div class="sub-title">${esc(sub.title || sub.url)}</div>
      <div class="sub-url">${esc(sub.url)}</div>
      <div class="sub-meta${sub.lastError ? ' error' : ''}">${esc(meta)}</div>
    </div>
    <button type="button" class="icon-btn bell" data-bell="${esc(sub.id)}" data-mode="${esc(sub.notify || 'default')}"
            title="${bellTitle(sub.notify)}" aria-label="Notification policy: ${bellTitle(sub.notify)}">${bellIcon(sub.notify)}</button>
    <button type="button" class="icon-btn" data-del="${esc(sub.id)}" aria-label="Remove subscription">
      <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
    </button>`;
  return row;
}

const BELL_MODES = ['default', 'always', 'never'];

function bellTitle(mode) {
  return { default: 'Notify on high rank', always: 'Always notify', never: 'Never notify' }[mode || 'default'];
}

function bellIcon(mode) {
  const bell = `<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>`;
  if (mode === 'never') {
    return `<svg viewBox="0 0 24 24" aria-hidden="true">${bell}<path d="M4 4l16 16"/></svg>`;
  }
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${bell}</svg>`;
}

async function loadSubs() {
  try {
    const data = await getJSON('/api/subscriptions');
    SUBS_LIST.replaceChildren();
    for (const sub of data.items || []) SUBS_LIST.appendChild(subRow(sub));
    SUBS_EMPTY.hidden = SUBS_LIST.children.length > 0;
  } catch (err) {
    SUBS_LIST.replaceChildren(emptyMsg('Could not load subscriptions.'));
  }
}

SUBS_LIST.addEventListener('click', async event => {
  const del = event.target.closest('[data-del]');
  if (del) {
    try {
      await deleteJSON(`/api/subscriptions/${encodeURIComponent(del.dataset.del)}`);
      showToast('Subscription removed');
      loadSubs();
    } catch (err) {
      showToast('Could not remove subscription');
    }
    return;
  }

  const bell = event.target.closest('[data-bell]');
  if (!bell) return;
  const next = BELL_MODES[(BELL_MODES.indexOf(bell.dataset.mode) + 1) % BELL_MODES.length];
  try {
    await postJSON(`/api/subscriptions/${encodeURIComponent(bell.dataset.bell)}`, { notify: next });
    bell.dataset.mode = next;
    bell.innerHTML = bellIcon(next);
    bell.title = bellTitle(next);
    bell.setAttribute('aria-label', `Notification policy: ${bellTitle(next)}`);
    showToast(`Notifications for this source: ${next}`);
  } catch (err) {
    showToast('Could not update notification policy');
  }
});

SUBS_FORM.addEventListener('submit', async event => {
  event.preventDefault();
  const url = SUBS_URL.value.trim();
  if (!url) return;
  const submitBtn = SUBS_FORM.querySelector('button[type="submit"]');
  submitBtn.disabled = true;
  try {
    const sub = await postJSON('/api/subscriptions', { url });
    SUBS_URL.value = '';
    showToast(sub.title ? `Subscribed to ${sub.title}` : 'Subscription added');
    loadSubs();
  } catch (err) {
    showToast(err.message || 'Could not add subscription');
  } finally {
    submitBtn.disabled = false;
  }
});

SUBS_REFRESH.addEventListener('click', async () => {
  await beginRefresh();
  loadSubs();
});

/* ---------- settings ---------- */

function renderMemoStatus(s) {
  if (s.memoLastError) {
    MEMO_STATUS.textContent = `Memos sync error: ${s.memoLastError}`;
    MEMO_STATUS.classList.add('error');
    MEMO_STATUS.hidden = false;
  } else if (s.memoLastSyncAt) {
    MEMO_STATUS.textContent = `Last Memos sync: ${timeAgo(s.memoLastSyncAt)}`;
    MEMO_STATUS.classList.remove('error');
    MEMO_STATUS.hidden = false;
  } else if (s.memosUrl) {
    MEMO_STATUS.textContent = 'Memos configured. Saved items will be mirrored on next save.';
    MEMO_STATUS.classList.remove('error');
    MEMO_STATUS.hidden = false;
  } else {
    MEMO_STATUS.hidden = true;
  }
}

async function loadSettings() {
  try {
    const s = await getJSON('/api/settings');
    MEMOS_URL.value = s.memosUrl || '';
    MEMOS_TOKEN.value = s.memosToken || '';
    renderMemoStatus(s);
  } catch (err) {
    MEMO_STATUS.textContent = 'Could not load settings.';
    MEMO_STATUS.classList.add('error');
    MEMO_STATUS.hidden = false;
  }
}

SETTINGS_FORM.addEventListener('submit', async event => {
  event.preventDefault();
  try {
    const s = await postJSON('/api/settings', {
      memosUrl: MEMOS_URL.value.trim(),
      memosToken: MEMOS_TOKEN.value.trim(),
    });
    renderMemoStatus(s);
    showToast('Settings saved');
  } catch (err) {
    showToast(err.message || 'Could not save settings');
  }
});

/* ---------- push notifications ---------- */

const PUSH_STATE_KEY = 'feed2:push';

function pushSupported() {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
}

function updatePushUI() {
  if (!pushSupported()) {
    PUSH_STATUS.textContent = 'Push notifications are not supported by this browser.';
    PUSH_ENABLE.hidden = true;
    PUSH_DISABLE.hidden = true;
    return;
  }
  if (Notification.permission === 'denied') {
    PUSH_STATUS.textContent = 'Notifications are blocked. Allow them in the browser\u2019s site settings.';
    PUSH_ENABLE.hidden = true;
    PUSH_DISABLE.hidden = true;
    return;
  }
  if (localStorage.getItem(PUSH_STATE_KEY)) {
    PUSH_STATUS.textContent = 'Notifications are enabled.';
    PUSH_ENABLE.hidden = true;
    PUSH_DISABLE.hidden = false;
  } else {
    PUSH_STATUS.textContent = 'Notifications are currently off.';
    PUSH_ENABLE.hidden = false;
    PUSH_DISABLE.hidden = true;
  }
}

function b64url(buf) {
  const bytes = new Uint8Array(buf);
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function urlB64ToUint8Array(s) {
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/');
  const padded = b64.padEnd(b64.length + ((4 - (b64.length % 4)) % 4), '=');
  const raw = atob(padded);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

async function enablePush() {
  try {
    const permission = await Notification.requestPermission();
    if (permission !== 'granted') {
      updatePushUI();
      showToast('Notification permission denied');
      return;
    }
    const reg = await navigator.serviceWorker.ready;
    const key = await getJSON('/api/push/key');
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlB64ToUint8Array(key.publicKey),
    });
    await postJSON('/api/push/subscribe', {
      endpoint: sub.endpoint,
      keys: { p256dh: b64url(sub.getKey('p256dh')), auth: b64url(sub.getKey('auth')) },
    });
    localStorage.setItem(PUSH_STATE_KEY, JSON.stringify({ endpoint: sub.endpoint }));
    updatePushUI();
    showToast('Notifications enabled');
  } catch (err) {
    showToast(`Could not enable notifications: ${err.message || err}`);
  }
}

async function disablePush() {
  try {
    const stored = JSON.parse(localStorage.getItem(PUSH_STATE_KEY) || '{}');
    if (stored.endpoint) {
      await api('/api/push/unsubscribe', {
        method: 'DELETE',
        body: JSON.stringify({ endpoint: stored.endpoint }),
      });
    }
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    if (sub) await sub.unsubscribe();
    localStorage.removeItem(PUSH_STATE_KEY);
    updatePushUI();
    showToast('Notifications disabled');
  } catch (err) {
    showToast('Could not disable notifications');
  }
}

PUSH_ENABLE.addEventListener('click', enablePush);
PUSH_DISABLE.addEventListener('click', disablePush);

/* ---------- toast ---------- */

let toastTimer = 0;

function showToast(message) {
  TOAST.textContent = message;
  TOAST.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => TOAST.classList.remove('show'), 1800);
}

/* ---------- router ---------- */

const VIEWS = ['feed', 'saved', 'subs', 'settings'];
let currentView = 'feed';

function route() {
  const hash = location.hash.replace(/^#\/?/, '');
  const view = VIEWS.includes(hash) ? hash : 'feed';
  currentView = view;

  VIEWS.forEach(v =>
    document.getElementById(`view-${v}`).classList.toggle('active', v === view)
  );
  document.querySelectorAll('.tab').forEach(t =>
    t.classList.toggle('active', t.dataset.view === view)
  );

  if (view === 'saved') loadSaved();
  else if (view === 'subs') loadSubs();
  else if (view === 'settings') {
    loadSettings();
    updatePushUI();
  }

  window.scrollTo({ top: 0 });
}

window.addEventListener('hashchange', route);

/* ---------- init ---------- */

function init() {
  // Register the service worker (PWA shell + push). Only meaningful over
  // https or on localhost.
  if ('serviceWorker' in navigator && (location.protocol === 'https:' || location.hostname === 'localhost' || location.hostname === '127.0.0.1')) {
    navigator.serviceWorker.register('/sw.js').catch(err =>
      console.debug('[feed] service worker registration failed', err)
    );
  }
  route();
  loadFeed(false);
}

init();
