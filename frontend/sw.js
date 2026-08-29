/* feed. — service worker: app-shell caching + push notifications. */
'use strict';

const VERSION = 'v3';
const CACHE = `feed-${VERSION}`;
const SHELL = ['/', '/index.html', '/styles.css', '/app.js', '/manifest.json',
  '/icons/icon-192.png', '/icons/icon-512.png', '/icons/apple-touch-icon.png',
  '/icons/favicon.svg', '/icons/favicon.ico'];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE).then(cache => cache.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(keys.filter(k => k !== CACHE).map(k => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', event => {
  const req = event.request;
  if (req.method !== 'GET' || !req.url.startsWith(self.location.origin)) return;

  // API and content are always live.
  if (req.url.includes('/api/')) {
    event.respondWith(fetch(req).catch(() => new Response('offline', { status: 503 })));
    return;
  }

  // Navigations: network first, fall back to the cached shell.
  if (req.mode === 'navigate') {
    event.respondWith(
      fetch(req).catch(() => caches.match('/index.html'))
    );
    return;
  }

  // Static assets: stale-while-revalidate.
  event.respondWith(
    caches.match(req).then(cached => {
      const fetched = fetch(req)
        .then(res => {
          if (res && res.ok) {
            const clone = res.clone();
            caches.open(CACHE).then(cache => cache.put(req, clone));
          }
          return res;
        })
        .catch(() => cached);
      return cached || fetched;
    })
  );
});

self.addEventListener('push', event => {
  let data = { title: 'feed.' };
  try {
    if (event.data) data = event.data.json();
  } catch (err) {
    data = { title: 'feed.', body: event.data ? event.data.text() : '' };
  }

  const options = {
    body: data.body || '',
    icon: '/icons/icon-192.png',
    badge: '/icons/icon-192.png',
    data: { url: data.url || '/' },
  };
  event.waitUntil(self.registration.showNotification(data.title || 'feed.', options));
});

self.addEventListener('notificationclick', event => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then(list => {
      for (const client of list) {
        if ('focus' in client) {
          client.navigate(url);
          return client.focus();
        }
      }
      return self.clients.openWindow(url);
    })
  );
});
