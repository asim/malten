var messageUrl = "/messages";
var streamUrl = "/streams";
var limit = 25;
var last = timeAgo();
var typing = false;
var maxChars = 1024;
var maxMessages = 1000;
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

function command(q) {
	var parts = q.split(" ");

	if (parts.length > 2 && parts[1] == "animate") {
		loadGif(parts.slice(2).join(" "));
	} else {
		postMessage();
	}

	return false;
}

function escapeHTML(str) {
	var div = document.createElement('div');
	div.style.display = 'none';
	div.appendChild(document.createTextNode(str));
	return div.innerHTML;
};

function displayMessages(array, direction) {
	var list = document.getElementById('messages');

        for(i = 0; i < array.length; i++) {
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

		if (direction >= 0) {
	                list.insertBefore(item, list.firstChild);
		} else {
			list.appendChild(item);
		}
		seen[array[i].Id] = array[i];
        }

	if (direction >= 0) {
		last = array[array.length -1].Created;
	}
};

function getStreams() {
	$.get(streamUrl, function(data) {
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
		clearMessages();
	};
	return false;
};

function newStream() {
	var form = document.getElementById('new-form');
 	var priv = document.getElementById("private").checked;
	var stream = form.elements["stream"].value;
	var ttl = form.elements["ttl"].value

        $.post(streamUrl, {stream: stream, private: priv, ttl: ttl })
          .done(function(data) {
             window.location = location.protocol + '//' + location.host + '/#' + data.stream;
	     return false;
           })
          .fail(function() {
             alert( "error creating stream" );
	     return false;
          })
	return false;
}

function loadGif(q) {
	var xhr = $.get("http://api.giphy.com/v1/gifs/search?q="+q+"&api_key=dc6zaTOxFJmzC");
	xhr.done(function(data) {
		if (data.data.length == 0) {
			return false;
		}
		var i = Math.floor(Math.random() * data.data.length)
		form.elements["text"].value = data.data[i].images.original.url;
		submitMessage();
	});
};

function loadListeners() {
	if (window.navigator.standalone) {
		$.ajaxSetup({isLocal:true});
	};


	$(window).scroll(function() {
		if($(window).scrollTop() == $(document).height() - $(window).height()) {
			loadMore();
		}
	});

	document.getElementById("text").addEventListener("keyup", function() {
		start();
		chars();
	}, false);

	document.getElementById("text").addEventListener("keydown", function() {
		stop();
	}, false);
};

function loadMore() {
	var divs = document.getElementsByClassName('time');
	var oldest = new Date().getTime() * 1e6;

	if (divs.length > 0) {
		oldest = divs[divs.length-1].getAttribute('data-time');
	}

	var params = "?direction=-1&limit=" + limit + "&last=" + oldest;

	if (window.location.hash.length > 0) {
		params += "&stream="+ window.location.hash.replace('#', '');
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
	var stream = window.location.hash.replace('#', '');

	// stream provided?
	if (window.location.hash.length > 0) {
		params += "&stream="+ stream;
		form.elements["stream"].value = stream;
	} else {
		form.elements["stream"].value = '';
	};

	$.get(messageUrl + params, function(data) {
		if (data != undefined && data.length > 0) {
			displayMessages(data, 1);
			clipMessages();
	    		updateTimestamps();
		}
	})
	.fail(function(err) {
		console.log(err);
	})
	.done();

        return false;
};

function pollMessages() {
	if (typing == false) {
		loadMessages();
	};

	setTimeout(function() {
	    pollMessages();
	}, 100);
};

function pollTimestamps() {
	updateTimestamps();

	setTimeout(function() {
	    pollTimestamps();
	}, 10000);
};


function postMessage() {
        $.post(messageUrl, $("#form").serialize());
        form.elements["text"].value = '';
        loadMessages();
	return false;
};

function setCurrent(text) {
	var current = document.getElementById('current');
	current.href = window.location.href;

	if (window.location.hash.length > 0) {
		current.text = window.location.hash;
	} else {
		current.text = "malten";
	}
};

function showMessages() {
	getStreams();
	setCurrent();
	clearMessages();
	loadMessages();
}

function start() {
	typing = false;
};

function stop() {
	typing = true;
};

function submitMessage() {
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

function updateTimestamps() {
	var divs = document.getElementsByClassName('time');
	for (i = 0; i < divs.length; i++) {
		var time = divs[i].getAttribute('data-time');
		divs[i].innerHTML = parseDate(time);
	};
};
