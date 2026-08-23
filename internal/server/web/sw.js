// Malten service worker. It makes the app installable and keeps a copy of the
// shell for offline launches — but it is NETWORK-FIRST for everything it serves,
// so a fresh deploy shows up on the very next launch instead of lagging a
// version behind. The cache is only a fallback for when the network is down or
// slow. API traffic is never cached.
//
// Bump VERSION whenever the shell changes; it wipes the old cache on activate.
const VERSION = 'malten-v51';
const SHELL = [
  '/',
  '/app.css', '/app.js', '/leaflet.js', '/leaflet.css', '/manifest.webmanifest',
  '/icon-192.png', '/icon-512.png', '/icon-maskable-512.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(VERSION).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

// Race the network against a short timeout, so a hung request never stops the
// app from opening — we fall back to cache instead.
function fetchWithTimeout(req, ms) {
  return new Promise((resolve, reject) => {
    const ctrl = new AbortController();
    const timer = setTimeout(() => { ctrl.abort(); reject(new Error('timeout')); }, ms);
    fetch(req, { signal: ctrl.signal }).then(
      (res) => { clearTimeout(timer); resolve(res); },
      (err) => { clearTimeout(timer); reject(err); }
    );
  });
}

async function networkFirst(req) {
  const cache = await caches.open(VERSION);
  try {
    const res = await fetchWithTimeout(req, 3500);
    if (res && res.ok) cache.put(req, res.clone()); // refresh the offline copy
    return res;
  } catch (_) {
    const cached = await cache.match(req);
    if (cached) return cached;
    if (req.mode === 'navigate') {
      const shell = await cache.match('/');
      if (shell) return shell;
    }
    throw _;
  }
}

// A nudge from the server: one notification, tapping it opens the app. The
// payload is encrypted end-to-end — the push service relayed it without being
// able to read it.
self.addEventListener('push', (event) => {
  let data = {};
  try { data = event.data ? event.data.json() : {}; } catch (_) { data = { body: event.data && event.data.text() }; }
  if (!data.body) return;
  event.waitUntil(self.registration.showNotification(data.title || 'Malten', {
    body: data.body,
    icon: '/icon-192.png',
    badge: '/icon-192.png',
    tag: 'malten-nudge',   // a newer nudge replaces an unread one
    renotify: false,
    data: { url: data.url || '/' },
  }));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';
  event.waitUntil((async () => {
    const all = await clients.matchAll({ type: 'window', includeUncontrolled: true });
    for (const c of all) {
      if (c.url.includes(self.location.origin)) return c.focus();
    }
    return clients.openWindow(url);
  })());
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  // API responses (and tile proxy) are dynamic — always live, never cached.
  if (url.pathname.startsWith('/api/')) return;

  // Everything we serve: fresh when online, cached only as a fallback.
  event.respondWith(networkFirst(req));
});
