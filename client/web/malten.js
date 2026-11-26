var agentMarkdown = window.markdownit && window.markdownit({ html: false }).disable('image');

var messageUrl = "/messages";
var streamUrl = "/streams";
var eventUrl = "/events";
var limit = 25;
var last = timeAgo();
var typing = false;
var maxMessageBytes = 4096;
var utf8 = new TextEncoder();
var maxMessages = 1024;
var seen = {};
var streams = {};
var source;
var activeAsk;
var streamReply;
var id = ID();

String.prototype.parseURL = function(embed) {
    return this.replace(/[A-Za-z]+:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:@%&~\?\/.=]+/g, function(url) {
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
    var excess = utf8.encode(prompt.value).length - maxMessageBytes;
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
    while (list.children.length > maxMessages) {
        const id = list.lastElementChild.dataset.id;
        delete seen[id];
        list.lastElementChild.remove();
        document.querySelectorAll('.reply-quote[data-parent="' + CSS.escape(id) + '"]').forEach(quote => quote.replaceWith(replyQuote(null)));
    }
};

function escapeHTML(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML.replace(/\r\n|\r|\n/g, '<br>');
};

function messageLink(id) {
    const url = new URL(window.location.href);
    url.searchParams.set('message', id);
    return url.href;
}

function streamQuote(message, id) {
    if (!message) return null;
    let author = '';
    if (message.Source === muMaltenProfile) author = 'Malten';
    else if (message.Source && id.startsWith('reminder-')) author = 'Daily Reminder';
    return {text: (message.Type === 'command' ? '@malten ' : '') + message.Text, author, expires: message.Expires};
}

function appendReading(body, text) {
    const paragraphs = text.split(/\r?\n\s*\n/);
    if (paragraphs.length > 1) {
        const reference = document.createElement('p');
        reference.className = 'reference';
        reference.textContent = paragraphs.shift();
        body.append(reference);
    }
    const quote = document.createElement('blockquote');
    quote.dir = 'auto';
    quote.textContent = paragraphs.join('\n\n');
    body.append(quote);
}

function verseCommentary(link) {
    if (link.origin !== 'https://reminder.dev' || !/^\/quran\/\d+(?:\/\d+)?\/?$/.test(link.pathname)) return '';
    const url = new URL(link.href);
    url.searchParams.set('commentary', '1');
    return url.href;
}

function renderDailyReminder(body, message) {
    const byMalten = message.Source === muMaltenProfile;
    const source = document.createElement('a');
    source.href = message.Source;
    if (byMalten) {
        body.append('Daily reflection by ', maltenProfileLink('Malten'));
    } else {
        source.textContent = 'Daily Reminder';
        body.append(source);
    }

    const dated = source.origin === 'https://reminder.dev' && source.pathname.startsWith('/daily/');
    const readings = message.References || (dated && message.Metadata
        ? [{Title: 'Qur’an passage', Text: message.Metadata.Description, URL: message.Metadata.Url}] : []);
    if (!byMalten && (!dated || readings.some(r => r.Text === message.Text))) {
        appendReading(body, message.Text);
    } else {
        const reflection = document.createElement('p');
        reflection.className = 'reflection-text';
        reflection.dir = 'auto';
        reflection.textContent = message.Text;
        body.append(reflection);
    }

    let commentary = verseCommentary(source);
    const details = document.createElement('details');
    const summary = document.createElement('summary');
    summary.textContent = 'Explore the Islamic sources';
    details.append(summary);
    for (const reading of readings) {
        if (!reading.Text && reading.URL === message.Source) continue;
        const section = document.createElement('div');
        section.className = 'daily-reading';
        const link = document.createElement('a');
        if (reading.URL) link.href = reading.URL;
        const title = link.origin === 'https://reminder.dev' ? link : document.createElement('strong');
        title.textContent = reading.Title === 'Hadith' ? 'A teaching of the Prophet Muhammad (hadith)' : reading.Title;
        section.append(title);
        if (reading.Text && reading.Text !== message.Text) appendReading(section, reading.Text);
        details.append(section);
        if (reading.Title === 'Qur’an passage') commentary = verseCommentary(link);
    }
    if (details.children.length > 1) body.append(details);
    return commentary;
}

function displayMessages(array, direction) {
    var list = document.getElementById('messages');

    for (let i = 0; i < array.length; i++) {
        if (array[i].Id in seen) {
            continue;
        };

        const isReminder = array[i].Source && (array[i].Id.startsWith('reminder-') || array[i].Id.startsWith('reflection-'));
        var embed = true;

        if (array[i].Metadata != null && array[i].Metadata.Type != "player") {
            embed = false;
        }

        var item = document.createElement('li');
        var footer = document.createElement('div');
        footer.className = 'message-footer';
        var tafsirURL = '';

        var timestamp = document.createElement('time');
        var posted = new Date(Number(array[i].Created) / 1e6);
        timestamp.className = 'time';
        timestamp.dataset.time = array[i].Created;
        timestamp.dateTime = posted.toISOString();
        timestamp.title = posted.toLocaleString();
        timestamp.textContent = relativeTime(array[i].Created, Date.now());
        var d2 = document.createElement('div');
        d2.className = 'message';
        if (isReminder) {
            item.className = 'daily-reminder';
            item.dataset.expires = Number(array[i].Created) / 1e6 + 86400000;
            tafsirURL = renderDailyReminder(d2, array[i]);
        } else if (array[i].Type === 'command' || array[i].Source === muMaltenProfile) {
            if (array[i].Source) {
                item.appendChild(maltenProfileLink('Malten'));
            }
            if (array[i].Type !== 'command' && agentMarkdown) {
                d2.classList.add('agent-reply');
                d2.innerHTML = agentMarkdown.render(array[i].Text);
            } else {
                d2.style.whiteSpace = 'pre-wrap';
                d2.textContent = (array[i].Type === 'command' ? '@malten ' : '') + array[i].Text;
            }
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
        const message = array[i];
        if (message.Parent) {
            item.appendChild(replyQuote(streamQuote(message.Quote, message.Parent), message.Parent, messageLink(message.Parent)));
        }
        linkMaltenMentions(d2);
        item.appendChild(d2);

        if (!isReminder && array[i].Metadata != null && array[i].Metadata.Type != "player") {
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
        item.dataset.id = message.Id;
        if (message.Expires) item.dataset.expires = message.Expires;
        permalink.href = messageLink(message.Id);
        permalink.setAttribute('aria-label', 'Open reflection from ' + timestamp.title);
        permalink.appendChild(timestamp);
        footer.appendChild(permalink);
        const actions = document.createElement('div');
        actions.className = 'message-actions';
        actions.append(replyButton(() => streamReply.select(message.Id, streamQuote(message, message.Id))),
            askButton(message, tafsirURL));
        footer.appendChild(actions);
        item.appendChild(footer);

        if (direction >= 0) {
            list.insertBefore(item, list.firstChild);
        } else {
            list.appendChild(item);
        }
        seen[message.Id] = true;
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
    streamReply = replyComposer(document.getElementById('reply-draft'), document.getElementById('prompt'));
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

    var stream = getStream();
    var params = "?direction=-1&limit=" + limit + "&last=" + oldest;

    if (window.location.hash.length > 0) {
        params += "&stream=" + getStream();
    };

    $.get(messageUrl + params, function(data) {
            if (stream !== getStream()) return;
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
    document.querySelectorAll('#messages > li[data-expires]').forEach(function(item) {
        if (Number(item.dataset.expires) <= now) {
            delete seen[item.dataset.id];
            item.remove();
        }
    });
    document.querySelectorAll('.reply-quote[data-expires]').forEach(function(quote) {
        if (Number(quote.dataset.expires) <= now) quote.replaceWith(replyQuote(null));
    });
    if (typing == false) {
        loadMessages();
    };

    setTimeout(function() {
        pollMessages();
    }, 5000);
}

function postMessage() {
    var prompt = document.getElementById('prompt');
    var status = document.getElementById('words');
    var draft = prompt.value;
    if (!draft.trim()) return false;
    var send = document.getElementById('send');
    send.disabled = true;
    status.textContent = '';
    const stream = getStream();
    const parent = streamReply.parent;
    $.post(messageUrl, {message: draft, stream: stream, parent: parent})
        .done(function() {
            if (stream !== getStream()) return;
            if (prompt.value === draft && streamReply.parent === parent) {
                prompt.value = '';
                streamReply.clear();
            }
            updateComposer();
            loadMessages();
        })
        .fail(function(xhr) {
            if (stream !== getStream()) return;
            var plainText = xhr.getResponseHeader('Content-Type')?.startsWith('text/plain');
            status.textContent = plainText && xhr.responseText ? xhr.responseText.trim() : 'Could not send. Please try again.';
        })
        .always(function() { send.disabled = false; });
    return false;
}

function processEvent(stream, event) {
    console.log(event);

    if (event.data == undefined || event.data.length == 0) {
        return
    }

    var ev = JSON.parse(event.data);

    if (ev.Stream !== stream || stream !== getStream()) {
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
    streamReply?.clear();
    document.getElementById('ask-malten').close();
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

function askButton(message, tafsirURL) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'message-action';
    button.textContent = 'Ask Malten';
    button.setAttribute('aria-haspopup', 'dialog');
    button.onclick = () => openAsk(message, tafsirURL);
    return button;
}

function openAsk(message, tafsirURL) {
    const dialog = document.getElementById('ask-malten');
    const form = document.getElementById('ask-form');
    const question = form.elements.question;
    const submit = form.querySelector('button');
    const status = document.getElementById('ask-status');
    const turns = document.getElementById('ask-turns');
    const tafsir = document.getElementById('ask-tafsir');
    if (activeAsk) activeAsk.abort();
    const controller = new AbortController();
    activeAsk = controller;
    let previous;

    const reflection = document.getElementById('ask-reflection');
    reflection.textContent = message.Text;
    linkMaltenMentions(reflection);
    tafsir.hidden = !tafsirURL;
    if (tafsirURL) tafsir.href = tafsirURL;
    else tafsir.removeAttribute('href');
    turns.replaceChildren();
    status.textContent = '';
    form.reset();
    submit.disabled = false;
    question.oninput = function() {
        const explainLabel = previous ? 'Explain further' : 'Explain this';
        submit.textContent = question.value.trim() ? 'Ask' : explainLabel;
        question.setCustomValidity(utf8.encode(question.value.trim()).length > maxMessageBytes
            ? 'Your question exceeds 4,096 bytes.' : '');
    };
    question.oninput();

    dialog.querySelector('.close-dialog').onclick = () => dialog.close();
    dialog.onclose = function() {
        if (dialog.open) return;
        controller.abort();
        activeAsk = null;
        form.onsubmit = null;
        form.reset();
        previous = undefined;
        turns.replaceChildren();
    };
    form.onsubmit = async function(event) {
        event.preventDefault();
        if (submit.disabled) return;
        const prompt = question.value.trim();
        const questionText = prompt || submit.textContent;
        submit.disabled = true;
        status.textContent = 'Asking Malten…';
        try {
            const response = await fetch('/ask', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({stream: message.Stream, id: message.Id, question: prompt, previous}),
                signal: controller.signal
            });
            if (!response.ok) {
                const failures = {
                    404: 'This reflection is no longer in the stream.',
                    413: 'There is too much text to send together. Try a shorter question.',
                    429: 'Malten is busy. Try again shortly.',
                    503: 'Malten is unavailable on this server.'
                };
                throw new Error(failures[response.status] || 'Malten could not answer. Try again.');
            }
            const reply = await response.json();
            if (!dialog.open || controller.signal.aborted) return;
            const asked = document.createElement('p');
            asked.className = 'reader-question';
            asked.textContent = questionText;
            const answer = document.createElement('div');
            answer.className = 'agent-reply';
            answer.dir = 'auto';
            if (agentMarkdown) {
                answer.innerHTML = agentMarkdown.render(reply.text);
            } else {
                answer.style.whiteSpace = 'pre-wrap';
                answer.textContent = reply.text;
            }
            turns.append(asked, answer);
            linkMaltenMentions(asked);
            linkMaltenMentions(answer);
            previous = {question: questionText, answer: reply.text.slice(0, maxMessageBytes)};
            if (question.value.trim() === prompt) question.value = '';
            question.oninput();
            status.textContent = '';
            asked.scrollIntoView({block: 'nearest'});
        } catch (err) {
            if (!controller.signal.aborted && dialog.open) {
                status.textContent = err instanceof TypeError ? 'Could not reach Malten. Try again.' : err.message;
            }
        } finally {
            if (!controller.signal.aborted) submit.disabled = false;
        }
    };
    dialog.showModal();
}
