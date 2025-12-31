var commandUrl = "/commands";
var messageUrl = "/messages";
var streamUrl = "/streams";
var eventUrl = "/events";
var limit = 25;
var last = timeAgo();
var maxChars = 1024;
var maxMessages = 1000;
var seen = {};
var streams = {};
var ws = null;
var currentStream = null;
var reconnectTimer = null;
var pendingMessages = {};

String.prototype.parseURL = function() {
    return this.replace(/[A-Za-z]+:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:%&~\?\/.=#]+/g, function(url) {
        var pretty = url.replace(/^http(s)?:\/\/(www\.)?/, '');
        return pretty.link(url);
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

        if (direction >= 0) {
            list.appendChild(item);
        } else {
            list.insertBefore(item, list.firstChild);
        }
        seen[array[i].Id] = array[i];
    }

    if (direction >= 0 && array.length > 0) {
        last = array[array.length - 1].Created;
    }

    // Auto-scroll to bottom
    if (direction >= 0) {
        window.scrollTo(0, document.body.scrollHeight);
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

    // Handle /goto command locally
    var gotoMatch = prompt.match(/^\/goto\s+#?(.+)$/);
    if (gotoMatch) {
        form.elements["prompt"].value = '';
        window.location.hash = gotoMatch[1];
        return false;
    }

    // Handle /new command locally
    if (prompt.match(/^\/new(\s|$)/)) {
        form.elements["prompt"].value = '';
        createNewStream();
        return false;
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

function loadListeners() {

    
    $(window).scroll(function() {
        if ($(window).scrollTop() == $(document).height() - $(window).height()) {
            loadMore();
        }
    });

    window.addEventListener("hashchange", loadStream);
    shareListener();
}

// Initialize
$(document).ready(function() {
    loadListeners();
    loadStream();
});
