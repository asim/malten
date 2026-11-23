var agentMarkdown = window.markdownit && window.markdownit({ html: false }).disable('image');

var commandUrl = "/commands";
var messageUrl = "/messages";
var streamUrl = "/streams";
var eventUrl = "/events";
var limit = 25;
var last = timeAgo();
var typing = false;
var maltenMention = /^[@\/]malten(?:[:,])?\s+/i;
var maxMessageBytes = 4096;
var utf8 = new TextEncoder();
var maxMessages = 1000;
var seen = {};
var streams = {};
var source;
var id = ID();

String.prototype.parseURL = function(embed) {
    return this.replace(/[A-Za-z]+:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:%&~\?\/.=]+/g, function(url) {
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

function relativeTime(created, now) {
    var minutes = Math.floor((now - Number(created) / 1e6) / 60000);
    if (minutes < 1) return 'Just now';
    if (minutes < 60) return minutes + ' min ago';
    var hours = Math.floor(minutes / 60);
    if (hours < 24) return hours + (hours === 1 ? ' hour ago' : ' hours ago');
    var days = Math.floor(hours / 24);
    return days + (days === 1 ? ' day ago' : ' days ago');
}

function updateComposer() {
    var prompt = document.getElementById('prompt');
    prompt.style.height = 'auto';
    prompt.style.height = prompt.scrollHeight + 'px';
    var text = prompt.value.replace(maltenMention, '');
    var excess = utf8.encode(text).length - maxMessageBytes;
    var warning = excess > 0 ? excess + ' bytes over the limit' : '';
    prompt.setCustomValidity(warning);
    document.getElementById('chars').textContent = warning;
}

function clearMessages() {
    document.getElementById('messages').innerHTML = "";
    last = timeAgo();
    seen = {};
};

function clipMessages() {
    var list = document.getElementById('messages');
    while (list.length > maxMessages) {
        list.removeChild(list.lastChild);
    }
};

function escapeHTML(str) {
    var div = document.createElement('div');
    div.style.display = 'none';
    str = str.replace(/(?:\r\n|\r|\n)/g, '<br>');
    div.innerHTML = str;
    //div.appendChild(document.createTextNode(str));
    return div.innerHTML;
};

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

        var item = document.createElement('li');

        var timestamp = document.createElement('time');
        var posted = new Date(Number(array[i].Created) / 1e6);
        timestamp.className = 'time';
        timestamp.dataset.time = array[i].Created;
        timestamp.dateTime = posted.toISOString();
        timestamp.title = posted.toLocaleString();
        timestamp.textContent = relativeTime(array[i].Created, Date.now());
        var d2 = document.createElement('div');
        d2.className = 'message';
        if (array[i].Type === 'command' || array[i].Source === 'https://micro.mu/agent/malten') {
            if (array[i].Source) {
                var author = document.createElement('a');
                author.href = array[i].Source;
                author.textContent = 'Malten';
                item.appendChild(author);
            }
            if (array[i].Type !== 'command' && agentMarkdown) {
                d2.classList.add('agent-reply');
                d2.innerHTML = agentMarkdown.render(array[i].Text);
            } else {
                d2.style.whiteSpace = 'pre-wrap';
                d2.textContent = (array[i].Type === 'command' ? '@malten ' : '') + array[i].Text;
            }
        } else if (array[i].Source && array[i].Id.startsWith('reminder-')) {
            item.className = 'daily-reminder';
            item.dataset.expires = Number(array[i].Created) / 1e6 + 86400000;
            var source = document.createElement('a');
            source.href = array[i].Source;
            source.textContent = 'Daily Reminder';
            var passage = array[i].Text.split(/\r?\n\s*\n/);
            d2.appendChild(source);
            if (passage.length > 1) {
                var reference = document.createElement('p');
                reference.className = 'reference';
                reference.textContent = passage.shift();
                d2.appendChild(reference);
            }
            var verse = document.createElement('blockquote');
            verse.textContent = passage.join('\n\n');
            d2.appendChild(verse);
        } else {
            d2.innerHTML = escapeHTML(tagText(array[i].Text)).parseURL(embed).parseHashTag();
            if (array[i].Source) {
                var link = document.createElement('a');
                link.href = array[i].Source;
                link.textContent = 'Source';
                d2.appendChild(document.createElement('br'));
                d2.appendChild(link);
            }
        }
        item.appendChild(d2);

        if (array[i].Metadata != null && array[i].Metadata.Type != "player") {
            var a1 = document.createElement('a');
            var a2 = document.createElement('a');
            var d3 = document.createElement('div');
            var d4 = document.createElement('div');
            var d5 = document.createElement('div');
            var img = document.createElement('img');

            a1.innerHTML = array[i].Metadata.Site + ": " + array[i].Metadata.Title;
            a1.href = array[i].Metadata.Url;
            a2.href = array[i].Metadata.Url;
            d3.className = 'image';
            d4.className = 'title';
            d5.className = 'desc';
            img.src = array[i].Metadata.Image;
            a2.appendChild(img);
            d3.appendChild(a2);
            d4.appendChild(a1);
            d5.innerHTML = array[i].Metadata.Description;
            item.appendChild(d3);
            item.appendChild(d4);
            item.appendChild(d5);
        };

        item.id = 'message-' + array[i].Id;
        var permalink = document.createElement('a');
        var url = new URL(window.location.href);
        url.searchParams.set('message', array[i].Id);
        permalink.href = url.href;
        permalink.appendChild(timestamp);
        var footer = document.createElement('div');
        footer.className = 'message-footer';
        footer.appendChild(permalink);
        item.appendChild(footer);
        addTranslation(item, footer, array[i]);

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

function getSpeech() {
    var speak = document.getElementById("speak");
    var words = document.getElementById("words");
    var text = document.getElementById("prompt");

    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;

    let recognition = new SpeechRecognition();
    recognition.onstart = () => {
        console.log("mic on");
	words.innerText = "Start speaking";
	speak.disabled = true;
    }
    recognition.onspeechend = () => recognition.stop();
    recognition.onend = () => {
        words.innerText = "";
        speak.disabled = false;
    };
    recognition.onresult = (result) => {
        var transcript = result.results[0][0].transcript.trim();
        if (!transcript) return;
        var start = text.selectionStart;
        var end = text.selectionEnd;
        var prefix = start && !/\s/.test(text.value[start - 1]) ? ' ' : '';
        var suffix = end < text.value.length && !/\s/.test(text.value[end]) ? ' ' : '';
        var insertion = prefix + transcript + suffix;
        if (insertion.trim()) text.setRangeText(insertion, start, end, 'end');
        updateComposer();
        text.focus();
    };

    recognition.start();
}

function getStream() {
    var stream = window.location.hash.replace('#', '');

    if (stream.length == 0) {
        stream = "_";
    }

    return stream
}

function getStreams() {
    return $.get(streamUrl).done(function(data) { streams = data; });
}

function showPublicStreams() {
    var list = document.getElementById('public-streams');
    var status = document.getElementById('streams-status');
    list.textContent = '';
    status.textContent = 'Loading streams…';
    getStreams().done(function(data) {
        list.textContent = '';
        Object.keys(data).sort().forEach(function(name) {
            if (data[name].Private) return;
            var link = document.createElement('a');
            link.textContent = name === '_' ? 'Home' : name;
            link.href = name === '_' ? '/' : '/#' + name;
            link.onclick = function() { document.getElementById('join').close(); };
            var row = document.createElement('li');
            row.appendChild(link);
            list.appendChild(row);
        });
        status.textContent = list.children.length ? '' : 'No public streams.';
    }).fail(function() { status.textContent = 'Could not load public streams.'; });
}

function gotoStream(t) {
    var stream = document.getElementById('goto').elements['gstream'].value.replace(/^#+/, '');
    if (stream.length > 0) {
        document.getElementById('goto').elements['gstream'].value = '';
        document.getElementById("join").close();
        window.location = location.protocol + '//' + location.hostname + (location.port ? ':' + location.port : '') + '/#' + stream;
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
            window.location = location.protocol + '//' + location.host + '/#' + data.stream;
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

    }, false);

    document.getElementById("prompt").addEventListener("keydown", function() {
        stop();
    }, false);

    document.getElementById("prompt").addEventListener("input", updateComposer);
    updateComposer();
    var dialog = document.getElementById('join');
    document.getElementById('open-join').onclick = function() { dialog.showModal(); showPublicStreams(); };
    document.getElementById('close-join').onclick = function() { dialog.close(); };

    window.addEventListener("beforeunload", function(e) {
        if (source != undefined) {
            source.close;
        }
    });

    document.getElementById("speak").addEventListener("click", function(ev) {
	ev.preventDefault();
	getSpeech();
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
        params += "&stream=" + getStream();
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

function loadMessages(initial) {
    var params = "?direction=1&limit=" + limit + "&last=" + last;
    var form = document.getElementById('form');
    var stream = getStream();

    // stream provided?
    if (stream.length > 0) {
        params += "&stream=" + stream;
        form.elements["stream"].value = stream;
    } else {
        form.elements["stream"].value = '';
    };

    $.get(messageUrl + params, function(data) {
            if (stream !== getStream()) return;
            if (data != undefined && data.length > 0) {
                displayMessages(data, 1);
                clipMessages();
            }
            if (initial === true) locateMessage();
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

    var url = window.location.origin.replace("http", "ws") + eventUrl + "?stream=" + stream;
    source = new WebSocket(url);
    //source = new EventSource(eventUrl + "?stream=" + stream);

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
    var now = Date.now();
    document.querySelectorAll('#messages time').forEach(function(timestamp) {
        var label = relativeTime(timestamp.dataset.time, now);
        if (timestamp.textContent !== label) timestamp.textContent = label;
    });
    document.querySelectorAll('.daily-reminder[data-expires]').forEach(function(item) {
        if (Number(item.dataset.expires) <= Date.now()) item.remove();
    });
    if (typing == false) {
        loadMessages();
    };

    setTimeout(function() {
        pollMessages();
    }, 5000);
}

function postCommand(draft) {
    var prompt = document.getElementById('prompt');
    var status = document.getElementById('words');
    status.textContent = '';
    $.post(commandUrl, {prompt: draft.replace(maltenMention, ''), stream: getStream()})
        .done(function() {
            if (prompt.value === draft) prompt.value = '';
            updateComposer();
        })
        .fail(function() {
            status.textContent = 'Malten is unavailable. Please try again.';
        });
    return false;
}

function postMessage() {
    var prompt = document.getElementById('prompt');
    var status = document.getElementById('words');
    var draft = prompt.value;
    if (!draft) return false;
    var send = document.getElementById('send');
    send.disabled = true;
    status.textContent = '';
    $.post(messageUrl, {message: draft, stream: getStream()})
        .done(function() {
            if (prompt.value === draft) prompt.value = '';
            updateComposer();
            loadMessages();
        })
        .fail(function() { status.textContent = 'Could not send. Please try again.'; })
        .always(function() { send.disabled = false; });
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

    if (ev.Type == "message" || ev.Type == "command") {
        var events = [];
        events.push(ev);
        displayMessages(events, 1);
        clipMessages();
        return;
    }

    if (ev.Type != "event") {
        return
    }

    if (stream === getStream() && typeof ev.Observers === 'number') {
        setObservers(ev.Observers);
    }
    getStreams();
}

function setObservers(count) {
    var present = document.getElementById("present");
    present.textContent = count + " here";
}

function setCurrent() {
    var current = document.getElementById('current');
    current.href = window.location.href;

    if (window.location.hash.length > 0) {
        current.text = window.location.hash.replace('#', '');
	document.title = window.location.hash;
    } else {
        current.text = "";
	document.title = "Home";
    }
}

function loadStream() {
    document.getElementById("present").textContent = "";
    document.getElementById("link-status").textContent = "";
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
    var form = document.getElementById('form');
    var el = form.elements;

    if (el["prompt"].value.length <= 0) {
        return false;
    }

    if (el["prompt"].value.match(/^\s+$/)) {
        return false;
    }

    if (el["prompt"].value.match(maltenMention)) {
        return postCommand(el["prompt"].value);
    }

    return postMessage();
};

function tagText(text) {
    var parts = text.split(" ");
    for (j = 0; j < parts.length; j++) {
        if (parts[j] in streams) {
            parts[j] = '#' + parts[j];
        }
    }
    return parts.join(" ");
};

function locateMessage() {
    var id = new URLSearchParams(location.search).get('message');
    if (!id) return;
    var stream = getStream();
    var status = document.getElementById('link-status');
    function locate() {
        if (stream !== getStream()) return;
        var item = document.getElementById('message-' + id);
        if (item) {
            item.classList.add('linked-message');
            item.scrollIntoView({block: 'center'});
            return;
        }
        var oldest = document.querySelector('#messages > li:last-child time');
        var cursor = oldest ? oldest.dataset.time : Date.now() * 1e6;
        $.get(messageUrl, {stream: stream, direction: -1, last: cursor, limit: limit})
            .done(function(page) {
                if (stream !== getStream()) return;
                if (!page || !page.some(function(message) { return !(message.Id in seen); })) {
                    status.textContent = 'This message is no longer in the stream.';
                    return;
                }
                displayMessages(page, -1);
                locate();
            })
            .fail(function() { status.textContent = 'Could not load the linked message.'; });
    }
    locate();
}

function addTranslation(item, footer, message) {
    var button = document.createElement('button');
    var output = document.createElement('div');
    var translated = '';
    button.type = 'button';
    button.className = 'translate';
    button.textContent = 'Translate';
    output.className = 'translation';
    output.setAttribute('role', 'status');
    footer.appendChild(button);
    item.appendChild(output);

    button.onclick = async function() {
        if (translated) {
            output.hidden = !output.hidden;
            button.textContent = output.hidden ? 'Translate' : 'Hide translation';
            return;
        }
        button.disabled = true;
        output.textContent = 'Translating…';
        var failure = 'Translation is temporarily unavailable.';
        try {
            var language = navigator.language || 'en';
            var response = await fetch('/translate', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({id: message.Id, stream: message.Stream, to: language})
            }).catch(() => null);
            if (response && response.ok) {
                translated = (await response.json()).text;
            } else {
                if (response) {
                    if (response.status === 404) failure = 'This message is no longer available.';
                    if (response.status === 503) failure = 'Translation is unavailable on this server.';
                    if (response.status === 429) failure = 'Too many translation requests. Try again shortly.';
                } else {
                    failure = 'Could not connect to Malten to translate this message.';
                }
                translated = await translateInBrowser(message.Text, language);
            }
            output.lang = language;
            output.dir = 'auto';
            output.textContent = translated;
            button.textContent = 'Hide translation';
        } catch {
            output.textContent = failure;
        } finally {
            button.disabled = false;
        }
    };
}

async function translateInBrowser(text, language) {
    if (!window.LanguageDetector || !window.Translator ||
        await LanguageDetector.availability() !== 'available') throw new Error();
    var detector = await LanguageDetector.create();
    try {
        var matches = await detector.detect(text);
        var options = {sourceLanguage: matches[0].detectedLanguage, targetLanguage: language.split('-')[0]};
        if (options.sourceLanguage === options.targetLanguage) return text;
        if (await Translator.availability(options) !== 'available') throw new Error();
        var translator = await Translator.create(options);
        try {
            return await translator.translate(text);
        } finally {
            translator.destroy();
        }
    } finally {
        detector.destroy();
    }
}
