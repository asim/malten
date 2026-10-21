// Only cache the personal app shell. Never cache stream messages, commands or events.
const CACHE = 'malten-personal-shell-v7';
const SHELL = ['/private', '/composer.js', '/timeline.css', '/timeline.js', '/install.js', '/manifest.webmanifest', '/icon-192.png', '/icon-512.png', '/apple-touch-icon.png'];
self.addEventListener('install', event => { event.waitUntil(caches.open(CACHE).then(cache => cache.addAll(SHELL))); });
self.addEventListener('activate', event => {
  event.waitUntil(caches.keys().then(keys => Promise.all(keys.filter(key => key.startsWith('malten-personal-shell-') && key !== CACHE).map(key => caches.delete(key)))).then(() => self.clients.claim()));
});
self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  if (event.request.mode === 'navigate' && url.origin === self.location.origin && (url.pathname === '/' || url.pathname === '/streams.html')) {
    event.respondWith(fetch(event.request, {cache:'no-store'}).catch(() => new Response('<!doctype html><meta name=viewport content="width=device-width,initial-scale=1"><title>Malten</title><p>Public streams need a connection.</p><a href="/private">For me</a>', {headers:{'Content-Type':'text/html'}})));
    return;
  }
  if (event.request.method !== 'GET' || url.origin !== self.location.origin || !SHELL.includes(url.pathname) || url.search) return;
  event.respondWith(fetch(event.request).then(response => {
    if (response.ok) { const copy = response.clone(); event.waitUntil(caches.open(CACHE).then(cache => cache.put(event.request, copy))); }
    return response;
  }).catch(() => caches.match(event.request)));
});
