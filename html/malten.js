var typing = false;
var maxChars = 500;

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

function escapeHTML(str) {
	var div = document.createElement('div');
	div.style.display = 'none';
	div.appendChild(document.createTextNode(str));
	return div.innerHTML;
};

function gotoStream(t) {
	var stream = document.getElementById("goto").elements['gstream'].value.replace(/^#+/, '');
	if (stream.length > 0) {
		document.getElementById("goto").elements['gstream'].value = '';
		window.location = location.protocol+'//'+location.hostname+(location.port ? ':'+location.port: '') + '/#' + stream;
	};
	return false;
};

function makeUL(array) {
        var list = document.createElement('ul');

        array = array.reverse();

        for(i = 0; i < array.length; i++) {
                var item = document.createElement('li');
                var div = document.createElement('div');
		var html = escapeHTML(array[i].Text);
		div.innerHTML = html.parseURL().parseHashTag();
                item.appendChild(div);
                list.appendChild(item);
        }

        return list;
};

function pollThoughts() {
	if (typing == false) {
		thoughts();
	};

	setTimeout(function() {
	    pollThoughts();
	}, 5000);
};

function start() {
	typing = false;
};

function stop() {
	typing = true;
};

function submitThought(t) {
        if (form.elements["text"].value.length > 0) {
                $.post(t.action, $("#form").serialize());
                form.elements["text"].value = '';
                thoughts();
        }
	return false;
};

function thoughts() {
	var params = "";
	var text = 'malten';
	var form = document.getElementById('form');
	var stream = window.location.hash.replace('#', '');

	if (window.location.hash.length > 0) {
		params = "?stream="+ stream;
		form.elements["stream"].value = stream;
		text = window.location.hash;
		document.getElementById('desc').style.display = 'none';
	} else {
		document.getElementById('desc').style.display = 'block';
		form.elements["stream"].value = '';
	};

	var current = document.getElementById('current');
	current.text = text;
	current.href = window.location.href;

	$.get('/thoughts' + params, function(data) {
                var list = document.getElementById('thoughts');
                while (list.lastChild) {
                        list.removeChild(list.lastChild);
                }
                list.appendChild(makeUL(data));     
                list.style.display = 'block';
	})
	.fail(function(err) {
		console.log(err);
	})
	.done();

        return false;
};
