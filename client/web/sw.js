const CACHE_NAME = 'malten-v__VERSION__';

// Only cache the bare minimum for offline shell
const STATIC_ASSETS = [
  '/icon-192.png',
  '/icon-512.png',
  '/logo.jpg',
  '/favicon.ico'
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(STATIC_ASSETS))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(keys => {
      return Promise.all(
        keys.filter(key => key !== CACHE_NAME)
            .map(key => caches.delete(key))
      );
    }).then(() => self.clients.claim())
  );
});

// Network first for everything - no stale JS/CSS
self.addEventListener('fetch', event => {
  // Skip WebSocket
  if (event.request.url.includes('/events')) return;
  
  event.respondWith(
    fetch(event.request)
      .catch(() => caches.match(event.request))
  );
});
