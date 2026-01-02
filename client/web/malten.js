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
                // Clear cards if version mismatch, keep location
                if (s.version !== this.version) {
                    this.lat = s.lat || null;
                    this.lon = s.lon || null;
                    this.cards = [];
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
                // Prune old cards on load
                var cutoff = Date.now() - (24 * 60 * 60 * 1000);
                this.cards = this.cards.filter(function(c) { return c.time > cutoff; });
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
            cards: this.cards
        }));
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
        
        // First context (arrival) - create full context card
        if (!oldCtx) {
            // Only if it has real content (not welcome message)
            if (newCtx.indexOf('📍') >= 0 || newCtx.indexOf('⛅') >= 0) {
                this.createCard(newCtx);
                displaySystemMessage(newCtx);
            }
            return;
        }
        
        // Extract location from context
        var oldLoc = this.extractLocation(oldCtx);
        var newLoc = this.extractLocation(newCtx);
        
        // Location changed significantly - create new context card
        if (newLoc && oldLoc && newLoc !== oldLoc) {
            this.createCard(newCtx);
            displaySystemMessage(newCtx);
            return; // Full context card replaces individual changes
        }
        
        // Bus stop - only log if changed and not recent
        var newStop = this.extractBusStop(newCtx);
        if (newStop) {
            var cardText = '🚏 ' + newStop;
            if (!this.hasRecentCard(cardText, 60)) {
                this.createCard(cardText);
            }
        }
        
        // Rain warning (and not recently logged)
        if (newCtx.indexOf('🌧️ Rain') >= 0 && oldCtx.indexOf('🌧️ Rain') < 0) {
            var rainMatch = newCtx.match(/🌧️ Rain[^\n]+/);
            if (rainMatch && !this.hasRecentCard(rainMatch[0], 30)) {
                this.createCard(rainMatch[0]);
                displaySystemMessage(rainMatch[0]);
            }
        }
        
        // Prayer time change (and not recently logged)
        var oldPrayer = this.extractPrayer(oldCtx);
        var newPrayer = this.extractPrayer(newCtx);
        if (newPrayer && oldPrayer && newPrayer !== oldPrayer) {
            var prayerCard = '🕌 ' + newPrayer;
            if (!this.hasRecentCard(prayerCard, 30)) {
                this.createCard(prayerCard);
                displaySystemMessage(prayerCard);
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
    extractBusStop: function(ctx) {
        var match = ctx.match(/🚏 ([^\n(]+)/);
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
    cards: []
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
            // Dedupe by ID
            if (ev.Id in seen) return;
            seen[ev.Id] = ev;
            
            // Check if this is a response to a pending command
            var pendingKey = Object.keys(pendingMessages)[0];
            if (pendingKey && pendingMessages[pendingKey]) {
                // Skip echoed input (server echoes back the question)
                if (ev.Text === pendingKey) {
                    return;
                }
                // Show response as simple card
                displaySystemMessage(ev.Text);
                state.createCard(ev.Text);
                delete pendingMessages[pendingKey];
                clipMessages();
                return;
            }
            
            // No pending question - display as system card
            state.createCard(ev.Text);
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
    loadMessages();
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

    // Handle new command locally (with or without slash)
    if (prompt.match(/^\/?new(\s|$)/i)) {
        form.elements["prompt"].value = '';
        createNewStream();
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

    // Post to /commands with location - response comes via WebSocket
    var data = {
        prompt: prompt,
        stream: getStream()
    };
    if (state.hasLocation()) {
        data.lat = state.lat;
        data.lon = state.lon;
    }
    
    // Track pending so we can match response
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
    contextDisplayed = true;
    // Render in persistent context div, not messages
    var ctx = document.getElementById('context');
    
    // Don't replace substantive cached context with empty/minimal response
    // Unless forceUpdate is true (e.g. initial load from cache)
    if (!forceUpdate && state.context && state.context.length > 50) {
        // Only update if new context has substantive content
        // Empty or minimal context (just welcome message) shouldn't replace bus times etc
        if (!text || text.length < 30 || text.indexOf('enable_location') >= 0) {
            console.log('Keeping cached context, new context too minimal:', text ? text.length : 0);
            return;
        }
    }
    
    var html = makeClickable(text).replace(/\n/g, '<br>');
    ctx.innerHTML = html;
    ctx.style.display = text ? 'block' : 'none';
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
    
    // Convert URLs to clickable links (for nearby results, etc)
    html = html.replace(/(https?:\/\/[A-Za-z0-9-_.]+\.[A-Za-z0-9-_:%&~\?\/.=#,@+]+)/g, function(url) {
        return '<a href="' + url + '" target="_blank">Open in Maps</a>';
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
            placeLine += '\n   <a href="' + mapUrl + '" target="_blank" class="map-link">Open in Maps</a>';
        }
        lines.push(placeLine);
    });
    
    displaySystemMessage(lines.join('\n\n'));
}

function displaySystemMessage(text, timestamp) {
    // Create a card in the messages area
    var time;
    if (timestamp) {
        time = new Date(timestamp).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    } else {
        time = new Date().toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    }
    var cardType = getCardType(text);
    var card = document.createElement('li');
    var html = makeClickable(text).replace(/\n/g, '<br>');
    card.innerHTML = '<div class="card ' + cardType + '">' +
        '<span class="card-time">' + time + '</span>' +
        html +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.insertBefore(card, messages.firstChild);
}

// Display a pending card for a question (awaiting answer)
function displayPendingCard(question) {
    var time = new Date().toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    var card = document.createElement('li');
    card.innerHTML = '<div class="card card-qa">' +
        '<span class="card-time">' + time + '</span>' +
        '<div class="card-question">' + escapeHTML(question) + '</div>' +
        '<div class="card-answer card-loading">...</div>' +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.insertBefore(card, messages.firstChild);
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

function displayCardAtEnd(text, timestamp) {
    // Append card at end (for loading history in order)
    var time = new Date(timestamp).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    var cardType = getCardType(text);
    var card = document.createElement('li');
    var html = makeClickable(text).replace(/\n/g, '<br>');
    card.innerHTML = '<div class="card ' + cardType + '">' +
        '<span class="card-time">' + time + '</span>' +
        html +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.appendChild(card);
}

function displayQACardAtEnd(question, answer, timestamp) {
    // Append Q+A card at end (for loading history)
    var time = new Date(timestamp).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    var cardType = getCardType(answer);
    var card = document.createElement('li');
    card.innerHTML = '<div class="card card-qa ' + cardType + '">' +
        '<span class="card-time">' + time + '</span>' +
        '<div class="card-question">' + escapeHTML(question) + '</div>' +
        '<div class="card-answer">' + makeClickable(answer).replace(/\n/g, '<br>') + '</div>' +
        '</div>';
    
    var messages = document.getElementById('messages');
    messages.appendChild(card);
}

function loadPersistedCards() {
    if (!state.cards || state.cards.length === 0) return;
    
    var lastDateStr = '';
    state.cards.forEach(function(c) {
        var dateStr = formatDateSeparator(c.time);
        if (dateStr && dateStr !== lastDateStr) {
            displayDateSeparator(dateStr);
            lastDateStr = dateStr;
        }
        if (c.question && c.answer) {
            displayQACardAtEnd(c.question, c.answer, c.time);
        } else if (c.text) {
            displayCardAtEnd(c.text, c.time);
        }
    });
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
        if (data.news && !state.hasRecentCard(data.news, 30)) {
            state.createCard(data.news);
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
            state.setLocation(pos.coords.latitude, pos.coords.longitude);
            // Ping returns context and news
            $.post(pingUrl, { lat: state.lat, lon: state.lon }).done(function(data) {
                if (data.context) {
                    state.setContext(data.context);
                    displayContext(data.context);
                }
                if (data.news && !state.hasRecentCard(data.news, 30)) {
                    state.createCard(data.news);
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
    
    document.getElementById('mic').addEventListener('click', toggleSpeech);
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
    window.addEventListener('scroll', function() {
        if (window.scrollY + window.innerHeight >= document.body.scrollHeight - 50) {
            loadMore();
        }
    });

    initSpeech();
}

// Register service worker
if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js')
        .catch(err => console.log('SW registration failed:', err));
}

// Initialize
$(document).ready(function() {
    loadListeners();
    initialLoad();
    
    // Load persisted cards from localStorage
    loadPersistedCards();
    
    // Show cached context immediately
    showCachedContext();
    
    // Then try to get fresh location/context
    getLocationAndContext();
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
