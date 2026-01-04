/*
 * ARCHITECTURE (read ARCHITECTURE.md and claude.md "The Spacetime Model"):
 *
 * localStorage = your worldline (YOUR private timeline through spacetime)
 * WebSocket    = real-time events from the stream (public spatial updates)
 * 
 * YOUR timeline (cards, conversations) lives in localStorage.
 * Server streams are for real-time spatial events, not persistence.
 * On refresh, localStorage is YOUR source of truth.
 *
 * TIMELINE FUNCTIONS:
 *   addToTimeline(text, type)  - Save + render (the ONE way to add anything)
 *   loadTimeline()             - Load from localStorage on startup
 *   renderTimelineItem(item)   - Render single item to DOM (internal)
 */

// FORCE KILL SERVICE WORKER AND CACHE - version 85
(function() {
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.getRegistrations().then(function(regs) {
            regs.forEach(function(r) { r.unregister(); });
        });
    }
    if ('caches' in window) {
        caches.keys().then(function(names) {
            names.forEach(function(name) { caches.delete(name); });
        });
    }
})();

var commandUrl = "/commands";
var messageUrl = "/messages";

var eventUrl = "/events";
var limit = 25;

// Debug logging to screen (enable with /debug on)
window.debugMode = false;
function debugLog(msg) {
    console.log('[debug]', msg);
    if (window.debugMode) {
        addToTimeline('🔧 ' + msg);
    }
}

// Enable credentials for all jQuery AJAX requests (needed for session cookies)
$.ajaxSetup({
    xhrFields: { withCredentials: true },
    crossDomain: true
});
var locationWatchId = null;
var last = timeAgo();

var maxMessages = 1000;
var seen = {};

var ws = null;
var currentStream = null;
var reconnectTimer = null;
var pendingMessages = {};
var isAcquiringLocation = false;
var lastAcquiringShown = 0;

// Set acquiring state - always shows in timeline
function setAcquiring(acquiring) {
    if (acquiring && !isAcquiringLocation) {
        // Only show in timeline if not shown in last 30 seconds
        var now = Date.now();
        if (now - lastAcquiringShown > 30000) {
            addToTimeline('📡 Acquiring location...');
            lastAcquiringShown = now;
        }
    }
    isAcquiringLocation = acquiring;
    updateAcquiringIndicator();
}

// Geohash for stream ID from location
function geohash(lat, lon, precision) {
    var base32 = '0123456789bcdefghjkmnpqrstuvwxyz';
    var minLat = -90, maxLat = 90;
    var minLon = -180, maxLon = 180;
    var hash = '';
    var bit = 0;
    var ch = 0;
    var even = true;
    
    while (hash.length < precision) {
        if (even) {
            var mid = (minLon + maxLon) / 2;
            if (lon >= mid) {
                ch |= 1 << (4 - bit);
                minLon = mid;
            } else {
                maxLon = mid;
            }
        } else {
            var mid = (minLat + maxLat) / 2;
            if (lat >= mid) {
                ch |= 1 << (4 - bit);
                minLat = mid;
            } else {
                maxLat = mid;
            }
        }
        even = !even;
        bit++;
        if (bit === 5) {
            hash += base32[ch];
            bit = 0;
            ch = 0;
        }
    }
    return hash;
}

// Consolidated state management
var state = {
    version: 3, // Increment to clear old state on format change (v3: JSON context)
    load: function() {
        try {
            var saved = localStorage.getItem('malten_state');
            if (saved) {
                var s = JSON.parse(saved);
                // Clear cards if version mismatch, keep important data
                if (s.version !== this.version) {
                    this.lat = s.lat || null;
                    this.lon = s.lon || null;
                    this.cards = [];
                    this.conversation = s.conversation || null;
                    this.savedPlaces = s.savedPlaces || {};  // Preserve saved places
                    this.steps = s.steps || { count: 0, date: null };  // Preserve steps
                    this.save();
                    return;
                }
                this.lat = s.lat || null;
                this.lon = s.lon || null;
                this.context = s.context || null;
                this.contextTime = s.contextTime || 0;
                this.contextExpanded = s.contextExpanded || false;
                this.locationHistory = s.locationHistory || [];
                this.lastBusStop = s.lastBusStop || null;
                this.cards = s.cards || [];
                this.seenNewsUrls = s.seenNewsUrls || [];
                this.conversation = s.conversation || null;
                this.checkedIn = s.checkedIn || null;
                this.savedPlaces = s.savedPlaces || {};
                this.steps = s.steps || { count: 0, date: null };
                this.reminderDate = s.reminderDate || null;
                this.prayerReminders = s.prayerReminders || {};
                // Prune old cards on load (24 hour retention)
                var cutoff = Date.now() - (24 * 60 * 60 * 1000);
                this.cards = this.cards.filter(function(c) { return c.time > cutoff; });
                // Prune old news URLs (7 day retention)
                var newsCutoff = Date.now() - (7 * 24 * 60 * 60 * 1000);
                this.seenNewsUrls = this.seenNewsUrls.filter(function(n) { return n.time > newsCutoff; });
                // Prune old conversations (24 hour retention)
                if (this.conversation && this.conversation.time) {
                    if (this.conversation.time < cutoff) {
                        this.conversation = null;
                    }
                }
            }
        } catch(e) {}
    },
    save: function() {
        localStorage.setItem('malten_state', JSON.stringify({
            version: this.version,
            lat: this.lat,
            lon: this.lon,
            context: this.context,
            contextTime: this.contextTime,
            contextExpanded: this.contextExpanded,
            locationHistory: this.locationHistory.slice(-20),
            lastBusStop: this.lastBusStop,
            cards: this.cards,
            seenNewsUrls: this.seenNewsUrls,
            conversation: this.conversation,
            checkedIn: this.checkedIn,
            savedPlaces: this.savedPlaces,
            steps: this.steps,
            reminderDate: this.reminderDate,
            prayerReminders: this.prayerReminders
        }));
    },
    hasSeenNews: function(newsText) {
        // Extract URL from news text
        var urlMatch = newsText.match(/https?:\/\/[^\s]+/);
        if (!urlMatch) return false;
        var url = urlMatch[0];
        for (var i = 0; i < this.seenNewsUrls.length; i++) {
            if (this.seenNewsUrls[i].url === url) return true;
        }
        return false;
    },
    markNewsSeen: function(newsText) {
        var urlMatch = newsText.match(/https?:\/\/[^\s]+/);
        if (!urlMatch) return;
        var url = urlMatch[0];
        this.seenNewsUrls.push({ url: url, time: Date.now() });
        // Keep only last 50 URLs
        if (this.seenNewsUrls.length > 50) {
            this.seenNewsUrls = this.seenNewsUrls.slice(-50);
        }
        this.save();
    },
    setLocation: function(lat, lon) {
        var prevLat = this.lat;
        var prevLon = this.lon;
        this.lat = lat;
        this.lon = lon;
        
        // Track location history for movement detection
        this.locationHistory.push({
            lat: lat, lon: lon, time: Date.now()
        });
        if (this.locationHistory.length > 20) {
            this.locationHistory.shift();
        }
        this.save();
    },
    setContext: function(ctx) {
        // ctx can be JSON string or object
        if (typeof ctx === 'string') {
            try {
                ctx = JSON.parse(ctx);
            } catch(e) {
                // Legacy text format - wrap it
                ctx = { html: ctx, places: {} };
            }
        }
        
        this.context = ctx;
        this.contextTime = Date.now();
        this.save();
        
        // Server pushes meaningful updates via websocket
        // Client just stores context for display
    },
    extractLocation: function(ctx) {
        if (ctx && ctx.location && ctx.location.name) {
            return ctx.location.name;
        }
        var html = (typeof ctx === 'string') ? ctx : (ctx && ctx.html) || '';
        var match = html.match(/📍 ([^\n]+)/);
        return match ? match[1].trim() : null;
    },
    createQACard: function(question, answer) {
        var card = {
            question: question,
            answer: answer,
            time: Date.now(),
            lat: this.lat,
            lon: this.lon
        };
        this.cards.push(card);
        // Prune cards older than 24 hours
        var cutoff = Date.now() - (24 * 60 * 60 * 1000);
        this.cards = this.cards.filter(function(c) { return c.time > cutoff; });
        this.save();
    },
    isMoving: function() {
        if (this.locationHistory.length < 3) return false;
        var recent = this.locationHistory.slice(-3);
        var totalDist = 0;
        for (var i = 1; i < recent.length; i++) {
            totalDist += this.distance(recent[i-1], recent[i]);
        }
        return totalDist > 0.02; // Moving if traveled >20m in recent updates
    },
    distance: function(a, b) {
        // Haversine distance in km
        var R = 6371;
        var dLat = (b.lat - a.lat) * Math.PI / 180;
        var dLon = (b.lon - a.lon) * Math.PI / 180;
        var lat1 = a.lat * Math.PI / 180;
        var lat2 = b.lat * Math.PI / 180;
        var x = Math.sin(dLat/2) * Math.sin(dLat/2) +
                Math.sin(dLon/2) * Math.sin(dLon/2) * Math.cos(lat1) * Math.cos(lat2);
        return R * 2 * Math.atan2(Math.sqrt(x), Math.sqrt(1-x));
    },
    hasLocation: function() {
        return this.lat && this.lon;
    },
    lat: null,
    lon: null,
    context: null,
    contextTime: 0,
    contextExpanded: false,
    locationHistory: [],
    lastBusStop: null,
    cards: [],
    seenNewsUrls: [],
    checkedIn: null,  // {name, lat, lon, time} - manual location override
    savedPlaces: {},  // Private named places: { "Home": {lat, lon}, "Work": {lat, lon} }
    steps: { count: 0, date: null },  // Daily step counter
    reminderDate: null,  // Last date daily reminder was shown (YYYY-MM-DD)
    prayerReminders: {},  // Track which prayer reminders shown today: {fajr: '2026-01-04', ...}
    motionDetected: false,  // Movement detected via accelerometer while GPS stuck
    
    // Check if user has manually checked in to a location
    isCheckedIn: function() {
        if (!this.checkedIn) return false;
        // Check-in expires after 2 hours
        if (Date.now() - this.checkedIn.time > 2 * 60 * 60 * 1000) {
            this.checkedIn = null;
            this.save();
            return false;
        }
        return true;
    },
    
    // Check in to a specific location
    checkIn: function(name, lat, lon) {
        this.checkedIn = {
            name: name,
            lat: lat,
            lon: lon,
            time: Date.now()
        };
        this.save();
        // Don't add to timeline here - callers handle it
    },
    
    // Clear check-in (when GPS moves significantly)
    clearCheckIn: function() {
        if (this.checkedIn) {
            this.checkedIn = null;
            this.save();
        }
    },
    
    // Check if GPS appears stuck (same position for 5+ min)
    // Haversine distance in meters
    haversine: function(lat1, lon1, lat2, lon2) {
        var R = 6371000;
        var dLat = (lat2 - lat1) * Math.PI / 180;
        var dLon = (lon2 - lon1) * Math.PI / 180;
        var a = Math.sin(dLat/2) * Math.sin(dLat/2) +
                Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
                Math.sin(dLon/2) * Math.sin(dLon/2);
        var c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a));
        return R * c;
    },
    
    // Get effective location (checked-in or GPS)
    getEffectiveLocation: function() {
        if (this.isCheckedIn()) {
            return { lat: this.checkedIn.lat, lon: this.checkedIn.lon, name: this.checkedIn.name, isCheckedIn: true };
        }
        return { lat: this.lat, lon: this.lon, name: null, isCheckedIn: false };
    }
};
state.load();

// =============================================================================
// =============================================================================
// TIMELINE - Your worldline through spacetime
// =============================================================================
//
// EVERYTHING goes through addToTimeline(). No exceptions.
//
// - Location updates? addToTimeline()
// - Directions? addToTimeline()
// - Errors? addToTimeline()
// - AI responses? addToTimeline()
// - Status changes? addToTimeline()
//
// NEVER:
// - document.createElement() and append to #messages directly
// - innerHTML on #messages
// - Any other way of showing content to the user
//
// Why: addToTimeline() saves to localStorage. Direct DOM = lost on reload.
// =============================================================================

// THE way to add anything to the timeline (saves + renders)
function addToTimeline(text, type) {
    if (!text) return;
    
    // Augment check-in prompts with saved places
    if (text.indexOf('Where are you?') >= 0) {
        text = augmentCheckInPrompt(text);
    }
    
    var item = {
        text: text,
        type: type || getTimelineType(text),
        time: Date.now(),
        lat: state.lat,
        lon: state.lon
    };
    
    // Don't persist transient status messages
    var isTransient = text.indexOf('Acquiring location') >= 0;
    
    if (!isTransient) {
        // Save to state
        state.cards.push(item);
        
        // Prune old items (24 hour retention)
        var cutoff = Date.now() - (24 * 60 * 60 * 1000);
        state.cards = state.cards.filter(function(c) { return c.time > cutoff; });
        state.save();
    }
    
    // Render
    renderTimelineItem(item);
    scrollToBottom();
}

// Load timeline from localStorage on startup
function loadTimeline() {
    if (!state.cards || !state.cards.length) return;
    
    // Sort oldest first
    var sorted = state.cards.slice().sort(function(a, b) { return a.time - b.time; });
    
    sorted.forEach(function(item) {
        if (item.text) {
            renderTimelineItem(item);
        }
    });
}

// Render single item to DOM
function renderTimelineItem(item) {
    var type = item.type || getTimelineType(item.text);
    var time = item.time || Date.now();
    
    var li = document.createElement('li');
    var html = makeCheckInClickable(item.text).replace(/\n/g, '<br>');
    
    li.innerHTML = '<div class="card card-' + type + '" data-timestamp="' + time + '">' +
        '<span class="card-time">' + formatTimeAgo(time) + '</span>' +
        html +
        '</div>';
    
    // Insert in chronological order
    var messages = document.getElementById('messages');
    var cards = messages.querySelectorAll('.card');
    var inserted = false;
    
    for (var i = 0; i < cards.length; i++) {
        var cardTime = parseInt(cards[i].getAttribute('data-timestamp')) || 0;
        if (time < cardTime) {
            messages.insertBefore(li, cards[i].parentElement);
            inserted = true;
            break;
        }
    }
    
    if (!inserted) {
        messages.appendChild(li);
    }
}

// Determine item type from text
function getTimelineType(text) {
    if (!text) return 'default';
    if (text.indexOf('🚶') >= 0 || text.indexOf('🚗') >= 0 || text.indexOf('📍 Entered') >= 0) return 'movement';
    if (text.indexOf('🚏') >= 0 || text.indexOf('🚌') >= 0) return 'transport';
    if (text.indexOf('🌧️') >= 0 || text.indexOf('☀️') >= 0 || text.indexOf('⛅') >= 0) return 'weather';
    if (text.indexOf('🕌') >= 0) return 'prayer';
    if (text.indexOf('📍') >= 0) return 'location';
    if (text.indexOf('📖') >= 0 || text.indexOf('💿') >= 0) return 'reminder';
    return 'default';
}

// =============================================================================

String.prototype.parseURL = function() {
    // Match URLs including @, commas, %, etc
    return this.replace(/https?:\/\/[A-Za-z0-9-_.]+\.[A-Za-z0-9-_:%&~\?\/.=#,@+]+/g, function(url) {
        var cleanUrl = url.replace(/&amp;/g, '&');
        return '<a href="' + cleanUrl + '" target="_blank">Map</a>';
    });
};

String.prototype.parseHashTag = function() {
    // Require at least one letter after # to avoid matching URL fragments like #127
    return this.replace(/#[A-Za-z~][A-Za-z0-9-_~]*/g, function(t) {
        var url = location.protocol + '//' + location.hostname + (location.port ? ':' + location.port : '');
        return t.link(url + '/' + t);
    });
};

function timeAgo() {
    var ts = new Date().getTime() / 1000;
    return (ts - 86400) * 1e9;
}

// Timeago format - converts timestamp to "2 min ago", "1 hour ago", etc.
function formatTimeAgo(timestamp) {
    var now = Date.now();
    var diff = now - timestamp;
    
    if (diff < 60000) return 'Just now';
    if (diff < 3600000) {
        var mins = Math.floor(diff / 60000);
        return mins + ' min' + (mins > 1 ? 's' : '') + ' ago';
    }
    if (diff < 86400000) {
        var hours = Math.floor(diff / 3600000);
        return hours + ' hour' + (hours > 1 ? 's' : '') + ' ago';
    }
    if (diff < 604800000) {
        var days = Math.floor(diff / 86400000);
        return days + ' day' + (days > 1 ? 's' : '') + ' ago';
    }
    // Older than a week - show date
    return new Date(timestamp).toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function getStream() {
    // If we have location, use geohash stream
    if (state.hasLocation()) {
        return geohash(state.lat, state.lon, 6);
    }
    // Fallback to URL hash or default
    var stream = window.location.hash.replace('#', '');
    return stream.length > 0 ? stream : "~";
}

function escapeHTML(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML.replace(/(?:\r\n|\r|\n)/g, '<br>');
}



function clearMessages() {
    document.getElementById('messages').innerHTML = "";
    last = timeAgo();
    seen = {};
}

function clipMessages() {
    var list = document.getElementById('messages');
    while (list.children.length > maxMessages) {
        list.removeChild(list.lastChild);
    }
}

function displayMessages(array, direction) {
    // Display oldest first so newest ends up on top
    var sorted = array.slice().sort(function(a, b) {
        return a.Created - b.Created;
    });
    
    for (var i = 0; i < sorted.length; i++) {
        if (sorted[i].Id in seen) continue;
        seen[sorted[i].Id] = sorted[i];
        
        // Use card format with timestamp from message
        var timestamp = sorted[i].Created / 1e6; // Convert from nanos to millis
        addToTimeline(sorted[i].Text, timestamp);
    }

    if (direction >= 0 && array.length > 0) {
        last = array[array.length - 1].Created;
    }
}


function loadMore() {
    var divs = document.getElementsByClassName('time');
    var oldest = new Date().getTime() * 1e6;
    if (divs.length > 0) {
        oldest = divs[divs.length - 1].getAttribute('data-time');
    }

    var stream = getStream();
    var params = "?direction=-1&limit=" + limit + "&last=" + oldest + "&stream=" + stream;

    $.get(messageUrl + params, function(data) {
        if (data && data.length > 0) {
            displayMessages(data, -1);
        }
    });
}

function connectWebSocket() {
    var stream = getStream();
    
    // Don't reconnect if same stream
    if (ws && ws.readyState === WebSocket.OPEN && currentStream === stream) {
        return;
    }

    // Close existing connection
    if (ws) {
        ws.onclose = null; // Prevent reconnect on intentional close
        ws.close();
    }

    currentStream = stream;
    var url = window.location.origin.replace("http", "ws") + eventUrl + "?stream=" + stream;
    ws = new WebSocket(url);

    ws.onopen = function() {
        console.log("WebSocket connected to", stream);
        if (reconnectTimer) {
            clearTimeout(reconnectTimer);
            reconnectTimer = null;
        }
    };

    ws.onmessage = function(event) {
        if (!event.data) return;
        
        var ev = JSON.parse(event.data);
        if (ev.Stream !== currentStream) return;
        
        if (ev.Type === "message") {
            // Dedupe
            if (ev.Id in seen) return;
            seen[ev.Id] = ev;
            
            // Skip if it's our own message (already shown)
            if (pendingMessages[ev.Text]) {
                delete pendingMessages[ev.Text];
                return;
            }
            
            // Show response as a card
            hideLoading();
            if (!ev.Text || ev.Text.trim() === '') return; // Skip empty messages
            if (pendingCommand) {
                displayResponse(ev.Text);
            } else {
                addToTimeline(ev.Text);
            }
            clipMessages();
        }
    };

    ws.onclose = function() {
        console.log("WebSocket closed");
        // Reconnect after delay if not intentional
        if (!reconnectTimer) {
            reconnectTimer = setTimeout(function() {
                reconnectTimer = null;
                connectWebSocket();
            }, 3000);
        }
    };

    ws.onerror = function(err) {
        console.log("WebSocket error", err);
    };
}

function initialLoad() {
    clearMessages();
    // User's timeline comes from localStorage (their worldline)
    // Server messages are for real-time stream events, not persistence
    // Per spacetime model: private experiences belong in localStorage
    connectWebSocket();
    
    var form = document.getElementById('form');
    form.elements["stream"].value = getStream();
    form.elements["prompt"].focus();
}

function submitCommand() {
    hideCommandPalette();
    
    var form = document.getElementById('form');
    var prompt = form.elements["prompt"].value.trim();
    
    debugLog('submitCommand: ' + prompt);
    
    if (prompt.length === 0) return false;

    // ========================================
    // CLIENT-ONLY COMMANDS (browser/localStorage)
    // ========================================
    
    // Handle refresh command - force reload latest version
    if (prompt.match(/^\/?refresh$/i)) {
        form.elements["prompt"].value = '';
        // Unregister service worker to force fresh fetch
        if ('serviceWorker' in navigator) {
            navigator.serviceWorker.getRegistrations().then(function(registrations) {
                registrations.forEach(function(r) { r.unregister(); });
            });
        }
        addToTimeline('🔄 Refreshing...');
        setTimeout(function() { location.reload(true); }, 500);
        return false;
    }
    
    // Handle clear command - reset all local state
    if (prompt.match(/^\/?clear$/i)) {
        form.elements["prompt"].value = '';
        localStorage.clear();
        // Unregister service worker
        if ('serviceWorker' in navigator) {
            navigator.serviceWorker.getRegistrations().then(function(registrations) {
                registrations.forEach(function(r) { r.unregister(); });
            });
        }
        // Show feedback then reload
        addToTimeline('🗑️ Cleared all local data. Reloading...');
        setTimeout(function() { location.reload(); }, 1000);
        return false;
    }
    
    // Handle save command - save current location as named place
    var saveMatch = prompt.match(/^\/?save\s+(.+)$/i);
    if (saveMatch) {
        form.elements["prompt"].value = '';
        var placeName = saveMatch[1].trim();
        if (!state.hasLocation()) {
            addToTimeline('❌ Need location to save a place');
            return false;
        }
        state.savedPlaces[placeName] = { lat: state.lat, lon: state.lon };
        state.save();
        addToTimeline('📍 Saved "' + placeName + '"');
        return false;
    }
    
    // Handle places command - list saved places
    if (prompt.match(/^\/?places$/i)) {
        debugLog('places matched');
        form.elements["prompt"].value = '';
        var names = Object.keys(state.savedPlaces || {});
        debugLog('savedPlaces: ' + names.join(', '));
        if (names.length === 0) {
            addToTimeline('📍 No saved places.\nUse /save Home to save current location.');
        } else {
            var msg = '📍 Saved places:\n';
            names.forEach(function(name) {
                msg += '• ' + name + '\n';
            });
            msg += '\nUse /checkin [name] or /delete [name]';
            addToTimeline(msg);
        }
        return false;
    }
    
    // Handle delete place command
    var deleteMatch = prompt.match(/^\/?delete\s+(.+)$/i);
    if (deleteMatch) {
        form.elements["prompt"].value = '';
        var placeName = deleteMatch[1].trim();
        if (state.savedPlaces[placeName]) {
            delete state.savedPlaces[placeName];
            state.save();
            addToTimeline('🗑️ Deleted "' + placeName + '"');
        } else {
            addToTimeline('❌ No saved place named "' + placeName + '"');
        }
        return false;
    }
    
    // Handle checkin command - check saved places first, else send to server
    var checkinMatch = prompt.match(/^\/?checkin\s+(.+)$/i);
    if (checkinMatch) {
        var placeName = checkinMatch[1].trim();
        
        // Check if it's a saved place - handle locally
        if (state.savedPlaces && state.savedPlaces[placeName]) {
            form.elements["prompt"].value = '';
            var place = state.savedPlaces[placeName];
            // Update state without adding to timeline (we'll do it ourselves with ⭐)
            state.checkedIn = {
                name: placeName,
                lat: place.lat,
                lon: place.lon,
                time: Date.now()
            };
            state.save();
            addToTimeline('📍 Checked in to ' + placeName + ' ⭐');
            return false;
        }
        // Otherwise let it fall through to server
    }
    
    // Handle checkout - clear local state, then send to server
    if (prompt.match(/^\/?checkout$/i)) {
        // Clear local state first
        if (state.checkedIn) {
            state.checkedIn = null;
            state.save();
        }
        // Let it fall through to server
    }
    
    // Handle export command - download state as JSON
    if (prompt.match(/^\/?export$/i)) {
        form.elements["prompt"].value = '';
        var data = localStorage.getItem('malten_state');
        if (!data) {
            addToTimeline('❌ Nothing to export');
            return false;
        }
        var blob = new Blob([data], { type: 'application/json' });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = 'malten-backup-' + new Date().toISOString().split('T')[0] + '.json';
        a.click();
        URL.revokeObjectURL(url);
        addToTimeline('💾 Exported backup to Downloads');
        return false;
    }
    
    // Handle import command - show file picker
    if (prompt.match(/^\/?import$/i)) {
        form.elements["prompt"].value = '';
        var input = document.createElement('input');
        input.type = 'file';
        input.accept = '.json';
        input.onchange = function(e) {
            var file = e.target.files[0];
            if (!file) return;
            var reader = new FileReader();
            reader.onload = function(e) {
                try {
                    var data = JSON.parse(e.target.result);
                    if (!data.version) throw new Error('Invalid backup file');
                    localStorage.setItem('malten_state', e.target.result);
                    addToTimeline('✅ Imported backup. Reloading...');
                    setTimeout(function() { location.reload(); }, 1000);
                } catch(err) {
                    addToTimeline('❌ Invalid backup file: ' + err.message);
                }
            };
            reader.readAsText(file);
        };
        input.click();
        return false;
    }
    
    
    // Handle debug on/off - toggle screen logging
    var debugMatch = prompt.match(/^\/?debug\s+(on|off)$/i);
    if (debugMatch) {
        form.elements["prompt"].value = '';
        window.debugMode = debugMatch[1].toLowerCase() === 'on';
        addToTimeline('🔧 Debug mode ' + (window.debugMode ? 'ON' : 'OFF'));
        return false;
    }
    
    // Handle debug command locally
    if (prompt.match(/^\/?debug$/i)) {
        form.elements["prompt"].value = '';
        var info = '🔧 DEBUG\n';
        info += 'Stream: ' + getStream() + '\n';
        info += 'Location: ' + (state.hasLocation() ? state.lat.toFixed(4) + ', ' + state.lon.toFixed(4) : 'none') + '\n';
        info += 'Context cached: ' + (state.context ? 'yes' : 'none') + '\n';
        info += 'Cards: ' + (state.cards ? state.cards.length : 0) + '\n';
        info += 'Saved places: ' + Object.keys(state.savedPlaces || {}).join(', ') + '\n';
        info += 'State version: ' + (state.version || 'unknown') + '\n';
        info += 'JS version: 104';
        addToTimeline(info);
        return false;
    }

    // Handle /ping silently - just refresh context, don't show in timeline
    if (prompt.match(/^\/?ping$/i)) {
        form.elements["prompt"].value = '';
        if (state.hasLocation()) {
            fetchContext();
        }
        return false;
    }
    
    // Handle ping on/off - enable location tracking client-side
    var pingMatch = prompt.match(/^\/?ping\s+(on|off)$/i);
    if (pingMatch) {
        form.elements["prompt"].value = '';
        var action = pingMatch[1].toLowerCase();
        if (action === 'on') {
            enableLocation();
            addToTimeline('📍 Location tracking enabled');
        } else {
            disableLocation();
            addToTimeline('📍 Location tracking disabled');
        }
        return false;
    }

    // ========================================
    // SERVER COMMANDS (everything else)
    // ========================================
    
    // Send fresh location for nearby queries
    if (prompt.match(/^\/?nearby\s+/i) && state.hasLocation()) {
        sendFreshLocation();
    }

    // Ensure WebSocket is connected to correct stream before sending
    var targetStream = getStream();
    if (currentStream !== targetStream) {
        connectWebSocket();
    }
    
    // Post to /commands with location - response comes via WebSocket
    var data = {
        prompt: prompt,
        stream: targetStream
    };
    if (state.hasLocation()) {
        data.lat = state.lat;
        data.lon = state.lon;
    }
    
    // Show user's message and loading indicator
    displayUserMessage(prompt);
    showLoading();
    
    // Track pending to dedupe echo
    pendingMessages[prompt] = true;
    
    $.post(commandUrl, data).done(function(response) {
        // If we got a direct response, show it immediately
        if (response && response.length > 0 && !response.startsWith('{')) {
            hideLoading();
            delete pendingMessages[prompt];
            displayResponse(response);
            scrollToBottom();
        }
        // JSON responses (like /ping) are handled elsewhere
        // Empty responses mean async (AI) - wait for WebSocket
    });

    form.elements["prompt"].value = '';
    return false;
}

// Location functions
function enableLocation() {
    if (!navigator.geolocation) {
        showLocationNeeded('unavailable');
        return;
    }

    // Check/request permission first
    if (navigator.permissions) {
        navigator.permissions.query({ name: 'geolocation' }).then(function(result) {
            if (result.state === 'denied') {
                showLocationNeeded('denied');
                return;
            }
            // prompt or granted - proceed to request
            requestLocation();
        }).catch(function() {
            // permissions API not fully supported, try anyway
            requestLocation();
        });
    } else {
        // No permissions API, just try
        requestLocation();
    }
}

function requestLocation() {
    navigator.geolocation.getCurrentPosition(
        function(pos) {
            state.setLocation(pos.coords.latitude, pos.coords.longitude);
            $.post(commandUrl, {
                prompt: '/ping',
                stream: getStream(),
                lat: state.lat,
                lon: state.lon
            }).done(function(data) {
                if (data && data.length > 0) {
                    state.setContext(data);
                    displayContext(data);
                }
            });
            startLocationWatch();
        },
        function(err) {
            console.log("Location error:", err.code, err.message);
            handleLocationError(err);
        },
        { enableHighAccuracy: true, timeout: 15000, maximumAge: 10000 }
    );
}

function handleLocationError(err) {
    // err.code: 1=PERMISSION_DENIED, 2=POSITION_UNAVAILABLE, 3=TIMEOUT
    if (err.code === 1) {
        showLocationNeeded('denied');
    } else if (err.code === 2) {
        showLocationNeeded('unavailable');
    } else if (err.code === 3) {
        showLocationNeeded('timeout');
    } else {
        showLocationNeeded();
    }
}

var lastPingSent = 0;
var lastPingLat = 0;
var lastPingLon = 0;
var lastPingTime = 0;

// Movement tracking
var movementTracker = {
    startLat: null,
    startLon: null,
    startTime: null,
    totalDistance: 0,
    lastHeartbeat: 0,
    isMoving: false,
    
    reset: function() {
        this.startLat = state.lat;
        this.startLon = state.lon;
        this.startTime = Date.now();
        this.totalDistance = 0;
    },
    
    update: function(lat, lon) {
        if (!this.startLat) {
            this.startLat = lat;
            this.startLon = lon;
            this.startTime = Date.now();
            return;
        }
        
        var dist = haversineDistance(state.lat || lat, state.lon || lon, lat, lon);
        if (dist > 2) { // Ignore GPS jitter < 2m
            this.totalDistance += dist;
            this.isMoving = true;
        }
    },
    
    getStatus: function() {
        if (!this.startTime) return null;
        var elapsed = (Date.now() - this.startTime) / 1000 / 60; // minutes
        if (elapsed < 0.5) return null;
        
        var speed = this.totalDistance / (elapsed * 60); // m/s
        var mode = speed > 10 ? 'driving' : (speed > 1.5 ? 'walking' : 'stationary');
        
        return {
            distance: Math.round(this.totalDistance),
            minutes: Math.round(elapsed),
            speed: speed,
            mode: mode
        };
    },
    
    // Show heartbeat if moving and enough time passed
    heartbeat: function() {
        var now = Date.now();
        var status = this.getStatus();
        
        // Only show heartbeat if moving
        if (!status || status.mode === 'stationary') return;
        
        // Show heartbeat every 60 seconds while moving
        if (now - this.lastHeartbeat < 60000) return;
        this.lastHeartbeat = now;
        
        var msg = '';
        if (status.mode === 'walking') {
            msg = '🚶 ' + status.distance + 'm';
        } else if (status.mode === 'driving') {
            msg = '🚗 ' + status.distance + 'm';
        }
        
        // Add direction if we have enough data
        var heading = getHeading();
        if (heading) {
            msg += ' ' + heading;
        }
        
        if (msg) {
            addToTimeline(msg, 'movement');
        }
    }
};

// Get compass heading from recent locations
function getHeading() {
    if (!state.locationHistory || state.locationHistory.length < 2) return null;
    var recent = state.locationHistory.slice(-5);
    if (recent.length < 2) return null;
    
    var first = recent[0];
    var last = recent[recent.length - 1];
    
    var dLon = (last.lon - first.lon) * Math.PI / 180;
    var y = Math.sin(dLon) * Math.cos(last.lat * Math.PI / 180);
    var x = Math.cos(first.lat * Math.PI / 180) * Math.sin(last.lat * Math.PI / 180) -
            Math.sin(first.lat * Math.PI / 180) * Math.cos(last.lat * Math.PI / 180) * Math.cos(dLon);
    var bearing = Math.atan2(y, x) * 180 / Math.PI;
    bearing = (bearing + 360) % 360;
    
    // Convert to compass direction
    var directions = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];
    var index = Math.round(bearing / 45) % 8;
    return '→' + directions[index];
}

// Adaptive ping interval based on speed
// Driving (>10 m/s): 5s, Walking (2-10 m/s): 10s, Stationary: 30s
function getPingInterval() {
    if (!lastPingTime || !lastPingLat) return 15000;
    
    var now = Date.now();
    var elapsed = (now - lastPingTime) / 1000; // seconds
    if (elapsed < 2) return 15000; // need time to measure
    
    var distance = haversineDistance(lastPingLat, lastPingLon, state.lat, state.lon);
    var speed = distance / elapsed; // m/s
    
    if (speed > 10) return 5000;  // Driving: every 5s
    if (speed > 2) return 10000;  // Walking: every 10s  
    return 30000;                 // Stationary: every 30s
}

// Haversine formula for distance in meters
function haversineDistance(lat1, lon1, lat2, lon2) {
    var R = 6371000; // Earth's radius in meters
    var dLat = (lat2 - lat1) * Math.PI / 180;
    var dLon = (lon2 - lon1) * Math.PI / 180;
    var a = Math.sin(dLat/2) * Math.sin(dLat/2) +
            Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
            Math.sin(dLon/2) * Math.sin(dLon/2);
    var c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1-a));
    return R * c;
}

// Send fresh location immediately (bypasses throttle)
function sendFreshLocation() {
    if (!navigator.geolocation) return;
    
    navigator.geolocation.getCurrentPosition(
        function(pos) {
            lastPingSent = Date.now();
            sendLocation(pos.coords.latitude, pos.coords.longitude);
        },
        function(err) {
            console.log("Fresh location error:", err.message);
        },
        { enableHighAccuracy: true, timeout: 10000, maximumAge: 0 }
    );
}

function startLocationWatch() {
    if (locationWatchId) {
        navigator.geolocation.clearWatch(locationWatchId);
    }
    
    // Reset movement tracker when starting watch
    movementTracker.reset();
    
    locationWatchId = navigator.geolocation.watchPosition(
        function(pos) {
            var lat = pos.coords.latitude;
            var lon = pos.coords.longitude;
            var now = Date.now();
            
            // Update movement tracker (before updating state)
            movementTracker.update(lat, lon);
            
            // Always update local state immediately
            var moved = false;
            if (state.lat && state.lon) {
                var distance = haversineDistance(state.lat, state.lon, lat, lon);
                moved = distance > 20; // More than 20m = significant movement
            }
            
            // Update local state
            state.setLocation(lat, lon);
            
            // Check for movement heartbeat
            movementTracker.heartbeat();
            
            // If significant movement, ping immediately
            if (moved || !lastPingSent) {
                lastPingLat = lat;
                lastPingLon = lon;
                lastPingTime = now;
                lastPingSent = now;
                sendLocation(lat, lon);
            } else {
                // Otherwise respect throttle interval
                var interval = getPingInterval();
                if (now - lastPingSent >= interval) {
                    lastPingLat = lat;
                    lastPingLon = lon;
                    lastPingTime = now;
                    lastPingSent = now;
                    sendLocation(lat, lon);
                }
            }
        },
        function(err) {
            console.log("Location watch error:", err.message);
        },
        { enableHighAccuracy: true, timeout: 5000, maximumAge: 1000 }
    );
}

// Fetch and display daily reminder (once per day)
function fetchReminder() {
    var today = new Date().toISOString().split('T')[0];
    
    // Daily reminder - shows once per day on first open
    if (state.reminderDate !== today) {
        $.post(commandUrl, { prompt: '/reminder', stream: getStream() }).done(function(response) {
            try {
                var r = JSON.parse(response);
                if (!r || (!r.verse && !r.name)) return;
                
                // Mark as shown
                state.reminderDate = today;
                state.save();
                
                // Display reminder card
                displayReminderCard(r);
            } catch(e) {}
        });
    }
    
    // Prayer-time reminders - check context for current prayer
    checkPrayerReminder();
}

// Check if we should show a prayer-time reminder based on current prayer
function checkPrayerReminder() {
    var ctx = state.context;
    if (!ctx || !ctx.prayer) return;
    
    var today = new Date().toISOString().split('T')[0];
    var hour = new Date().getHours();
    var reminderKey = null;
    
    // Duha time: after sunrise (roughly 8am) until Dhuhr (roughly noon)
    // This is when there's no "current" prayer in the morning
    if (!ctx.prayer.current && hour >= 8 && hour < 12) {
        reminderKey = 'duha';
    } else if (ctx.prayer.current) {
        // Map prayer names to reminder keys
        var prayerToReminder = {
            'fajr': 'fajr',
            'dhuhr': 'dhuhr',
            'asr': 'asr',
            'maghrib': 'maghrib',
            'isha': 'isha'
        };
        reminderKey = prayerToReminder[ctx.prayer.current.toLowerCase()];
    }
    
    if (!reminderKey) return;
    
    // Check if already shown today
    if (state.prayerReminders[reminderKey] === today) return;
    
    // Fetch and display the prayer reminder
    $.post(commandUrl, { prompt: '/reminder ' + reminderKey, stream: getStream() }).done(function(response) {
        try {
            var r = JSON.parse(response);
            if (!r || (!r.verse && !r.name)) return;
            
            // Mark as shown
            state.prayerReminders[reminderKey] = today;
            state.save();
            
            // Display reminder card
            displayReminderCard(r);
        } catch(e) {}
    });
}

function displayReminderCard(r) {
    // Build the text
    var text = '';
    if (r.name && r.name.length > 0) {
        text = '📿 ' + r.name.split('\n\n')[0]; // Just the title
    } else if (r.verse && r.verse.length > 0) {
        var verseParts = r.verse.split('\n\n');
        var verseRef = verseParts[0] || '';
        var verseText = verseParts.slice(1).join('\n\n') || r.verse;
        text = '📖 "' + verseText.trim() + '" — ' + verseRef;
    }
    
    if (!text) return;
    
    // displaySystemMessage handles both display AND persistence
    addToTimeline(text);
}

function fetchContext() {
    if (!state.hasLocation()) return;
    $.post(commandUrl, {
        prompt: '/ping',
        stream: getStream(),
        lat: state.lat,
        lon: state.lon
    }).done(function(response) {
        if (response && response.length > 0) {
            state.setContext(response);
            displayContext(response);
        }
    });
}

function displayContext(ctx, forceUpdate, needsAction) {
    // Handle both JSON object and string
    if (typeof ctx === 'string') {
        try {
            ctx = JSON.parse(ctx);
        } catch(e) {
            // Legacy text format
            ctx = { html: ctx, places: {} };
        }
    }
    
    // Don't replace substantive cached context with empty/minimal response
    var html = ctx.html || '';
    if (!forceUpdate && state.context && state.context.html && state.context.html.length > 50) {
        if (!html || html.length < 30 || html.indexOf('enable_location') >= 0) {
            console.log('Keeping cached context, new context too minimal:', html.length);
            return;
        }
    }
    
    // Build summary line from structured data or HTML
    var summary = buildContextSummary(ctx, needsAction);
    
    // Build full HTML with clickable place links
    var fullHtml = buildContextHtml(ctx);
    
    // Age indicator (same for both collapsed and expanded)
    var ageIndicator = '';
    if (state.contextTime > 0 && !needsAction) {
        var age = Date.now() - state.contextTime;
        var ageSecs = Math.floor(age / 1000);
        var ageStr = ageSecs < 60 ? 'now' : (ageSecs < 3600 ? Math.floor(ageSecs / 60) + 'm' : Math.floor(ageSecs / 3600) + 'h');
        var staleClass = ageSecs > 300 ? 'stale' : 'fresh';
        if (isAcquiringLocation) {
            ageIndicator = '<span class="context-age acquiring">📡</span>';
        } else {
            ageIndicator = '<span class="context-age ' + staleClass + '">' + ageStr + '</span>';
        }
    }
    
    // Update the context card (outside messages list)
    var contextCard = document.getElementById('context-card');
    var cardHtml = '<div class="context-summary"><span class="context-content">' + summary + '</span>' + ageIndicator + '</div>' +
        '<div class="context-full">' + ageIndicator + fullHtml + '</div>';
    
    if (!contextCard) {
        // Create context card before messages container
        var div = document.createElement('div');
        div.id = 'context-card';
        div.innerHTML = cardHtml;
        div.onclick = function(e) {
            // Don't toggle if clicking a link inside the card
            var target = e.target;
            if (target.tagName === 'A') return;
            // Check parent chain for links
            while (target && target !== this) {
                if (target.tagName === 'A') return;
                target = target.parentElement;
            }
            this.classList.toggle('expanded');
            state.contextExpanded = this.classList.contains('expanded');
            state.save();
        };
        var container = document.getElementById('messages-area');
        container.parentNode.insertBefore(div, container);
        contextCard = div;
    } else {
        var wasExpanded = contextCard.classList.contains('expanded');
        contextCard.innerHTML = cardHtml;
        if (wasExpanded) contextCard.classList.add('expanded');
    }
    
    // Expand by default when action is needed (first load, location needed)
    if (needsAction) {
        contextCard.classList.add('expanded');
    }
    
    // Restore expanded state from localStorage
    if (state.contextExpanded && !needsAction) {
        contextCard.classList.add('expanded');
    }
}

// Build one-line summary from context
function buildContextSummary(ctx, needsAction) {
    var html = ctx.html || '';
    
    // If action is needed (welcome, location needed), show that
    if (needsAction) {
        if (html.indexOf('enable_location') >= 0) {
            return '📍 Enable location for live updates';
        }
        if (html.indexOf('Location needed') >= 0) {
            return '📍 Location needed - tap for details';
        }
        return 'Tap for details';
    }
    
    var parts = [];
    
    // Date/time
    var now = new Date();
    var dateStr = now.toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short' });
    parts.push(dateStr);
    
    // Temperature
    if (ctx.weather && ctx.weather.condition) {
        var tempMatch = ctx.weather.condition.match(/-?\d+°C/);
        if (tempMatch) parts.push(tempMatch[0]);
    }
    
    // Prayer - short version
    if (ctx.prayer && ctx.prayer.display) {
        var prayerShort = ctx.prayer.display.replace('🕌 ', '').split(' · ')[0];
        parts.push('🕌 ' + prayerShort);
    }
    
    // Fallback to parsing HTML if no structured data
    if (parts.length <= 1) {
        var tempMatch = html.match(/-?\d+°C/);
        if (tempMatch && parts.indexOf(tempMatch[0]) < 0) parts.push(tempMatch[0]);
    }
    
    return parts.length > 0 ? parts.join(' · ') : 'Tap to expand';
}

// Build full context HTML with clickable place links
function buildContextHtml(ctx) {
    var html = ctx.html || '';
    
    // Enable location button
    html = html.replace(/\{enable_location\}/g, 
        '<a href="javascript:void(0)" class="enable-location-btn">📍 Enable location</a>');
    
    // Replace place counts with clickable links
    var categoryIcons = {
        'cafe': '☕',
        'restaurant': '🍽️',
        'pharmacy': '💊',
        'supermarket': '🛒'
    };
    
    for (var category in ctx.places) {
        if (!ctx.places.hasOwnProperty(category)) continue;
        var places = ctx.places[category];
        if (!places || places.length === 0) continue;
        
        var icon = categoryIcons[category] || '📍';
        var label = places.length === 1 ? places[0].name : places.length + ' places';
        
        // Create link with data attribute containing JSON
        var link = '<a href="javascript:void(0)" class="place-link" data-category="' + category + '">' + label + '</a>';
        
        // Replace the pattern in HTML (e.g., "☕ 7 places" or "💊 Boots")
        var pattern = new RegExp(icon + ' (' + label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '|\\d+ places|' + places[0].name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')');
        html = html.replace(pattern, icon + ' ' + link);
    }
    
    return html.replace(/\n/g, '<br>');
}

// Augment check-in prompt with saved places
function augmentCheckInPrompt(text) {
    var savedPlaces = state.savedPlaces || {};
    var names = Object.keys(savedPlaces);
    if (names.length === 0 || !state.hasLocation()) return text;
    
    // Find where POIs end (before the "Reply with..." line)
    var lines = text.split('\n');
    var insertIdx = lines.length - 2; // Before blank line and "Reply with..."
    
    // Add saved places
    var userLoc = { lat: state.lat, lon: state.lon };
    names.forEach(function(name) {
        var place = savedPlaces[name];
        if (place && place.lat && place.lon) {
            var dist = state.distance(userLoc, place) * 1000; // km to m
            lines.splice(insertIdx, 0, '• ' + name + ' (' + Math.round(dist) + 'm) ⭐');
            insertIdx++;
        }
    });
    
    return lines.join('\n');
}

// Make check-in options clickable buttons
function makeCheckInClickable(text) {
    if (text.indexOf('Where are you?') < 0) {
        return makeClickable(text);
    }
    
    var html = text;
    
    // Convert "• Place (123m)" lines to clickable buttons
    html = html.replace(/• ([^(]+) \((\d+)m\)( ⭐)?/g, function(match, name, dist, star) {
        var isSaved = star ? ' saved' : '';
        return '<button class="checkin-option' + isSaved + '" data-name="' + escapeHTML(name.trim()) + '">' +
            name.trim() + ' <span class="dist">(' + dist + 'm)</span>' +
            (star ? ' ⭐' : '') + '</button>';
    });
    
    // Remove the "Reply with..." instruction since we have buttons
    html = html.replace(/Reply with the name to check in, or ignore\.?/g, 
        '<button class="checkin-dismiss">Not here</button>');
    
    return html;
}

// Make place names and counts clickable
function makeClickable(text) {
    var html = text;
    
    // Enable location button
    html = html.replace(/\{enable_location\}/g, 
        '<a href="javascript:void(0)" class="enable-location-btn">📍 Enable location</a>');
    
    // Convert [Directions:name:lat:lon] to clickable link
    html = html.replace(/\[Directions:([^:]+):([^:]+):([^\]]+)\]/g, function(match, name, lat, lon) {
        return '<a href="javascript:void(0)" class="directions-link" data-name="' + 
            escapeAttr(name) + '" data-lat="' + lat + '" data-lon="' + lon + '">Directions</a>';
    });
    
    // Convert URLs to clickable links
    html = html.replace(/(https?:\/\/[A-Za-z0-9-_.]+\.[A-Za-z0-9-_:%&~\?\/.=#,@+]+)/g, function(url) {
        if (url.includes('google.com/maps') || url.includes('maps.google.com')) {
            return '<a href="' + url + '" target="_blank" class="map-link">Map</a>';
        }
        return '<a href="' + url + '" target="_blank" class="web-link">Link</a>';
    });
    
    return html;
}

// Escape for HTML attributes
function escapeAttr(str) {
    return str.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// Handle enable location button click
$(document).on('click', '.enable-location-btn', function(e) {
    e.preventDefault();
    e.stopPropagation(); // Prevent context card from closing
    
    // Disable button and show acquiring state
    var btn = $(this);
    btn.text('📍 Acquiring location...').addClass('acquiring');
    
    getLocationAndContext();
});

function resetLocationButton() {
    $('.enable-location-btn').text('📍 Enable location').removeClass('acquiring');
}

// Handle clicks on place links - show places (ephemeral, not persisted)
$(document).on('click touchend', '.place-link', function(e) {
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    
    var category = $(this).attr('data-category');
    if (!category || !state.context || !state.context.places) return false;
    
    var places = state.context.places[category];
    if (!places || places.length === 0) return false;
    
    var lines = [];
    for (var i = 0; i < places.length; i++) {
        var p = places[i];
        var line = '📍 ' + p.name;
        
        // Build info line
        var info = [];
        if (p.address) info.push(p.address);
        if (p.postcode) info.push(p.postcode);
        if (p.hours) info.push('🕒' + p.hours);
        if (p.phone) info.push('📞' + p.phone);
        if (info.length) line += '\n   ' + info.join(', ');
        
        // Add Map and Directions links as HTML (rendered directly, not stored)
        var encodedName = encodeURIComponent(p.name).replace(/'/g, '%27');
        var mapUrl = 'https://www.google.com/maps/search/' + encodedName + '/@' + p.lat + ',' + p.lon + ',17z';
        var links = [];
        links.push('<a href="' + mapUrl + '" target="_blank" class="map-link">Map</a>');
        links.push('<a href="javascript:void(0)" class="directions-link" data-name="' + escapeAttr(p.name) + '" data-lat="' + p.lat + '" data-lon="' + p.lon + '">Directions</a>');
        line += '\n   ' + links.join(' · ');
        
        lines.push(line);
    }
    
    var text = lines.join('\n\n');
    
    // Add directly to DOM (ephemeral - place expansion doesn't need to persist)
    var li = document.createElement('li');
    var html = text.replace(/\n/g, '<br>');
    li.innerHTML = '<div class="card card-location"><span class="card-time">Just now</span>' + html + '</div>';
    document.getElementById('messages').appendChild(li);
    scrollToBottom();
    
    return false;
});

// Clicking directions link gets walking directions
$(document).on('click touchend', '.directions-link', function(e) {
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    
    var name = $(this).data('name');
    var toLat = $(this).data('lat');
    var toLon = $(this).data('lon');
    
    if (!state.hasLocation()) {
        addToTimeline('📍 Need your location for directions');
        return false;
    }
    
    // Show loading in timeline
    addToTimeline('🚶 Getting directions to ' + name + '...');
    
    // Call server for directions
    $.post(commandUrl, {
        prompt: '/directions ' + name,
        stream: getStream(),
        lat: state.lat,
        lon: state.lon,
        toLat: toLat,
        toLon: toLon
    }).done(function(response) {
        // Add response to timeline (persisted)
        addToTimeline(response);
    }).fail(function() {
        addToTimeline('❌ Couldn\'t get directions to ' + name);
    });
    
    return false;
});

// Show places as a card in the timeline

function scrollToBottom() {
    setTimeout(function() {
        // Try scrollIntoView on last message
        var messages = document.getElementById('messages');
        if (messages && messages.lastElementChild) {
            messages.lastElementChild.scrollIntoView({ behavior: 'smooth', block: 'end' });
        }
    }, 100);
}

function showLoading() {
    var el = document.getElementById('loading');
    if (!el) {
        el = document.createElement('div');
        el.id = 'loading';
        el.textContent = '...';
        document.getElementById('messages').parentNode.insertBefore(el, document.getElementById('messages'));
    }
    el.style.display = 'block';
}

function hideLoading() {
    var el = document.getElementById('loading');
    if (el) el.style.display = 'none';
}

// Command metadata from server (loaded on startup)
var commandMeta = {};

// Load command metadata from server
function loadCommandMeta() {
    $.get('/commands/meta').done(function(data) {
        if (Array.isArray(data)) {
            data.forEach(function(cmd) {
                commandMeta[cmd.name] = cmd;
            });
        }
    });
}

// Convert /commands to human-readable display text using server metadata
function humanizeCommand(text) {
    // Parse command: /name args
    var m = text.match(/^\/?([a-z]+)(?:\s+(.*))?$/i);
    if (!m) return text;
    
    var cmdName = m[1].toLowerCase();
    var args = m[2] || '';
    var cmd = commandMeta[cmdName];
    
    if (cmd && cmd.emoji && cmd.loading) {
        // Use server-provided format: "Getting directions to %s..."
        var loadingText = cmd.loading.replace('%s', args);
        return cmd.emoji + ' ' + loadingText;
    }
    
    // Fallback: just remove the slash
    if (text.startsWith('/')) {
        return text.substring(1);
    }
    
    return text;
}

// Display user command as a terminal-style line (not a card)
function displayUserMessage(text) {
    var ts = Date.now();
    var displayText = humanizeCommand(text);
    
    var li = document.createElement('li');
    li.className = 'command-item';
    li.innerHTML = '<div class="command-line" data-timestamp="' + ts + '">' +
        escapeHTML(displayText) +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.appendChild(li);
    scrollToBottom();
    
    // Track pending command for response matching
    pendingCommand = { element: li, text: text, ts: ts };
    
    // Save to conversation state (save display text, not raw command)
    if (!state.conversation) {
        state.conversation = { time: ts, messages: [] };
    }
    state.conversation.messages.push({ role: 'user', text: displayText });
    state.save();
}

var pendingCommand = null;

// Display AI response as a card below the command
function displayResponse(text) {
    // Add to timeline (persisted) - this is the ONE way
    addToTimeline(text);
    
    // Also save to conversation for context
    if (state.conversation) {
        state.conversation.messages.push({ role: 'assistant', text: text });
        state.save();
    }
    
    pendingCommand = null;
}

// No longer need conversation timeout with new format
function restoreConversation() {
    if (!state.conversation || !state.conversation.messages) return;
    
    var ts = state.conversation.time;
    var messages = document.getElementById('messages');
    
    // Restore each message in the new format
    state.conversation.messages.forEach(function(msg) {
        var li = document.createElement('li');
        if (msg.role === 'user') {
            li.className = 'command-item';
            li.innerHTML = '<div class="command-line" data-timestamp="' + ts + '">' +
                escapeHTML(msg.text) + '</div>';
        } else {
            var html = makeClickable(msg.text).replace(/\n/g, '<br>');
            var cardType = getCardType(msg.text);
            li.innerHTML = '<div class="card ' + cardType + '" data-timestamp="' + ts + '">' + html + '</div>';
        }
        messages.appendChild(li);
    });
    scrollToBottom();
}

function getCardType(text) {
    if (text.indexOf('🚏') >= 0 || text.indexOf('🚌') >= 0) return 'card-transport';
    if (text.indexOf('🌧️') >= 0 || text.indexOf('☀️') >= 0 || text.indexOf('⛅') >= 0) return 'card-weather';
    if (text.indexOf('🕌') >= 0) return 'card-prayer';
    if (text.indexOf('📍') >= 0) return 'card-location';
    if (text.indexOf('📖') >= 0 || text.indexOf('📿') >= 0) return 'card-reminder';
    return '';
}

function disableLocation() {
    if (locationWatchId) {
        navigator.geolocation.clearWatch(locationWatchId);
        locationWatchId = null;
    }
}

function sendLocation(lat, lon) {
    var oldStream = currentStream;
    state.setLocation(lat, lon);
    
    // Detect stream/area change
    var newStream = getStream();
    var areaChanged = oldStream && newStream !== oldStream;
    if (areaChanged) {
        connectWebSocket(); // Reconnect to new stream
        // Reset movement tracker for new area
        movementTracker.reset();
    }
    
    $.post(commandUrl, {
        prompt: '/ping',
        stream: newStream,
        lat: lat,
        lon: lon
    }).done(function(data) {
        if (data && data.length > 0) {
            state.setContext(data);
            displayContext(data);
            
            // Note area change in timeline
            if (areaChanged) {
                var ctx = typeof data === 'string' ? JSON.parse(data) : data;
                if (ctx && ctx.location) {
                    var areaName = ctx.location.name.split(',')[0];
                    addToTimeline('📍 Entered ' + areaName, 'movement');
                }
            }
        }
    });
}

// Get location and refresh context
function getLocationAndContext() {
    if (!navigator.geolocation) {
        showLocationNeeded('unavailable');
        return;
    }
    
    // Check permission state first to show appropriate message while waiting
    if (navigator.permissions) {
        navigator.permissions.query({ name: 'geolocation' }).then(function(result) {
            if (result.state === 'denied') {
                // Already denied - show instructions
                if (!state.context || !state.hasLocation()) {
                    showLocationNeeded('denied');
                }
            } else if (result.state === 'prompt') {
                // Browser will prompt - show "acquiring" not "enable button"
                // The enable button is only for when denied
                if (!state.context) {
                    showAcquiring();
                }
                requestLocationForContext();
            } else {
                // granted - just get location
                requestLocationForContext();
            }
        }).catch(function() {
            // Permissions API not supported, just try
            requestLocationForContext();
        });
    } else {
        // No permissions API, just try
        requestLocationForContext();
    }
}

// Show acquiring state (browser will prompt for permission)
function showAcquiring() {
    var msg = '📡 Acquiring location...\n\n';
    msg += 'Your browser will ask for permission.\n';
    msg += 'Please allow location access.';
    displayContext({ html: msg, places: {} }, true);
}

function requestLocationForContext() {
    // Always show acquiring state - it's an event
    setAcquiring(true);
    
    var isFirstLocation = !state.hasLocation();
    
    navigator.geolocation.getCurrentPosition(
        function(pos) {
            setAcquiring(false);
            var oldStream = currentStream;
            var wasStale = state.contextTime && (Date.now() - state.contextTime > 10 * 60 * 1000);
            var wasFirstLocation = isFirstLocation;
            state.setLocation(pos.coords.latitude, pos.coords.longitude);
            
            // Reconnect WebSocket if stream changed
            var newStream = getStream();
            if (newStream !== oldStream) {
                connectWebSocket();
            }
            
            // Ping returns context
            $.post(commandUrl, {
                prompt: '/ping',
                stream: newStream,
                lat: state.lat,
                lon: state.lon
            }).done(function(data) {
                if (data && data.length > 0) {
                    // Parse JSON if string
                    var ctx = data;
                    if (typeof data === 'string') {
                        try { ctx = JSON.parse(data); } catch(e) { ctx = { html: data }; }
                    }
                    
                    // Server pushes location changes via WebSocket
                    // Client just updates context display
                    if (wasFirstLocation && ctx.agent) {
                        var agentMsg = '🤖 Agent ' + ctx.agent.status;
                        if (ctx.agent.poi_count > 0) {
                            agentMsg += ' · ' + ctx.agent.poi_count + ' places nearby';
                        }
                        addToTimeline(agentMsg);
                    }
                    state.setContext(ctx);
                    displayContext(ctx);
                }
            });
            startLocationWatch();
        },
        function(err) {
            setAcquiring(false);
            resetLocationButton();
            console.log("Location error:", err.code, err.message);
            // If we have cached context, show it but also note the error
            if (state.context && state.hasLocation()) {
                displayContext(state.context, true);
            } else {
                handleLocationError(err);
            }
        },
        { enableHighAccuracy: true, timeout: 10000, maximumAge: 10000 }
    );
}

function updateAcquiringIndicator() {
    // Update context card to show acquiring state
    if (state.context) {
        displayContext(state.context, true);
    }
}

// Speech recognition
var recognition = null;
var isListening = false;

function initSpeech() {
    var SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognition) {
        var mic = document.getElementById('mic');
        if (mic) mic.style.display = 'none';
        return;
    }
    
    recognition = new SpeechRecognition();
    recognition.continuous = false;
    recognition.interimResults = true;
    recognition.lang = 'en-GB';
    
    recognition.onresult = function(e) {
        var transcript = '';
        for (var i = e.resultIndex; i < e.results.length; i++) {
            transcript += e.results[i][0].transcript;
        }
        document.getElementById('prompt').value = transcript;
        
        // Auto-submit on final result
        if (e.results[e.results.length - 1].isFinal) {
            setTimeout(function() {
                if (transcript.trim()) submitCommand();
            }, 300);
        }
    };
    
    recognition.onend = function() {
        isListening = false;
        document.getElementById('mic').classList.remove('listening');
    };
    
    recognition.onerror = function(e) {
        isListening = false;
        document.getElementById('mic').classList.remove('listening');
    };
    
    var mic = document.getElementById('mic');
    if (mic) mic.addEventListener('click', toggleSpeech);
}

function toggleSpeech() {
    if (!recognition) return;
    
    if (isListening) {
        recognition.stop();
        isListening = false;
    } else {
        recognition.start();
        isListening = true;
        document.getElementById('mic').classList.add('listening');
    }
}

// Available commands for autocomplete
var commands = [
    { cmd: '/nearby', desc: 'Find nearby places', usage: '/nearby cafe' },
    { cmd: '/directions', desc: 'Get walking directions', usage: '/directions Waitrose' },
    { cmd: '/checkin', desc: 'Check in to a place', usage: '/checkin Home' },
    { cmd: '/checkout', desc: 'Clear check-in' },
    { cmd: '/save', desc: 'Save current location', usage: '/save Home' },
    { cmd: '/places', desc: 'List saved places' },
    { cmd: '/delete', desc: 'Delete saved place', usage: '/delete Home' },
    { cmd: '/weather', desc: 'Current weather' },
    { cmd: '/bus', desc: 'Bus times' },
    { cmd: '/prayer', desc: 'Prayer times' },
    { cmd: '/reminder', desc: 'Daily verse' },
    { cmd: '/export', desc: 'Backup to file' },
    { cmd: '/import', desc: 'Restore from file' },
    { cmd: '/refresh', desc: 'Force reload' },
    { cmd: '/clear', desc: 'Reset all data' },
    { cmd: '/debug', desc: 'Show debug info' },
];

function showCommandPalette(filter) {
    var existing = document.getElementById('command-palette');
    if (existing) existing.remove();
    
    var filtered = commands.filter(function(c) {
        return !filter || c.cmd.toLowerCase().indexOf(filter.toLowerCase()) === 0;
    });
    
    if (filtered.length === 0) return;
    
    var palette = document.createElement('div');
    palette.id = 'command-palette';
    
    // Add title
    var title = document.createElement('div');
    title.className = 'command-title';
    title.textContent = 'Commands';
    palette.appendChild(title);
    
    filtered.forEach(function(c) {
        var item = document.createElement('div');
        item.className = 'command-item';
        item.innerHTML = '<span class="cmd">' + c.cmd + '</span> <span class="desc">' + c.desc + '</span>';
        item.onclick = function() {
            document.getElementById('prompt').value = c.cmd + ' ';
            document.getElementById('prompt').focus();
            hideCommandPalette();
        };
        palette.appendChild(item);
    });
    
    document.getElementById('bottom-fixed').appendChild(palette);
}

function hideCommandPalette() {
    var palette = document.getElementById('command-palette');
    if (palette) palette.remove();
}

function loadListeners() {
    // Scroll to load more (scroll down = load older)
    var area = document.getElementById('messages-area');
    if (area) {
        area.addEventListener('scroll', function() {
            if (area.scrollTop + area.clientHeight >= area.scrollHeight - 50) {
                loadMore();
            }
        });
    }

    var prompt = document.getElementById('prompt');
    if (prompt) {
        // Submit on Enter key
        prompt.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') {
                e.preventDefault();
                hideCommandPalette();
                submitCommand();
            }
            if (e.key === 'Escape') {
                hideCommandPalette();
            }
        });
        
        // Show command palette when typing /
        prompt.addEventListener('input', function(e) {
            var val = prompt.value;
            if (val.startsWith('/')) {
                showCommandPalette(val);
            } else {
                hideCommandPalette();
            }
        });
        
        // Collapse context card when input is focused (mobile keyboard)
        prompt.addEventListener('focus', function() {
            var card = document.getElementById('context-card');
            if (card) card.classList.remove('expanded');
            scrollToBottom();
        });
        
        // Hide palette on blur (with delay for click to register)
        prompt.addEventListener('blur', function() {
            setTimeout(hideCommandPalette, 200);
        });
    }

    initSpeech();
    
    // Handle mobile keyboard showing/hiding
    if (window.visualViewport) {
        window.visualViewport.addEventListener('resize', function() {
            // Scroll input into view when keyboard opens
            var prompt = document.getElementById('prompt');
            if (document.activeElement === prompt) {
                setTimeout(function() {
                    prompt.scrollIntoView({ block: 'end' });
                }, 100);
            }
        });
    }
}

// NO SERVICE WORKER - killed at top of file

// Update all card timestamps periodically
function updateTimestamps() {
    var cards = document.querySelectorAll('.card[data-timestamp]');
    cards.forEach(function(card) {
        var ts = parseInt(card.getAttribute('data-timestamp'), 10);
        var timeEl = card.querySelector('.card-time');
        if (timeEl && ts) {
            timeEl.textContent = formatTimeAgo(ts);
        }
    });
}

// Step counter and motion detection
var stepDetector = {
    lastAccel: 0,
    lastTime: 0,
    threshold: 10,  // Acceleration threshold for step detection
    minInterval: 250,  // Min ms between steps (prevents double counting)
    movementWindow: [],  // Last 5 seconds of movement data
    movementThreshold: 3,  // Number of movements to consider "walking"
    
    init: function() {
        if (!window.DeviceMotionEvent) {
            console.log('DeviceMotion not supported');
            return;
        }
        
        // iOS 13+ requires permission
        if (typeof DeviceMotionEvent.requestPermission === 'function') {
            // Will request on first user interaction
            this.needsPermission = true;
        } else {
            this.start();
        }
    },
    
    requestPermission: function() {
        if (typeof DeviceMotionEvent.requestPermission === 'function') {
            DeviceMotionEvent.requestPermission()
                .then(function(response) {
                    if (response === 'granted') {
                        stepDetector.start();
                    }
                })
                .catch(console.error);
        }
    },
    
    start: function() {
        var self = this;
        window.addEventListener('devicemotion', function(e) {
            self.handleMotion(e);
        });
        console.log('Step counter started');
    },
    
    handleMotion: function(e) {
        var accel = e.accelerationIncludingGravity;
        if (!accel) return;
        
        var now = Date.now();
        
        // Calculate total acceleration magnitude
        var magnitude = Math.sqrt(accel.x * accel.x + accel.y * accel.y + accel.z * accel.z);
        
        // Track movement for GPS-stuck detection
        this.movementWindow.push({ time: now, mag: magnitude });
        // Keep only last 5 seconds
        var cutoff = now - 5000;
        this.movementWindow = this.movementWindow.filter(function(m) { return m.time > cutoff; });
        
        // Detect step: acceleration spike above threshold
        var delta = Math.abs(magnitude - this.lastAccel);
        if (delta > this.threshold && (now - this.lastTime) > this.minInterval) {
            this.countStep();
            this.lastTime = now;
        }
        this.lastAccel = magnitude;
    },
    
    countStep: function() {
        var today = new Date().toDateString();
        
        // Reset count if new day
        if (state.steps.date !== today) {
            state.steps = { count: 0, date: today };
        }
        
        state.steps.count++;
        state.save();
        
        // Update display if visible
        this.updateDisplay();
    },
    
    updateDisplay: function() {
        var el = document.getElementById('step-count');
        if (el) {
            el.textContent = state.steps.count.toLocaleString();
        }
    },
    
    isMoving: function() {
        // Check if significant movement in last 5 seconds
        var movements = this.movementWindow.filter(function(m) {
            return m.mag > 12;  // Above resting gravity (~9.8)
        });
        return movements.length > this.movementThreshold;
    },
    
    getSteps: function() {
        var today = new Date().toDateString();
        if (state.steps.date !== today) {
            return 0;
        }
        return state.steps.count;
    }
};

// Check for motion while GPS is stuck (for check-in prompt)
var lastCheckInPrompt = 0;

function checkMotionGpsStuck() {
    // If we're moving (accelerometer) but GPS hasn't changed
    if (stepDetector.isMoving() && lastPingLat && lastPingLon) {
        var timeSinceMove = Date.now() - lastPingTime;
        if (timeSinceMove > 60000) {  // GPS stuck for 1+ minute
            var distance = haversineDistance(lastPingLat, lastPingLon, state.lat, state.lon);
            if (distance < 20) {  // GPS moved less than 20m
                state.motionDetected = true;
                
                // Don't prompt more than once per 10 minutes
                var now = Date.now();
                if (now - lastCheckInPrompt > 10 * 60 * 1000) {
                    lastCheckInPrompt = now;
                    showCheckInPrompt();
                }
            }
        }
    }
}

function showCheckInPrompt() {
    // Get nearby places from context
    var places = [];
    if (state.context && state.context.places) {
        // Collect places from all categories
        Object.keys(state.context.places).forEach(function(cat) {
            var list = state.context.places[cat];
            if (list && list.length) {
                list.slice(0, 2).forEach(function(p) {
                    places.push({ name: p.name, saved: false });
                });
            }
        });
    }
    
    // Add saved places at the top
    var savedPlaces = state.savedPlaces || {};
    Object.keys(savedPlaces).forEach(function(name) {
        places.unshift({ name: name, saved: true });
    });
    
    var msg = '📍 GPS seems stuck but you\'re moving.\nAre you indoors?\n\n';
    if (places.length > 0) {
        msg += 'Check in to:\n';
        places.slice(0, 6).forEach(function(p) {
            var star = p.saved ? ' ⭐' : '';
            msg += '• <a href="javascript:void(0)" class="checkin-link" data-place="' + escapeHTML(p.name) + '">' + escapeHTML(p.name) + star + '</a>\n';
        });
        msg += '\nOr type /checkin [place name]';
    } else {
        msg += 'Type /checkin [place name] to set your location manually.';
    }
    
    addToTimeline(msg);
}

// Initialize
$(document).ready(function() {
    loadListeners();
    initialLoad();
    
    // Load command metadata from server
    loadCommandMeta();
    
    // Load persisted cards and conversation from localStorage
    loadTimeline();
    restoreConversation();
    
    // Fetch daily reminder (shows once per day at top)
    fetchReminder();
    
    // Scroll to bottom after loading persisted content
    scrollToBottom();
    
    // Show cached context immediately
    showCachedContext();
    
    // Then try to get fresh location/context
    getLocationAndContext();
    
    // Update timestamps every minute
    setInterval(updateTimestamps, 60000);
    
    // Update timestamps and refresh location when page becomes visible (PWA reopen)
    document.addEventListener('visibilitychange', function() {
        if (!document.hidden) {
            updateTimestamps();
            // Server pushes updates via /ping, no need for client-side showPresence
            getLocationAndContext();
        }
    });
    
    // Initialize step counter
    stepDetector.init();
    
    // Check for motion vs GPS stuck periodically
    setInterval(checkMotionGpsStuck, 10000);
    
    // Request motion permission on first tap (iOS)
    if (stepDetector.needsPermission) {
        document.body.addEventListener('click', function() {
            if (stepDetector.needsPermission) {
                stepDetector.requestPermission();
                stepDetector.needsPermission = false;
            }
        }, { once: true });
    }
});

function showCachedContext() {
    if (state.context) {
        displayContext(state.context, true); // Force update from cache
    } else {
        // Nothing cached - show welcome
        showWelcome();
    }
}

// Show brief presence acknowledgment on app reopen
function showPresence() {
    var now = new Date();
    var time = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    
    // Always show something on reopen
    if (state.context && state.context.location) {
        var ctx = state.context;
        var parts = ['🕐 ' + time];
        
        // Location - just street name
        if (ctx.location.name) {
            var loc = ctx.location.name.split(',')[0];
            parts.push(loc);
        }
        
        // Weather - extract from condition string
        if (ctx.weather && ctx.weather.condition) {
            var tempMatch = ctx.weather.condition.match(/-?\d+°C/);
            if (tempMatch) parts.push(tempMatch[0]);
        }
        
        // Prayer - extract short version
        if (ctx.prayer && ctx.prayer.display) {
            var prayerShort = ctx.prayer.display.replace('🕌 ', '').split(' · ')[0];
            parts.push('🕌 ' + prayerShort);
        }
        
        addToTimeline(parts.join(' · '));
    } else {
        // No cached context - show we're working on it  
        addToTimeline('🕐 ' + time + ' · 📡 Getting location...');
    }
}

function showWelcome() {
    var welcome = 'Welcome to Malten\n';
    welcome += 'Spatial AI for the real world.\n\n';
    welcome += 'Your context-aware assistant that knows where you are and what\'s around you.\n\n';
    welcome += '📍 Where you are (street, postcode)\n';
    welcome += '⛅ Weather + rain forecast\n';
    welcome += '🕌 Prayer times\n';
    welcome += '🚏 Live bus/train arrivals\n';
    welcome += '☕ Nearby cafes, restaurants, shops\n\n';
    welcome += '{enable_location}';
    
    displayContext({ html: welcome, places: {} }, true, true); // Force update, needs action
}

function showLocationNeeded(reason) {
    var msg = '📍 Location needed\n\n';
    if (reason === 'denied') {
        msg += 'Location permission was denied.\n\n';
        msg += 'To enable: Settings → Privacy → Location\n';
        msg += 'Then refresh this page.';
    } else if (reason === 'unavailable') {
        msg += 'Location is unavailable.\n\n';
        msg += 'Check your device\'s location settings.';
    } else if (reason === 'timeout') {
        msg += 'Location timed out.\n\n';
        msg += 'Try again or check your connection.';
    } else {
        msg += '{enable_location}';
    }
    displayContext(msg, true, true); // Force update, needs action
}

// Handle check-in selection (server pushes the prompt via WebSocket)
$(document).on('click', '.checkin-option, .checkin-link', function(e) {
    e.preventDefault();
    var name = $(this).data('name') || $(this).data('place');
    var lat = parseFloat($(this).data('lat')) || state.lat;
    var lon = parseFloat($(this).data('lon')) || state.lon;
    
    // Send check-in as a command
    $.post(commandUrl, {
        prompt: '/checkin ' + name,
        stream: getStream(),
        lat: lat,
        lon: lon
    });
    
    // Update local state and add to timeline
    state.checkIn(name, lat, lon);
    var isSaved = state.savedPlaces && state.savedPlaces[name];
    addToTimeline('📍 Checked in to ' + name + (isSaved ? ' ⭐' : ''));
    
    // Remove the prompt card from DOM and state
    var $li = $(this).closest('li');
    var cardTime = parseInt($li.find('.card').data('timestamp'));
    if (cardTime) {
        state.cards = state.cards.filter(function(c) { return c.time !== cardTime; });
        state.save();
    }
    $li.remove();
});

$(document).on('click', '.checkin-dismiss', function(e) {
    e.preventDefault();
    var $li = $(this).closest('li');
    var cardTime = parseInt($li.find('.card').data('timestamp'));
    if (cardTime) {
        state.cards = state.cards.filter(function(c) { return c.time !== cardTime; });
        state.save();
    }
    $li.remove();
});
