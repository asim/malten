// Malten client-side storage. The server keeps nothing: your finds and your
// Ordnance Survey API key live here, in this browser. Map tiles are requested
// straight from the OS APIs with your key — the server never sees it.
(function () {
  const FINDS = 'malten_finds'; // [{id, lat, lng, note, grid_ref, created_at}]
  const KEY = 'malten_oskey';   // your OS Data Hub project API key

  function read(k, f) { try { return JSON.parse(localStorage.getItem(k)) || f; } catch (_) { return f; } }
  function write(k, v) { localStorage.setItem(k, JSON.stringify(v)); }

  window.Malten = {
    getFinds: () => read(FINDS, []),
    setFinds: (f) => write(FINDS, f),
    addFind: (f) => { const a = read(FINDS, []); a.push(f); write(FINDS, a); return f; },
    removeFind: (id) => { write(FINDS, read(FINDS, []).filter(f => f.id !== id)); },
    getKey: () => localStorage.getItem(KEY) || '',
    setKey: (k) => k ? localStorage.setItem(KEY, k) : localStorage.removeItem(KEY),
    newId: () => { const a = new Uint8Array(6); crypto.getRandomValues(a); return 'F-' + Array.from(a, b => b.toString(16).padStart(2, '0')).join(''); },
  };
})();
