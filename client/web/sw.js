const CACHE_NAME = 'malten-v9';
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/malten.css',
  '/malten.js',
  '/manifest.webmanifest',
  '/icon-192.png',
  '/icon-512.png',
  '/logo.jpg',
  '/favicon.ico'
];

// Install - cache static assets
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(STATIC_ASSETS))
      .then(() => self.skipWaiting())
  );
});

// Activate - clean old caches
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

// Fetch - network first for API, cache first for static
self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  
  // Skip WebSocket and API requests
  if (url.pathname.startsWith('/events') ||
      url.pathname.startsWith('/messages') ||
      url.pathname.startsWith('/commands') ||
      url.pathname.startsWith('/streams') ||
      url.pathname.startsWith('/ping')) {
    return;
  }
  
  // Static assets - cache first
  event.respondWith(
    caches.match(event.request)
      .then(cached => {
        if (cached) {
          // Return cached, update cache in background
          fetch(event.request)
            .then(response => {
              if (response.ok) {
                caches.open(CACHE_NAME)
                  .then(cache => cache.put(event.request, response));
              }
            })
            .catch(() => {});
          return cached;
        }
        // Not cached - fetch and cache
        return fetch(event.request)
          .then(response => {
            if (response.ok) {
              const clone = response.clone();
              caches.open(CACHE_NAME)
                .then(cache => cache.put(event.request, clone));
            }
            return response;
          });
      })
  );
});
