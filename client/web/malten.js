var commandUrl = "/commands";
var messageUrl = "/messages";
var streamUrl = "/streams";
var eventUrl = "/events";
var limit = 25;
var last = timeAgo();
var typing = false;
var maxChars = 1024;
var maxMessages = 1000;
var seen = {};
var streams = {};
var source;
var id = ID();

String.prototype.parseURL = function(embed) {
    return this.replace(/https?:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:%&~\?\/.=]+/g, function(url) {
        if (embed == true) {
            var match = url.match(/^.*(youtu.be\/|v\/|u\/\w\/|embed\/|watch\?v=|\&v=)([^#\&\?]*).*/);
            if (match && match[2].length == 11) {
                return '<div class="iframe">' +
                    '<iframe src="//www.youtube.com/embed/' + match[2] +
                    '" frameborder="0" allowfullscreen></iframe>' + '</div>';
            };
            if (url.match(/^.*giphy.com\/media\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+.gif$/)) {
                return '<div class="animation"><img src="' + url + '"></div>';
            }
        };
        var pretty = url.replace(/^http(s)?:\/\/(www\.)?/, '');
        return pretty.link(url);
    });
};

String.prototype.parseUsername = function() {
    return this.replace(/[@]+[A-Za-z0-9-_]+/g, function(u) {
        var username = u.replace("@", "");
        return u.link("http://twitter.com/" + username);
    });
};

String.prototype.parseHashTag = function() {
    return this.replace(/[#]+[A-Za-z0-9-_]+/g, function(t) {
        //var tag = t.replace("#","%23")
        var url = location.protocol + '//' + location.hostname + (location.port ? ':' + location.port : '');
        return t.link(url + '/' + t);
    });
};

function ID() {
    return ([1e7] + -1e3 + -4e3 + -8e3 + -1e11).replace(/[018]/g, c =>
        (c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> c / 4).toString(16)
    );
}

function timeAgo() {
    var ts = new Date().getTime() / 1000;
    return (ts - 86400) * 1e9;
};

function parseDate(tdate) {
    var system_date = new Date(tdate / 1e6);
    if (K.ie) {
        system_date = Date.parse(tdate.replace(/( \+)/, ' UTC$1'))
    }
    return system_date.toLocaleTimeString();;
};

// from http://widgets.twimg.com/j/1/widget.js
var K = function() {
    var a = navigator.userAgent;
    return {
        ie: a.match(/MSIE\s([^;]*)/)
    }
}();

function chars() {
    var i = document.getElementById('prompt').value.length;
    var c = maxChars;

    if (i > maxChars) {
        c = i - maxChars;
    } else {
        c = maxChars - i;
    }

    document.getElementById('chars').innerHTML = c;
};

function clearMessages() {
    document.getElementById('messages').innerHTML = "";
    last = timeAgo();
    seen = {};
};

function clipMessages() {
    var list = document.getElementById('messages');
    var bytes = Object.values(seen).reduce((sum, message) => sum + (message.Image || '').length + message.Text.length, 0);
    while (list.children.length > maxMessages || (bytes > 8 * 1024 * 1024 && list.children.length > 1)) {
        var item = list.lastElementChild, message = seen[item.dataset.id];
        if(message) { bytes -= (message.Image || '').length + message.Text.length; delete seen[item.dataset.id]; }
        item.remove();
    }
};

function escapeHTML(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML.replace(/(?:\r\n|\r|\n)/g, '<br>');
};

function safeURL(value) {
    try {
        var url = new URL(value);
        return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : '';
    } catch (_) { return ''; }
}

function displayMessages(array, direction) {
    var list = document.getElementById('messages');

    for (i = 0; i < array.length; i++) {
        if (array[i].Id in seen) {
            continue;
        };

        var embed = true;

        if (array[i].Metadata != null && array[i].Metadata.Type != "player") {
            embed = false;
        }

        // tagging
        array[i].Text = tagText(array[i].Text);

        var item = document.createElement('li');
        item.dataset.id = array[i].Id;
        var html = escapeHTML(array[i].Text);
        var d1 = document.createElement('div');
        var d2 = document.createElement('div');
        d1.className = 'time';
        d2.className = 'message';
        d1.innerHTML = parseDate(array[i].Created);
        d1.setAttribute('data-time', array[i].Created);

        d2.innerHTML = html.parseURL(embed).parseHashTag();
        item.appendChild(d1);
        item.appendChild(d2);
        if(validPhoto(array[i].Image)) { var photo = document.createElement('img'); photo.src = array[i].Image; photo.alt = 'Photo'; photo.className = 'attached-photo'; item.appendChild(photo); }

        if (array[i].Metadata != null && array[i].Metadata.Type != "player") {
            var a1 = document.createElement('a');
            var a2 = document.createElement('a');
            var d3 = document.createElement('div');
            var d4 = document.createElement('div');
            var d5 = document.createElement('div');
            var img = document.createElement('img');

            a1.textContent = array[i].Metadata.Site + ": " + array[i].Metadata.Title;
            a1.href = safeURL(array[i].Metadata.Url);
            a2.href = safeURL(array[i].Metadata.Url);
            d3.className = 'image';
            d4.className = 'title';
            d5.className = 'desc';
            img.src = safeURL(array[i].Metadata.Image);
            a2.appendChild(img);
            d3.appendChild(a2);
            d4.appendChild(a1);
            d5.textContent = array[i].Metadata.Description;
            item.appendChild(d3);
            item.appendChild(d4);
            item.appendChild(d5);
        };

        if (direction >= 0) {
            list.insertBefore(item, list.firstChild);
        } else {
            list.appendChild(item);
        }
        seen[array[i].Id] = array[i];
    }

    if (direction >= 0) {
        last = array[array.length - 1].Created;
    }
};

function getStream() {
    var stream = window.location.hash.replace('#', '');
    try { stream = decodeURIComponent(stream); } catch (_) {}

    if (stream.length == 0) {
        stream = "_";
    }

    return stream
}

function getStreams(fn) {
    $.get(streamUrl, function(data) {
            streams = data;
            var s = streams[getStream()];
            if (s != undefined) {
                setObservers(s.Observers);
		//setStreamsList();
            }
        })
        .fail(function(err) {
            console.log(err);
        })
        .done();
}

function setStreamsList() {
    var side = document.getElementById('side');
    side.innerText = '';

    for (const key in streams) {
	    var name = key;
	    var href = '/#' + name;

	    if (name == "_") {
		    href = "/";
            }	
	    var span = document.createElement('span');
	    var a = document.createElement('a');
	    a.innerText = name;
	    a.href = href
	    span.appendChild(a);
	    side.appendChild(span);
    }	
}

function gotoStream(t) {
    var stream = document.getElementById('goto').elements['gstream'].value.replace(/^#+/, '');
    if (stream.length > 0) {
        document.getElementById('goto').elements['gstream'].value = '';
        window.location = location.protocol + '//' + location.hostname + (location.port ? ':' + location.port : '') + '/#' + encodeURIComponent(stream);
        document.getElementById('goto').hidden = true;
        document.getElementById('goto-toggle').setAttribute('aria-expanded', 'false');
        clearMessages();
    };
    return false;
};

function newStream() {
    var form = document.getElementById('new-form');
    var priv = document.getElementById("private").checked;
    var stream = form.elements["stream"].value;
    var ttl = form.elements["ttl"].value

    $.post(streamUrl, {
            stream: stream,
            private: priv,
            ttl: ttl
        })
        .done(function(data) {
            window.location = location.protocol + '//' + location.host + '/#' + encodeURIComponent(data.stream);
            return false;
        })
        .fail(function() {
            alert("error creating stream");
            return false;
        })
    return false;
}

function loadListeners() {
    if (window.navigator.standalone) {
        $.ajaxSetup({
            isLocal: true
        });
    };

    $(window).scroll(function() {
        if ($(window).scrollTop() == $(document).height() - $(window).height()) {
            loadMore();
        }
    });

    document.getElementById("prompt").addEventListener("keyup", function() {
        start();
        chars();
    }, false);

    document.getElementById("prompt").addEventListener("keydown", function() {
        stop();
    }, false);

    shareListener();
    document.getElementById('goto-toggle').onclick = function(event) {
        event.preventDefault();
        var form = document.getElementById('goto'); form.hidden = !form.hidden;
        this.setAttribute('aria-expanded', String(!form.hidden));
        if(!form.hidden) document.getElementById('gstream').focus();
    };

    window.addEventListener("pagehide", function(e) {
        if (source != undefined) {
            source.close();
        }
    });


};

function loadMore() {
    var divs = document.getElementsByClassName('time');
    var oldest = new Date().getTime() * 1e6;

    if (divs.length > 0) {
        oldest = divs[divs.length - 1].getAttribute('data-time');
    }

    var params = "?direction=-1&limit=" + limit + "&last=" + oldest;

    if (window.location.hash.length > 0) {
        params += "&stream=" + encodeURIComponent(getStream());
    };

    $.get(messageUrl + params, function(data) {
            if (data != undefined && data.length > 0) {
                displayMessages(data, -1);
            }
        })
        .fail(function(err) {
            console.log(err);
        })
        .done();

    return false;
};

function loadMessages() {
    var params = "?direction=1&limit=" + limit + "&last=" + last;
    var form = document.getElementById('form');
    var stream = getStream();

    // stream provided?
    if (stream.length > 0) {
        params += "&stream=" + encodeURIComponent(stream);
        form.elements["stream"].value = stream;
    } else {
        form.elements["stream"].value = '';
    };

    $.get(messageUrl + params, function(data) {
            if (data != undefined && data.length > 0) {
                displayMessages(data, 1);
                clipMessages();
            }
        })
        .fail(function(err) {
            console.log(err);
        })
        .done();

    return false;
}

function observeEvents() {
    if (source != undefined) {
        source.close();
    }

    var stream = getStream();

    var url = window.location.origin.replace("http", "ws") + eventUrl + "?stream=" + encodeURIComponent(stream);
    source = new WebSocket(url);
    //source = new EventSource(eventUrl + "?stream=" + encodeURIComponent(stream));

    source.onopen = (event) => {
	console.log(event)
    }
    source.onmessage = (event) => {
        processEvent(stream, event)
    }
    source.onclose = (event) => {
        console.log(event)
    }
    source.onerror = (event) => {
        console.log(event)
    }
}

function pollMessages() {
    if (typing == false) {
        loadMessages();
    };

    setTimeout(function() {
        pollMessages();
    }, 5000);
}

var streamComposer, sending = false;
function initStreamComposer() {
    streamComposer = createComposer(document.getElementById('prompt'), function(message) { document.getElementById('words').textContent = message; });
    fetch('/config').then(r => r.json()).then(config => { document.getElementById('moderation').textContent = config.moderation ? 'Public posts and photos are sent to OpenAI for moderation before sharing.' : 'AI moderation is off.'; }).catch(() => {});
    window.addEventListener('pageshow', event => { if(event.persisted) observeEvents(); });
}
function postMessage(command) {
    if(sending || streamComposer.loading) return false;
    var text = document.getElementById('prompt');
    var words = document.getElementById('words');
    if(!text.value.trim() && !streamComposer.image) return false;
    if(command && streamComposer.image) { words.textContent = 'Send photos as a stream post.'; return false; }
    if(new TextEncoder().encode(text.value).length > 1024) { words.textContent = 'Message is too long (1,024 bytes maximum).'; return false; }
    sending = true; streamComposer.setBusy(true); text.disabled = true; document.getElementById('send').disabled = true; words.textContent = 'Sending…';
    var payload = {stream:getStream(), message:text.value, image:streamComposer.image};
    if(command) payload.prompt = text.value.replace(/^\/malten\s/, '');
    $.post(command ? commandUrl : messageUrl, payload).done(function() {
        text.value = ''; streamComposer.setImage(''); words.textContent = ''; chars(); loadMessages();
    }).fail(function(error) { words.textContent = error.responseText || 'Could not send. Your draft is still here.'; })
    .always(function() { sending = false; streamComposer.setBusy(false); text.disabled = false; document.getElementById('send').disabled = false; });
    return false;
}

function processEvent(stream, event) {
    console.log(event);

    if (event.data == undefined || event.data.length == 0) {
        return
    }

    var ev = JSON.parse(event.data);

    if (ev.Stream != stream) {
        return
    }

    if (ev.Type == "message") {
        var events = [];
        events.push(ev);
        displayMessages(events, 1);
        clipMessages();
        return;
    }

    if (ev.Type != "event") {
        return
    }

    if (ev.Text == "connect") {
        // user joined event 
    } else if (ev.Text == "close") {
        // user left event
    }

    // no stream data, try refresh
    getStreams();
}

function setObservers(count) {
    var present = document.getElementById("present");
    present.innerText = count + " here";
}

function setCurrent() {
    var current = document.getElementById('current');
    current.href = window.location.href;

    if (window.location.hash.length > 0) {
        current.text = getStream() === "_" ? "Public stream" : "#" + getStream();
	document.title = window.location.hash;
    } else {
        current.text = "Public stream";
	document.title = "Public streams | Malten";
    }
}

function loadStream() {
    getStreams()
    setCurrent();
    clearMessages();
    loadMessages();
    observeEvents();
}

function start() {
    typing = false;
};

function stop() {
    typing = true;
};

function submitCommand() {
    return postMessage(/^\/malten\s/.test(document.getElementById('prompt').value));
}

function tagText(text) {
    var parts = text.split(" ");
    for (j = 0; j < parts.length; j++) {
        if (parts[j] in streams) {
            parts[j] = '#' + parts[j];
        }
    }
    return parts.join(" ");
};

function shareListener() {
    var shareButton = document.getElementById("share");

    shareButton.addEventListener('click', event => {
        event.preventDefault();

        if (navigator.share) {
            navigator.share({
                    title: 'Malten',
                    url: window.location.href,
                }).then(() => {
                    console.log('Thanks for sharing!');
                })
                .catch(console.error);
        } else {
            navigator.clipboard.writeText(window.location.href).then(() => { document.getElementById('words').textContent = 'Link copied.'; }).catch(() => { window.prompt('Copy stream link', window.location.href); });
        }

        return false;
    });
}

function restoreStreamDraft() {
    try {
        var draft = JSON.parse(sessionStorage.getItem('malten-stream-draft-v1'));
        if (draft && draft.stream === getStream() && typeof draft.text === 'string') {
            document.getElementById('prompt').value = draft.text;
            streamComposer.setImage(draft.image);
            sessionStorage.removeItem('malten-stream-draft-v1');
            chars();
        }
    } catch (_) {}
}
