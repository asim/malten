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

String.prototype.parseURL = function() {
    return this.replace(/[A-Za-z]+:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:%&~\?\/.=]+/g, function(url) {
        var pretty = url.replace(/^http(s)?:\/\/(www\.)?/, '');
        return pretty.link(url);
    });
};

String.prototype.parseHashTag = function() {
    return this.replace(/[#]+[A-Za-z0-9-_]+/g, function(t) {
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
    str = str.replace(/(?:\r\n|\r|\n)/g, '<br>');
    div.innerHTML = str;
    return div.innerHTML;
}

function chars() {
    var i = document.getElementById('prompt').value.length;
    document.getElementById('chars').innerHTML = maxChars - i;
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
            list.insertBefore(item, list.firstChild);
        } else {
            list.appendChild(item);
        }
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
            // Dedupe
            if (ev.Id in seen) return;
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
}

function submitCommand() {
    var form = document.getElementById('form');
    var prompt = form.elements["prompt"].value.trim();
    
    if (prompt.length === 0) return false;

    // Post to /commands
    $.post(commandUrl, {
        prompt: prompt,
        stream: getStream()
    });

    form.elements["prompt"].value = '';
    chars();
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
    document.getElementById("prompt").addEventListener("keyup", chars);
    
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
