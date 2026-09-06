// Preserve older private captures on this device.
(function () {
  const NETWORK = 'malten_network1';
  const PREVIOUS = 'malten_graph1';
  const memory = {};
  let fallbackStorage = null, volatileNetwork = null;

  function store() {
    for (const get of [() => localStorage, () => sessionStorage]) {
      try {
        const s = get(); s.setItem('__malten_probe', '1'); s.removeItem('__malten_probe'); return s;
      } catch (_) {}
    }
    return { getItem: k => memory[k] || null, setItem: (k, v) => { memory[k] = String(v); } };
  }
  const storage = store();

  function valid(network) {
    return network && network.v === 1
      && Array.isArray(network.points) && Array.isArray(network.connections);
  }

  function fromPrevious() {
    try {
      const previous = JSON.parse(storage.getItem(PREVIOUS));
      if (previous && Array.isArray(previous.nodes) && Array.isArray(previous.edges)) {
        return { v: 1, points: previous.nodes, connections: previous.edges };
      }
    } catch (_) {}
    return null;
  }

  function fromLegacy() {
    const points = [], connections = [], seen = new Set();
    for (const key of ['malten_notes', 'malten_finds']) {
      try {
        const items = JSON.parse(storage.getItem(key)) || [];
        for (const item of items) {
          const text = String(item.text || item.note || '').trim()
            || (key === 'malten_finds' ? 'A place I recorded' : '');
          if (!text || seen.has(item.id)) continue;
          seen.add(item.id);
          points.push({
            id: item.id || window.crypto.randomUUID(),
            text,
            created_at: item.t || Date.parse(item.created_at) || Date.now(),
            location: Number.isFinite(item.lat) && Number.isFinite(item.lng)
              ? { lat: item.lat, lng: item.lng, accuracy: null } : null,
            legacy: key === 'malten_finds'
              ? { find: true, photo: !!item.photo, grid_ref: item.grid_ref || null }
              : null,
          });
        }
      } catch (_) {}
    }
    return { v: 1, points, connections };
  }

  function readFrom(source) {
    try {
      const network = JSON.parse(source.getItem(NETWORK));
      return valid(network) ? network : null;
    } catch (_) { return null; }
  }

  function read() {
    if (volatileNetwork) return volatileNetwork;
    if (fallbackStorage) {
      const network = readFrom(fallbackStorage);
      if (network) return network;
    }
    const stored = readFrom(storage);
    if (stored) return stored;
    const network = fromPrevious() || fromLegacy();
    if (network.points.length) write(network);
    return network;
  }

  function write(network) {
    const value = JSON.stringify(network);
    try {
      storage.setItem(NETWORK, value);
      fallbackStorage = null; volatileNetwork = null;
      return true;
    } catch (_) {}
    try {
      sessionStorage.setItem(NETWORK, value);
      fallbackStorage = sessionStorage; volatileNetwork = null;
      return true;
    } catch (_) {}
    volatileNetwork = network;
    return false;
  }

  // One record per capture: photos do not compete with localStorage's small quota.
  let outboxDB;
  function outboxStore(mode, action) {
    if (!outboxDB) outboxDB = new Promise((resolve, reject) => {
      const request = indexedDB.open('malten_outbox', 1);
      request.onupgradeneeded = () => request.result.createObjectStore('posts', {keyPath: 'id'});
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => { outboxDB = null; reject(request.error); };
    });
    return outboxDB.then(db => new Promise((resolve, reject) => {
      const transaction = db.transaction('posts', mode);
      const request = action(transaction.objectStore('posts'));
      transaction.oncomplete = () => resolve(request.result);
      transaction.onabort = () => reject(transaction.error || new Error('Storage unavailable'));
      transaction.onerror = () => reject(transaction.error);
    }));
  }

  window.Malten = {
    outbox: {
      list: () => outboxStore('readonly', store => store.getAll()),
      put: post => outboxStore('readwrite', store => store.put(post)),
      remove: id => outboxStore('readwrite', store => store.delete(id)),
    },
    getNetwork: read,
    setNetwork: write,
    newId: () => {
      const bytes = new Uint8Array(6); crypto.getRandomValues(bytes);
      return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
    },
  };
})();
