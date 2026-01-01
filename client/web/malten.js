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
            }
        } catch(e) {}
    },
    save: function() {
        localStorage.setItem('malten_state', JSON.stringify({
            lat: this.lat,
            lon: this.lon,
            context: this.context,
            contextTime: this.contextTime
        }));
    },
    setLocation: function(lat, lon) {
        this.lat = lat;
        this.lon = lon;
        this.save();
    },
    setContext: function(ctx) {
        this.context = ctx;
        this.contextTime = Date.now();
        this.save();
    },
    hasLocation: function() {
        return this.lat && this.lon;
    },
    lat: null,
    lon: null,
    context: null,
    contextTime: 0
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

    // Post to /commands
    $.post(commandUrl, {
        prompt: prompt,
        stream: getStream()
    });

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
        { enableHighAccuracy: false, timeout: 30000, maximumAge: 300000 }
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
        { enableHighAccuracy: false, timeout: 10000, maximumAge: 60000 }
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
        { enableHighAccuracy: false, timeout: 30000, maximumAge: 60000 }
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
    ctx.innerHTML = text.replace(/\n/g, '<br>');
    ctx.style.display = text ? 'block' : 'none';
}

function displaySystemMessage(text) {
    var msg = {
        Id: 'system-' + Date.now(),
        Text: text,
        Created: Date.now() * 1e6,
        Type: 'message',
        Stream: getStream()
    };
    displayMessages([msg], 1);
}

function disableLocation() {
    if (locationWatchId) {
        navigator.geolocation.clearWatch(locationWatchId);
        locationWatchId = null;
    }
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
        { enableHighAccuracy: false, timeout: 10000, maximumAge: 300000 }
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
