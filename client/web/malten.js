var commandUrl = "/commands";
var messageUrl = "/messages";
var streamUrl = "/streams";
var eventUrl = "/events";
var pingUrl = "/ping";
var contextUrl = "/context";
var limit = 25;
var locationWatchId = null;
var last = timeAgo();
var maxChars = 1024;
var maxMessages = 1000;
var seen = {};
var streams = {};
var ws = null;
var currentStream = null;
var reconnectTimer = null;
var pendingMessages = {};
var activeConversation = null; // Currently active conversation card
var conversationTimeout = null;

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
    version: 2, // Increment to clear old state on format change
    load: function() {
        try {
            var saved = localStorage.getItem('malten_state');
            if (saved) {
                var s = JSON.parse(saved);
                // Clear cards if version mismatch, keep location and conversation
                if (s.version !== this.version) {
                    this.lat = s.lat || null;
                    this.lon = s.lon || null;
                    this.cards = [];
                    this.conversation = s.conversation || null;
                    this.save();
                    return;
                }
                this.lat = s.lat || null;
                this.lon = s.lon || null;
                this.context = s.context || null;
                this.contextTime = s.contextTime || 0;
                this.locationHistory = s.locationHistory || [];
                this.lastBusStop = s.lastBusStop || null;
                this.cards = s.cards || [];
                this.seenNewsUrls = s.seenNewsUrls || [];
                this.conversation = s.conversation || null;
                // Prune old cards on load
                var cutoff = Date.now() - (24 * 60 * 60 * 1000);
                this.cards = this.cards.filter(function(c) { return c.time > cutoff; });
                // Prune old news URLs (keep last 7 days)
                var newsCutoff = Date.now() - (7 * 24 * 60 * 60 * 1000);
                this.seenNewsUrls = this.seenNewsUrls.filter(function(n) { return n.time > newsCutoff; });
                // Clear conversation if older than 1 hour
                if (this.conversation && Date.now() - this.conversation.time > 3600000) {
                    this.conversation = null;
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
            locationHistory: this.locationHistory.slice(-20),
            lastBusStop: this.lastBusStop,
            cards: this.cards,
            seenNewsUrls: this.seenNewsUrls,
            conversation: this.conversation
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
        var oldContext = this.context;
        this.context = ctx;
        this.contextTime = Date.now();
        this.save();
        
        // Detect significant changes and create cards
        this.detectChanges(oldContext, ctx);
    },
    detectChanges: function(oldCtx, newCtx) {
        if (!newCtx) return;
        
        // First context - show initial location
        if (!oldCtx) {
            var loc = this.extractLocation(newCtx);
            if (loc) {
                this.createCard('📍 ' + loc);
            }
            return;
        }
        
        // Location/street changed
        var oldLoc = this.extractLocation(oldCtx);
        var newLoc = this.extractLocation(newCtx);
        if (newLoc && oldLoc && newLoc !== oldLoc) {
            var oldStreet = oldLoc.split(',')[0];
            var newStreet = newLoc.split(',')[0];
            if (newStreet !== oldStreet) {
                this.createCard('📍 ' + newStreet);
            }
        }
        
        // Rain warning
        if (newCtx.indexOf('🌧️ Rain') >= 0 && oldCtx.indexOf('🌧️ Rain') < 0) {
            var rainMatch = newCtx.match(/🌧️ Rain[^\n]+/);
            if (rainMatch && !this.hasRecentCard(rainMatch[0], 30)) {
                this.createCard(rainMatch[0]);
            }
        }
        
        // Prayer time change
        var oldPrayer = this.extractPrayer(oldCtx);
        var newPrayer = this.extractPrayer(newCtx);
        if (newPrayer && oldPrayer && newPrayer !== oldPrayer) {
            var prayerCard = '🕌 ' + newPrayer;
            if (!this.hasRecentCard(prayerCard, 30)) {
                this.createCard(prayerCard);
            }
        }
        
        // Bus arriving soon (< 3 mins)
        var busMatch = newCtx.match(/(\d+) → ([^\n]+) in (\d+)m/);
        if (busMatch) {
            var mins = parseInt(busMatch[3]);
            if (mins <= 3) {
                var busCard = '🚌 ' + busMatch[1] + ' → ' + busMatch[2] + ' in ' + mins + 'm';
                if (!this.hasRecentCard(busCard, 5)) {
                    this.createCard(busCard);
                }
            }
        }
        
        // Traffic disruption - new incident
        var oldDisrupt = oldCtx.match(/🚧[^\n]+/);
        var newDisrupt = newCtx.match(/🚧[^\n]+/);
        if (newDisrupt && (!oldDisrupt || oldDisrupt[0] !== newDisrupt[0])) {
            if (!this.hasRecentCard(newDisrupt[0], 60)) {
                this.createCard(newDisrupt[0]);
            }
        }
    },
    hasRecentCard: function(text, minutes) {
        // Check if a card with similar text exists within last N minutes
        var cutoff = Date.now() - (minutes * 60 * 1000);
        for (var i = 0; i < this.cards.length; i++) {
            if (this.cards[i].time > cutoff && this.cards[i].text === text) {
                return true;
            }
        }
        return false;
    },
    extractLocation: function(ctx) {
        var match = ctx.match(/📍 ([^\n]+)/);
        return match ? match[1].trim() : null;
    },
    extractPrayer: function(ctx) {
        var match = ctx.match(/🕌 ([^\n]+)/);
        return match ? match[1] : null;
    },
    createCard: function(text) {
        var card = {
            text: text,
            time: Date.now(),
            lat: this.lat,
            lon: this.lon
        };
        this.cards.push(card);
        // Prune cards older than 24 hours
        var cutoff = Date.now() - (24 * 60 * 60 * 1000);
        this.cards = this.cards.filter(function(c) { return c.time > cutoff; });
        this.save();
        displaySystemMessage(text);
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
    locationHistory: [],
    lastBusStop: null,
    cards: [],
    seenNewsUrls: []
};
state.load();

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

function parseDate(tdate) {
    var system_date = new Date(tdate / 1e6);
    return system_date.toLocaleTimeString();
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
        displaySystemMessage(sorted[i].Text, timestamp);
    }

    if (direction >= 0 && array.length > 0) {
        last = array[array.length - 1].Created;
    }
}

function loadMessages() {
    var stream = getStream();
    var params = "?direction=1&limit=" + limit + "&last=" + last + "&stream=" + stream;

    $.get(messageUrl + params, function(data) {
        if (data && data.length > 0) {
            displayMessages(data, 1);
            clipMessages();
            scrollToBottom();
        }
    });
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
            
            // Show response - append to conversation if active
            hideLoading();
            if (activeConversation) {
                appendToConversation('ai', ev.Text);
            } else {
                displaySystemMessage(ev.Text);
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

function loadStream() {
    connectWebSocket();
    
    var form = document.getElementById('form');
    form.elements["stream"].value = getStream();
    form.elements["prompt"].focus();
}

function initialLoad() {
    clearMessages();
    // Skip loadMessages() - personal timeline comes from localStorage
    // Server messages are stream-based, but user's view is personal
    connectWebSocket();
    
    var form = document.getElementById('form');
    form.elements["stream"].value = getStream();
    form.elements["prompt"].focus();
}

function submitCommand() {
    var form = document.getElementById('form');
    var prompt = form.elements["prompt"].value.trim();
    
    if (prompt.length === 0) return false;

    // Handle goto command locally (deprecated but keep for compatibility)
    var gotoMatch = prompt.match(/^\/?goto\s+#?(.+)$/i);
    if (gotoMatch) {
        form.elements["prompt"].value = '';
        return false;
    }

    // "new" command disabled - streams are geo-based now
    if (prompt.match(/^\/?new(\s|$)/i)) {
        form.elements["prompt"].value = '';
        displaySystemMessage('Stream creation disabled - location determines your stream');
        return false;
    }
    
    // Handle debug command locally
    if (prompt.match(/^\/?debug$/i)) {
        form.elements["prompt"].value = '';
        var info = '🔧 DEBUG\n';
        info += 'Stream: ' + getStream() + '\n';
        info += 'Location: ' + (state.hasLocation() ? state.lat.toFixed(4) + ', ' + state.lon.toFixed(4) : 'none') + '\n';
        info += 'Context cached: ' + (state.context ? state.context.length + ' chars' : 'none') + '\n';
        info += 'Cards: ' + (state.cards ? state.cards.length : 0) + '\n';
        info += 'Version: ' + (state.version || 'unknown');
        displaySystemMessage(info);
        return false;
    }

    // Handle ping on/off - enable location tracking client-side
    var pingMatch = prompt.match(/^\/?ping\s+(on|off)$/i);
    if (pingMatch) {
        var action = pingMatch[1].toLowerCase();
        if (action === 'on') {
            enableLocation();
        } else {
            disableLocation();
        }
        // Still send to server so it can respond
    }

    // Handle nearby - send fresh location before query
    var nearbyMatch = prompt.match(/^\/?nearby\s+/i);
    if (nearbyMatch && state.hasLocation()) {
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
    
    $.post(commandUrl, data);

    form.elements["prompt"].value = '';
    return false;
}

// Location functions
function enableLocation() {
    if (!navigator.geolocation) {
        displaySystemMessage("📍 Geolocation not supported in this browser");
        return;
    }

    // Check/request permission first
    if (navigator.permissions) {
        navigator.permissions.query({ name: 'geolocation' }).then(function(result) {
            if (result.state === 'denied') {
                displaySystemMessage("📍 Location permission denied. Please enable in browser settings.");
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
            $.post(pingUrl, { lat: state.lat, lon: state.lon }).done(function(data) {
                if (data.context) {
                    state.setContext(data.context);
                    displayContext(data.context);
                }
            });
            startLocationWatch();
        },
        function(err) {
            console.log("Location error:", err.message);
        },
        { enableHighAccuracy: true, timeout: 15000, maximumAge: 10000 }
    );
}

var lastPingSent = 0;
var pingInterval = 15000; // Update every 15 seconds when moving

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
    locationWatchId = navigator.geolocation.watchPosition(
        function(pos) {
            var now = Date.now();
            if (now - lastPingSent >= pingInterval) {
                lastPingSent = now;
                sendLocation(pos.coords.latitude, pos.coords.longitude);
            }
        },
        function(err) {
            console.log("Location watch error:", err.message);
        },
        { enableHighAccuracy: true, timeout: 30000, maximumAge: 10000 }
    );
}

function fetchContext() {
    // Use cached location if available, otherwise server's stored location
    var url = contextUrl;
    if (state.hasLocation()) {
        url = contextUrl + '?lat=' + state.lat + '&lon=' + state.lon;
    }
    $.get(url).done(function(data) {
        if (data.context && data.context.length > 0) {
            state.setContext(data.context);
            displayContext(data.context);
        }
    });
}

function displayContext(text, forceUpdate) {
    // Don't replace substantive cached context with empty/minimal response
    if (!forceUpdate && state.context && state.context.length > 50) {
        if (!text || text.length < 30 || text.indexOf('enable_location') >= 0) {
            console.log('Keeping cached context, new context too minimal:', text ? text.length : 0);
            return;
        }
    }
    
    // Build summary line (first line of each type)
    var summary = buildContextSummary(text);
    var fullHtml = makeClickable(text).replace(/\n/g, '<br>');
    
    // Update the context card (outside messages list)
    var contextCard = document.getElementById('context-card');
    var cardHtml = '<div class="context-summary">' + summary + '</div>' +
        '<div class="context-full">' + fullHtml + '</div>';
    
    if (!contextCard) {
        // Create context card before messages container
        var div = document.createElement('div');
        div.id = 'context-card';
        div.innerHTML = cardHtml;
        div.onclick = function() { this.classList.toggle('expanded'); };
        var container = document.getElementById('messages-area');
        container.parentNode.insertBefore(div, container);
    } else {
        var wasExpanded = contextCard.classList.contains('expanded');
        contextCard.innerHTML = cardHtml;
        if (wasExpanded) contextCard.classList.add('expanded');
    }
}

// Build one-line summary from context
function buildContextSummary(text) {
    var parts = [];
    
    // Location
    var locMatch = text.match(/📍 ([^,\n]+)/);
    if (locMatch) parts.push('📍 ' + locMatch[1]);
    
    // Weather temp
    var tempMatch = text.match(/(\d+)°C/);
    if (tempMatch) parts.push(tempMatch[0]);
    
    // Prayer - current one
    var prayerMatch = text.match(/🕌 ([A-Z][a-z]+) (now|in \d+m?)/);
    if (prayerMatch) parts.push('🕌 ' + prayerMatch[1] + ' ' + prayerMatch[2]);
    
    // Bus - first one
    var busMatch = text.match(/(\d+) → [^\n]+ in (\d+m)/);
    if (busMatch) parts.push('🚌 ' + busMatch[1] + ' ' + busMatch[2]);
    
    return parts.length > 0 ? parts.join(' · ') : 'Tap to expand';
}

// Make place names and counts clickable
function makeClickable(text) {
    var html = text;
    
    // Enable location button
    html = html.replace(/\{enable_location\}/g, 
        '<a href="#" class="enable-location-btn">📍 Enable location</a>');
    
    // Match {data} format - contains all places data (single or multiple separated by ;;)
    html = html.replace(/\{([^}]+)\}/g, function(match, data) {
        var places = data.split(';;');
        var firstName = places[0].split('|')[0] || '';
        var count = places.length;
        var label = count === 1 ? firstName : count + ' places';
        return '<a href="#" class="place-link" data-type="places" data-details="' + encodeURIComponent(data) + '">' + label + '</a>';
    });
    
    // Convert URLs to clickable links - detect news vs maps
    html = html.replace(/(https?:\/\/[A-Za-z0-9-_.]+\.[A-Za-z0-9-_:%&~\?\/.=#,@+]+)/g, function(url) {
        // News domains
        var newsPatterns = /bbc\.com|bbc\.co\.uk|theguardian\.com|news\.|cnn\.com|reuters\.|nytimes\.|sky\.com|independent\.co\.uk|telegraph\.co\.uk|mirror\.co\.uk|dailymail\.co\.uk|metro\.co\.uk|huffpost|washingtonpost|apnews|aljazeera/i;
        if (newsPatterns.test(url)) {
            return '<a href="' + url + '" target="_blank" class="article-link">Read article →</a>';
        }
        return '<a href="' + url + '" target="_blank" class="map-link">Map</a>';
    });
    
    return html;
}

// Handle enable location button click
$(document).on('click', '.enable-location-btn', function(e) {
    e.preventDefault();
    getLocationAndContext();
});

// Handle clicks on place links - toggle expansion
$(document).on('click', '.place-link', function(e) {
    e.preventDefault();
    var link = $(this);
    
    // Prevent double-click
    if (link.data('clicked')) return;
    link.data('clicked', true);
    
    var data = decodeURIComponent(link.data('details'));
    showPlacesCard(data);
});

// Show places card from embedded data (no API call)
function showPlacesCard(data) {
    var places = data.split(';;');
    var lines = [];
    
    places.forEach(function(placeData, idx) {
        var parts = placeData.split('|');
        var name = parts[0] || '';
        var placeLine = '📍 ' + name;
        var mapUrl = '';
        
        for (var i = 1; i < parts.length; i++) {
            var part = parts[i];
            if (part.startsWith('http')) {
                mapUrl = part;
            } else if (part) {
                placeLine += '\n   ' + part;
            }
        }
        if (mapUrl) {
            placeLine += '\n   <a href="' + mapUrl + '" target="_blank" class="map-link">Map</a>';
        }
        lines.push(placeLine);
    });
    
    displaySystemMessage(lines.join('\n\n'));
}

var displayedCards = {}; // Track displayed card text to prevent duplicates

function displaySystemMessage(text, timestamp) {
    // Dedupe - don't show same card text twice
    var textKey = text.substring(0, 100); // Use first 100 chars as key
    if (displayedCards[textKey]) {
        return;
    }
    displayedCards[textKey] = true;
    
    // Create a card in the messages area
    var ts = timestamp || Date.now();
    var timeStr = formatTimeAgo(ts);
    var cardType = getCardType(text);
    var card = document.createElement('li');
    var html = makeClickable(text).replace(/\n/g, '<br>');
    card.innerHTML = '<div class="card ' + cardType + '" data-timestamp="' + ts + '">' +
        '<span class="card-time">' + timeStr + '</span>' +
        html +
        '</div>';
    
    insertCardByTimestamp(card, ts);
}

// Insert card in chronological order (oldest at top, newest at bottom)
function insertCardByTimestamp(card, timestamp, shouldScroll) {
    var messages = document.getElementById('messages');
    
    // Always append new cards at the end (bottom) for chat-like flow
    messages.appendChild(card);
    
    // Only scroll if explicitly requested (user-initiated)
    if (shouldScroll) {
        scrollToBottom();
    }
}

function scrollToBottom() {
    var area = document.getElementById('messages-area');
    if (area) area.scrollTop = area.scrollHeight;
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

function displayUserMessage(text) {
    // If we have an active conversation, append to it
    if (activeConversation) {
        appendToConversation('user', text);
        return;
    }
    
    // Otherwise create a new conversation card
    createConversationCard(text);
}

// Create a new conversation card with the user's message
function createConversationCard(text) {
    var ts = Date.now();
    var card = document.createElement('li');
    card.className = 'conversation-item';
    card.innerHTML = '<div class="card conversation-card active" data-timestamp="' + ts + '">' +
        '<div class="convo-thread">' +
        '<div class="convo-msg convo-user">' + escapeHTML(text) + '</div>' +
        '<div class="convo-msg convo-ai convo-loading">...</div>' +
        '</div>' +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.appendChild(card);
    scrollToBottom();
    
    activeConversation = card;
    
    // Save conversation to state
    state.conversation = { time: ts, messages: [{ role: 'user', text: text }] };
    state.save();
    
    resetConversationTimeout();
}

// Append a message to the active conversation
function appendToConversation(role, text) {
    if (!activeConversation) return;
    
    var thread = activeConversation.querySelector('.convo-thread');
    if (!thread) return;
    
    // Remove loading indicator if present
    var loading = thread.querySelector('.convo-loading');
    if (loading) loading.remove();
    
    // Add the message
    var msgClass = role === 'user' ? 'convo-user' : 'convo-ai';
    var html = role === 'user' ? escapeHTML(text) : makeClickable(text).replace(/\n/g, '<br>');
    
    var msg = document.createElement('div');
    msg.className = 'convo-msg ' + msgClass;
    msg.innerHTML = html;
    thread.appendChild(msg);
    
    // Add loading indicator if this was a user message
    if (role === 'user') {
        var loadingDiv = document.createElement('div');
        loadingDiv.className = 'convo-msg convo-ai convo-loading';
        loadingDiv.textContent = '...';
        thread.appendChild(loadingDiv);
    }
    
    // Save to state
    if (state.conversation) {
        state.conversation.messages.push({ role: role, text: text });
        state.save();
    }
    
    scrollToBottom();
    resetConversationTimeout();
}

// Reset the conversation timeout (ends conversation after inactivity)
function resetConversationTimeout() {
    if (conversationTimeout) clearTimeout(conversationTimeout);
    conversationTimeout = setTimeout(endConversation, 60000); // 1 minute timeout
}

// End the active conversation
function endConversation() {
    if (activeConversation) {
        var cardDiv = activeConversation.querySelector('.conversation-card');
        if (cardDiv) cardDiv.classList.remove('active');
        
        // Remove any lingering loading indicator
        var loading = activeConversation.querySelector('.convo-loading');
        if (loading) loading.remove();
    }
    activeConversation = null;
    if (conversationTimeout) {
        clearTimeout(conversationTimeout);
        conversationTimeout = null;
    }
}

// Restore conversation from state on load
// Unused pending card functions kept for compatibility
function displayPendingCard(question) {
    var ts = Date.now();
    var card = document.createElement('li');
    card.innerHTML = '<div class="card card-qa" data-timestamp="' + ts + '">' +
        '<span class="card-time">' + formatTimeAgo(ts) + '</span>' +
        '<div class="card-question">' + escapeHTML(question) + '</div>' +
        '<div class="card-answer card-loading">...</div>' +
        '</div>';
    
    insertCardByTimestamp(card, ts);
    return card;
}

// Update pending card with answer
function updateCardWithAnswer(card, question, answer) {
    var answerDiv = card.querySelector('.card-answer');
    if (answerDiv) {
        answerDiv.classList.remove('card-loading');
        answerDiv.innerHTML = makeClickable(answer).replace(/\n/g, '<br>');
    }
    // Update card type based on answer content
    var cardDiv = card.querySelector('.card');
    if (cardDiv) {
        var type = getCardType(answer);
        if (type) cardDiv.classList.add(type);
    }
}

function displayCard(text, timestamp) {
    // Dedupe - don't show same card text twice
    var textKey = text.substring(0, 100);
    if (displayedCards[textKey]) {
        return;
    }
    displayedCards[textKey] = true;
    
    var cardType = getCardType(text);
    var card = document.createElement('li');
    var html = makeClickable(text).replace(/\n/g, '<br>');
    card.innerHTML = '<div class="card ' + cardType + '" data-timestamp="' + timestamp + '">' +
        '<span class="card-time">' + formatTimeAgo(timestamp) + '</span>' +
        html +
        '</div>';
    insertCardByTimestamp(card, timestamp);
}

function displayQACard(question, answer, timestamp) {
    var cardType = getCardType(answer);
    var card = document.createElement('li');
    card.innerHTML = '<div class="card card-qa ' + cardType + '" data-timestamp="' + timestamp + '">' +
        '<span class="card-time">' + formatTimeAgo(timestamp) + '</span>' +
        '<div class="card-question">' + escapeHTML(question) + '</div>' +
        '<div class="card-answer">' + makeClickable(answer).replace(/\n/g, '<br>') + '</div>' +
        '</div>';
    insertCardByTimestamp(card, timestamp);
}

function loadPersistedCards() {
    if (!state.cards || state.cards.length === 0) return;
    
    // Sort oldest first for chronological display
    var sorted = state.cards.slice().sort(function(a, b) { return a.time - b.time; });
    
    sorted.forEach(function(c) {
        if (c.text) {
            displayCard(c.text, c.time);
        }
    });
}

function restoreConversation() {
    if (!state.conversation || !state.conversation.messages) return;
    
    // Don't restore if already have a conversation card
    if (document.querySelector('.conversation-card')) return;
    
    var ts = state.conversation.time;
    var card = document.createElement('li');
    card.className = 'conversation-item';
    
    var threadHtml = '';
    state.conversation.messages.forEach(function(msg) {
        var msgClass = msg.role === 'user' ? 'convo-user' : 'convo-ai';
        var html = msg.role === 'user' ? escapeHTML(msg.text) : makeClickable(msg.text).replace(/\n/g, '<br>');
        threadHtml += '<div class="convo-msg ' + msgClass + '">' + html + '</div>';
    });
    
    card.innerHTML = '<div class="card conversation-card" data-timestamp="' + ts + '">' +
        '<div class="convo-thread">' + threadHtml + '</div>' +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.appendChild(card);
    scrollToBottom();
}

function formatDateSeparator(timestamp) {
    var date = new Date(timestamp);
    var today = new Date();
    var yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);
    
    if (date.toDateString() === today.toDateString()) {
        return ''; // Today - no separator needed
    } else if (date.toDateString() === yesterday.toDateString()) {
        return 'Yesterday';
    } else {
        return date.toLocaleDateString([], { weekday: 'long' });
    }
}

function displayDateSeparator(text) {
    var li = document.createElement('li');
    li.innerHTML = '<div class="date-separator">' + text + '</div>';
    document.getElementById('messages').appendChild(li);
}

function getCardType(text) {
    if (text.indexOf('🚏') >= 0 || text.indexOf('🚌') >= 0) return 'card-transport';
    if (text.indexOf('🌧️') >= 0 || text.indexOf('☀️') >= 0 || text.indexOf('⛅') >= 0) return 'card-weather';
    if (text.indexOf('🕌') >= 0) return 'card-prayer';
    if (text.indexOf('📍') >= 0) return 'card-location';
    return '';
}

function disableLocation() {
    if (locationWatchId) {
        navigator.geolocation.clearWatch(locationWatchId);
        locationWatchId = null;
    }
}

function showStatus(msg) {
    var el = document.getElementById('status');
    el.textContent = msg;
    el.classList.add('active');
}

function hideStatus() {
    var el = document.getElementById('status');
    el.classList.remove('active');
}

function sendLocation(lat, lon) {
    var oldStream = currentStream;
    state.setLocation(lat, lon);
    
    // Silently switch stream if geohash changed
    var newStream = getStream();
    if (newStream !== oldStream) {
        connectWebSocket(); // Reconnect to new stream silently
    }
    
    $.post(pingUrl, { lat: lat, lon: lon }).done(function(data) {
        if (data.context) {
            state.setContext(data.context);
            displayContext(data.context);
        }
    });
}

// Get location and refresh context
function getLocationAndContext() {
    if (!navigator.geolocation) {
        refreshContextFromState();
        return;
    }
    
    navigator.geolocation.getCurrentPosition(
        function(pos) {
            var oldStream = currentStream;
            state.setLocation(pos.coords.latitude, pos.coords.longitude);
            
            // Reconnect WebSocket if stream changed
            var newStream = getStream();
            if (newStream !== oldStream) {
                connectWebSocket();
            }
            
            // Ping returns context and news
            $.post(pingUrl, { lat: state.lat, lon: state.lon }).done(function(data) {
                if (data.context) {
                    state.setContext(data.context);
                    displayContext(data.context);
                }
            });
            startLocationWatch();
        },
        function(err) {
            console.log("Location error:", err.message);
            refreshContextFromState();
        },
        { enableHighAccuracy: true, timeout: 10000, maximumAge: 10000 }
    );
}

function refreshContextFromState() {
    if (state.hasLocation()) {
        fetchContext();
    } else if (state.context) {
        // Keep showing cached context - don't overwrite with empty
        console.log('No location, keeping cached context');
    } else {
        showWelcome();
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

    initSpeech();
}

// Register service worker
if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js')
        .catch(err => console.log('SW registration failed:', err));
}

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

// Initialize
$(document).ready(function() {
    loadListeners();
    initialLoad();
    
    // Load persisted cards and conversation from localStorage
    loadPersistedCards();
    restoreConversation();
    
    // Scroll to bottom after loading persisted content
    scrollToBottom();
    
    // Show cached context immediately
    showCachedContext();
    
    // Then try to get fresh location/context
    getLocationAndContext();
    
    // Update timestamps every minute
    setInterval(updateTimestamps, 60000);
    
    // Update timestamps when page becomes visible (PWA reopen)
    document.addEventListener('visibilitychange', function() {
        if (!document.hidden) {
            updateTimestamps();
        }
    });
});

function showCachedContext() {
    if (state.context) {
        displayContext(state.context, true); // Force update from cache
    } else {
        // Nothing cached - show welcome
        showWelcome();
    }
}

function showWelcome() {
    var hour = new Date().getHours();
    var greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening';
    
    var welcome = greeting + '\n\n';
    welcome += '{enable_location}\n\n';
    welcome += '• Live bus arrivals\n';
    welcome += '• Weather & prayer times\n';
    welcome += '• Nearby cafes, shops, pharmacies\n\n';
    welcome += 'Ask me anything — "cafes nearby", "pharmacies", "reminder"';
    
    displayContext(welcome, true); // Force update for welcome
}
