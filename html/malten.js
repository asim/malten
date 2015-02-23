function makeUL(array) {
        var list = document.createElement('ul');

        array = array.reverse();

        for(i = 0; i < array.length; i++) {
                var item = document.createElement('li');
                var div = document.createElement('div');
		var text = document.createTextNode(array[i]);
		div.appendChild(text);
                item.appendChild(div);
                list.appendChild(item);
        }

        return list;
};

function thoughts() {
        var xmlHttp = null;
        xmlHttp = new XMLHttpRequest();
        xmlHttp.open("GET", '/thoughts', false);
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
