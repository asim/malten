// Malten Service Worker - Push Notifications
const CACHE_NAME = 'malten-v1';

// Install event
self.addEventListener('install', function(event) {
    self.skipWaiting();
});

// Activate event
self.addEventListener('activate', function(event) {
    event.waitUntil(clients.claim());
});

// Push event - receive push notifications
self.addEventListener('push', function(event) {
    if (!event.data) return;

    var data;
    try {
        data = event.data.json();
    } catch (e) {
        data = { title: 'Malten', body: event.data.text() };
    }

    var options = {
        body: data.body || '',
        icon: data.icon || '/icon-192.png',
        badge: '/icon-192.png',
        tag: data.tag || 'malten-update',
        renotify: true,
        data: data.data || {}
    };

    event.waitUntil(
        self.registration.showNotification(data.title || 'Malten', options)
    );
});

// Notification click - open app
self.addEventListener('notificationclick', function(event) {
    event.notification.close();

    event.waitUntil(
        clients.matchAll({ type: 'window', includeUncontrolled: true }).then(function(clientList) {
            // Focus existing window if open
            for (var i = 0; i < clientList.length; i++) {
                var client = clientList[i];
                if (client.url.includes('malten') && 'focus' in client) {
                    return client.focus();
                }
            }
            // Otherwise open new window
            if (clients.openWindow) {
                return clients.openWindow('/');
            }
        })
    );
});
