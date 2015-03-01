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

function escapeHTML(str) {
    var div = document.createElement('div');
    div.style.display = 'none';
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
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
	setTimeout(function() {
	    thoughts();
	}, 0);

	setTimeout(function() {
	    pollThoughts();
	}, 5000);
};

function submitThought(t) {
	$.post(t.action, $("#form").serialize());
	form.elements["text"].value = '';
	thoughts();
	return false;
};

function thoughts() {
	var params = "";
	var text = '#_';

	if (window.location.hash.length > 0) {
		var form = document.getElementById('form');
		var stream = window.location.hash.replace('#', '');
		params = "?stream="+ stream;
		form.elements["stream"].value = stream;
		text = window.location.hash;
	};

	var current = document.getElementById('current');
	current.text = text;
	current.href = window.location.href;

        var xmlHttp = null;
        xmlHttp = new XMLHttpRequest();
        xmlHttp.open("GET", '/thoughts' + params, false);
        xmlHttp.send(null);

        if (xmlHttp.status == 200) {
                var things = JSON.parse(xmlHttp.responseText);
                if (things == null) {
                        return false;
                }

                var list = document.getElementById('thoughts');
                while (list.lastChild) {
                        list.removeChild(list.lastChild);
                }
                list.appendChild(makeUL(things));     
                list.style.display = 'block';
        }

        return false;
};
