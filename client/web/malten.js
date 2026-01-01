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

// Consolidated state management
var state = {
    load: function() {
        try {
            var saved = localStorage.getItem('malten_state');
            if (saved) {
                var s = JSON.parse(saved);
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
        if (!oldCtx || !newCtx) return;
        
        // Extract bus stop from context
        var oldStop = this.extractBusStop(oldCtx);
        var newStop = this.extractBusStop(newCtx);
        
        // New bus stop approached (and not recently logged)
        if (newStop && newStop !== this.lastBusStop) {
            var cardText = '🚏 Arrived at ' + newStop;
            if (!this.hasRecentCard(cardText, 5)) {
                this.lastBusStop = newStop;
                this.createCard(cardText);
            }
        }
        
        // Rain warning (and not recently logged)
        if (newCtx.indexOf('🌧️ Rain') >= 0 && oldCtx.indexOf('🌧️ Rain') < 0) {
            var rainMatch = newCtx.match(/🌧️ Rain[^\n]+/);
            if (rainMatch && !this.hasRecentCard(rainMatch[0], 30)) {
                this.createCard(rainMatch[0]);
            }
        }
        
        // Prayer time change (and not recently logged)
        var oldPrayer = this.extractPrayer(oldCtx);
        var newPrayer = this.extractPrayer(newCtx);
        if (newPrayer && oldPrayer && newPrayer !== oldPrayer) {
            var prayerCard = '🕌 ' + newPrayer;
            if (!this.hasRecentCard(prayerCard, 30)) {
                this.createCard(prayerCard);
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
    var list = document.getElementById('messages');

    for (var i = 0; i < array.length; i++) {
        if (array[i].Id in seen) continue;

        var item = document.createElement('li');
        var html = escapeHTML(array[i].Text);
        var d1 = document.createElement('div');
        var d2 = document.createElement('div');
        d1.className = 'time';
        d2.className = 'message';
        d1.innerHTML = parseDate(array[i].Created);
        d1.style.display = 'none';
        d2.innerHTML = html.parseURL().parseHashTag();
        item.appendChild(d1);
        item.appendChild(d2);

        // Always prepend - newest at top
        list.insertBefore(item, list.firstChild);
        seen[array[i].Id] = array[i];
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
            // Skip if we already displayed this as a pending message
            if (ev.Text in pendingMessages) {
                seen[ev.Id] = ev;
                delete pendingMessages[ev.Text];
                return;
            }
            displayMessages([ev], 1);
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

function setCurrent() {
    var current = document.getElementById('current');
    var stream = getStream();
    current.innerText = "#" + stream;
    document.title = stream === "~" ? "Malten" : stream;
}

function loadStream() {
    setCurrent();
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

    // Handle goto command locally (with or without slash)
    var gotoMatch = prompt.match(/^\/?goto\s+#?(.+)$/i);
    if (gotoMatch) {
        form.elements["prompt"].value = '';
        window.location.hash = gotoMatch[1];
        return false;
    }

    // Handle new command locally (with or without slash)
    if (prompt.match(/^\/?new(\s|$)/i)) {
        form.elements["prompt"].value = '';
        createNewStream();
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

    // Display message immediately for responsiveness
    var tempId = 'local-' + Date.now();
    pendingMessages[prompt] = tempId;
    var msg = {
        Id: tempId,
        Text: prompt,
        Created: Date.now() * 1e6,
        Type: 'message',
        Stream: getStream()
    };
    displayMessages([msg], 1);

    // Post to /commands with location if available
    var data = {
        prompt: prompt,
        stream: getStream()
    };
    if (state.hasLocation()) {
        data.lat = state.lat;
        data.lon = state.lon;
    }
    $.post(commandUrl, data);

    form.elements["prompt"].value = '';
    return false;
}

function createNewStream() {
    $.post('/streams', {}, function(data) {
        if (data && data.stream) {
            window.location.hash = data.stream;
        }
    });
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
    // Server knows our location from session - just ask for context
    $.get(contextUrl).done(function(data) {
        if (data.context && data.context.length > 0) {
            state.setContext(data.context);
            displayContext(data.context);
        }
    });
}

function displayContext(text) {
    contextDisplayed = true;
    // Render in persistent context div, not messages
    var ctx = document.getElementById('context');
    var html = makeClickable(text).replace(/\n/g, '<br>');
    ctx.innerHTML = html;
    ctx.style.display = text ? 'block' : 'none';
}

// Make place names and counts clickable
function makeClickable(text) {
    var html = text;
    
    // Match "3 cafes", "2 restaurants" etc -> /nearby type
    html = html.replace(/(\d+)\s+(cafes?|restaurants?|pubs?|shops?|supermarkets?|pharmacies?|banks?|stations?)/gi, function(match, count, type) {
        var singular = type.replace(/s$/, '').replace(/ies$/, 'y');
        return '<a href="#" class="place-link" data-type="category" data-query="/nearby ' + singular + '">' + match + '</a>';
    });
    
    // Match [id:name] format for single places with known ID
    html = html.replace(/([☕🍽️💊🛒🏪])\s+\[([^:]+):([^\]]+)\]/g, function(match, icon, id, name) {
        return icon + ' <a href="#" class="place-link" data-type="place" data-id="' + id + '" data-name="' + name + '">' + name + '</a>';
    });
    
    return html;
}

// Handle clicks on place links
$(document).on('click', '.place-link', function(e) {
    e.preventDefault();
    var type = $(this).data('type');
    
    if (type === 'category') {
        // Category search like "3 cafes"
        document.getElementById('prompt').value = $(this).data('query');
    } else if (type === 'place') {
        // Single place with ID - fetch and expand inline
        var id = $(this).data('id');
        var link = $(this);
        fetchPlaceDetails(id, link);
    }
});

// Fetch place details and show as card
function fetchPlaceDetails(id, linkElement) {
    $.get('/place/' + id).done(function(data) {
        // Build details as a card message
        var details = '📍 ' + data.name;
        if (data.address) details += '\n' + data.address;
        if (data.postcode) details += ', ' + data.postcode;
        if (data.hours) details += '\n🕒 ' + data.hours;
        if (data.phone) details += '\n📞 ' + data.phone;
        details += '\n<a href="https://www.google.com/maps/search/' + encodeURIComponent(data.name) + '/@' + data.lat + ',' + data.lon + ',17z" target="_blank">Open in Maps</a>';
        
        // Show as card in messages
        displaySystemMessage(details);
    }).fail(function() {
        // Fallback to map link
        var name = linkElement.data('name') || linkElement.text();
        window.open('https://www.google.com/maps/search/' + encodeURIComponent(name), '_blank');
    });
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

function loadPersistedCards() {
    // Load cards from localStorage, oldest first
    if (state.cards && state.cards.length > 0) {
        state.cards.forEach(function(c) {
            displayCardAtEnd(c.text, c.time);
        });
    }
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
    state.setLocation(lat, lon);
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
            state.setLocation(pos.coords.latitude, pos.coords.longitude);
            // Ping returns context - no separate fetch needed
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
    } else if (!state.context) {
        showWelcome();
    }
}

function gotoStream(t) {
    var input = document.getElementById('goto').elements['gstream'];
    var stream = input.value.replace(/^#+/, '').trim();
    if (stream.length > 0) {
        input.value = '';
        window.location.hash = stream;
    }
    return false;
}

function shareListener() {
    var shareButton = document.getElementById("share");
    if (!shareButton) return;
    
    shareButton.addEventListener('click', function(e) {
        e.preventDefault();
        if (navigator.share) {
            navigator.share({ title: 'Malten', url: window.location.href });
        }
    });
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

    window.addEventListener("hashchange", loadStream);
    shareListener();
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
    loadStream();
    
    // Load persisted cards from localStorage
    loadPersistedCards();
    
    // Show cached context immediately
    showCachedContext();
    
    // Then try to get fresh location/context
    getLocationAndContext();
});

function showCachedContext() {
    if (state.context) {
        displayContext(state.context);
    } else {
        // Nothing cached - show welcome
        showWelcome();
    }
}

function showWelcome() {
    var hour = new Date().getHours();
    var greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening';
    
    var welcome = greeting + '\n\n';
    welcome += 'Enable location to see what\'s around you:\n';
    welcome += '• Live bus arrivals\n';
    welcome += '• Weather & prayer times\n';
    welcome += '• Nearby cafes, shops, pharmacies\n\n';
    welcome += 'Or ask me anything — "cafes nearby", "btc price", "reminder"';
    
    displayContext(welcome);
}
