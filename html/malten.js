var last = timeAgo();
var typing = false;
var maxChars = 500;
var maxThoughts = 1000;
var seen = {};
var streams = {};

String.prototype.parseURL = function(embed) {
	return this.replace(/[A-Za-z]+:\/\/[A-Za-z0-9-_]+\.[A-Za-z0-9-_:%&~\?\/.=]+/g, function(url) {
		if (embed == true) {
			var match = url.match(/^.*(youtu.be\/|v\/|u\/\w\/|embed\/|watch\?v=|\&v=)([^#\&\?]*).*/);
			if (match && match[2].length == 11) {
				return '<div class="iframe">'+
				'<iframe src="//www.youtube.com/embed/' + match[2] +
				'" frameborder="0" allowfullscreen></iframe>' + '</div>';
			};
			if (url.match(/^.*giphy.com\/media\/[a-zA-Z0-9]+\/[a-zA-Z0-9]+.gif$/)) {
				return '<div class="animation"><img src="'+url+'"></div>';
			}
		};
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

function timeAgo() {
	var ts = new Date().getTime() / 1000;
	return (ts - 86400) * 1e9;
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
};

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
	document.getElementById('thoughts').innerHTML = "";
	last = timeAgo();
	seen = {};
};

function clipThoughts() {
	var list = document.getElementById('thoughts');
	while (list.length > maxThoughts) {
		list.removeChild(list.lastChild);
	}
};

function command(q) {
	var parts = q.split(" ");

	if (parts.length > 2 && parts[1] == "animate") {
		loadGif(parts.slice(2).join(" "));
	} else {
		postThought();
	}

	return false;
}

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

		var embed = true;

		if (array[i].Glimmer != null && array[i].Glimmer.Type != "player") {
			embed = false;
		}

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
		d2.innerHTML = html.parseURL(embed).parseHashTag();
                item.appendChild(d1);
                item.appendChild(d2);

		if (array[i].Glimmer != null && array[i].Glimmer.Type != "player") {
			var d3 = document.createElement('div');
			var img = document.createElement('img');
			var a = document.createElement('a');
			d3.className = 'image';
			img.src = array[i].Glimmer.Image;
			a.href = array[i].Glimmer.Url;
			a.appendChild(img);
			d3.appendChild(a);
			item.appendChild(d3);
		};

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

function loadGif(q) {
	var xhr = $.get("http://api.giphy.com/v1/gifs/search?q="+q+"&api_key=dc6zaTOxFJmzC");
	xhr.done(function(data) {
		if (data.data.length == 0) {
			return false;
		}
		var i = Math.floor(Math.random() * data.data.length)
		form.elements["text"].value = data.data[i].images.original.url;
		submitThought();
	});
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
	    		updateTimestamps();
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
	}, 5000);
};

function pollTimestamps() {
	updateTimestamps();

	setTimeout(function() {
	    pollTimestamps();
	}, 60000);
};


function postThought() {
        $.post("/thoughts", $("#form").serialize());
        form.elements["text"].value = '';
        loadThoughts();
	return false;
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

function submitThought() {
	if (form.elements["text"].value.length <= 0) {
		return false;
	}

	if (form.elements["text"].value.match(/^\s+$/)) {
		return false;
	}

	if (form.elements["text"].value.match(/^\/malten\s/)) {
		command(form.elements["text"].value);
		return false;
	}
 
	return postThought();
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
