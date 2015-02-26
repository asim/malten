function makeUL(array) {
        var list = document.createElement('ul');

        array = array.reverse();

        for(i = 0; i < array.length; i++) {
                var item = document.createElement('li');
                var div = document.createElement('div');
		var text = document.createTextNode(array[i].Text);
		div.appendChild(text);
                item.appendChild(div);
                list.appendChild(item);
        }

        return list;
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
