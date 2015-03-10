var last = "0";
var typing = false;
var maxChars = 500;
var maxThoughts = 1000;
var seen = {};
var streams = {};

String.prototype.parseURL = function() {
	return this.replace(/[A-Za-z]+:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:%&~\?\/.=]+/g, function(url) {
		var pretty = url.replace(/^http(s)?:\/\/(www\.)?/, '');
		return pretty.link(url);
	});
};
String.prototype.parseUsername = function() {
	return this.replace(/[@]+[A-Za-z0-9-_]+/g, function(u) {
		var username = u.replace("@","");
		return u.link("http://twitter.com/"+username);
	});
};
String.prototype.parseHashTag = function() {
	return this.replace(/[#]+[A-Za-z0-9-_]+/g, function(t) {
		//var tag = t.replace("#","%23")
		var url = location.protocol+'//'+location.hostname+(location.port ? ':'+location.port: '');
		return t.link(url + '/' + t);
	});
};

function parseDate(tdate) {
    var system_date = new Date(tdate/1e6);
    var user_date = new Date();
    if (K.ie) {
        system_date = Date.parse(tdate.replace(/( \+)/, ' UTC$1'))
    }
    var diff = Math.floor((user_date - system_date) / 1000);
    if (diff < 0) {return "0s";}
    if (diff < 60) {return diff + "s";}
    if (diff <= 90) {return "1m";}
    if (diff <= 3540) {return Math.round(diff / 60) + "m";}
    if (diff <= 5400) {return "1h";}
    if (diff <= 86400) {return Math.round(diff / 3600) + "h";}
    if (diff <= 129600) {return "1d";}
    if (diff < 604800) {return Math.round(diff / 86400) + "d";}
    if (diff <= 777600) {return "1w";}
    return "on " + system_date;
}

// from http://widgets.twimg.com/j/1/widget.js
var K = function () {
    var a = navigator.userAgent;
    return {
        ie: a.match(/MSIE\s([^;]*)/)
    }
}();

function chars() {
	var i = document.getElementById('text').value.length;
	var c = maxChars;

	if (i > maxChars) {
		c = i - maxChars;
	} else {
		c = maxChars - i;
	}

	document.getElementById('chars').innerHTML = c;
};

function clearThoughts() {
	var list = document.getElementById('thoughts');
	while (list.lastChild) {
		list.removeChild(list.lastChild);
	}
	last = "0";
	seen = {};
};

function clipThoughts() {
	var list = document.getElementById('thoughts');
	while (list.length > maxThoughts) {
		list.removeChild(list.lastChild);
	}
};

function escapeHTML(str) {
	var div = document.createElement('div');
	div.style.display = 'none';
	div.appendChild(document.createTextNode(str));
	return div.innerHTML;
};

function displayThoughts(array) {
	var list = document.getElementById('thoughts');

        for(i = 0; i < array.length; i++) {
		if (array[i].Id in seen) {
			continue;
		};

		// tagging
		array[i].Text = tagText(array[i].Text);

                var item = document.createElement('li');
                var d1 = document.createElement('div');
                var d2 = document.createElement('div');
		var html = escapeHTML(array[i].Text);
		d1.className = 'time';
		d2.className = 'thought';
		d1.innerHTML = parseDate(array[i].Created);
		d1.setAttribute('data-time', array[i].Created);
		d2.innerHTML = html.parseURL().parseHashTag();
                item.appendChild(d1);
                item.appendChild(d2);
                list.insertBefore(item, list.firstChild);
		seen[array[i].Id] = array[i];
        }

	last = array[array.length -1].Created;
};

function getStreams() {
	$.get('/streams', function(data) {
		streams = data;
	})
	.fail(function(err) {
		console.log(err);
	})
	.done();
}

function gotoStream(t) {
	var stream = document.getElementById('goto').elements['gstream'].value.replace(/^#+/, '');
	if (stream.length > 0) {
		document.getElementById('goto').elements['gstream'].value = '';
		window.location = location.protocol+'//'+location.hostname+(location.port ? ':'+location.port: '') + '/#' + stream;
		clearThoughts();
	};
	return false;
};

function loadThoughts() {
	var params = "?last=" + last;
	var text = 'malten';
	var form = document.getElementById('form');
	var stream = window.location.hash.replace('#', '');
	var list = document.getElementById('thoughts');

	// stream provided?
	if (window.location.hash.length > 0) {
		params += "&stream="+ stream;
		form.elements["stream"].value = stream;
		text = window.location.hash;
	} else {
		form.elements["stream"].value = '';
	};

	setCurrent(text)

	$.get('/thoughts' + params, function(data) {
		if (data != undefined && data.length > 0) {
			displayThoughts(data);
			clipThoughts();
		}
	})
	.fail(function(err) {
		console.log(err);
	})
	.done();

        return false;
};

function pollThoughts() {
	if (typing == false) {
		loadThoughts();
	};

	setTimeout(function() {
	    pollThoughts();
	    updateTimestamps();
	}, 5000);
};

function setCurrent(text) {
	var current = document.getElementById('current');
	current.text = text;
	current.href = window.location.href;
};

function showThoughts() {
	getStreams();
	clearThoughts();
	loadThoughts();
}

function start() {
	typing = false;
};

function stop() {
	typing = true;
};

function submitThought(t) {
        if (form.elements["text"].value.length > 0 && !form.elements["text"].value.match(/^\s+$/)) {
                $.post(t.action, $("#form").serialize());
                form.elements["text"].value = '';
                loadThoughts();
        }
	return false;
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

function updateTimestamps() {
	var divs = document.getElementsByClassName('time');
	for (i = 0; i < divs.length; i++) {
		var time = divs[i].getAttribute('data-time');
		divs[i].innerHTML = parseDate(time);
	};
};
