// Malten's mental map lives entirely in this browser. The server has no copy.
(function () {
  const GRAPH = 'malten_graph1';
  const memory = {};
  let fallbackStorage = null, volatileGraph = null;
  function store() {
    for (const get of [() => localStorage, () => sessionStorage]) {
      try {
        const s = get(); s.setItem('__malten_probe', '1'); s.removeItem('__malten_probe'); return s;
      } catch (_) {}
    }
    return { getItem: k => memory[k] || null, setItem: (k, v) => { memory[k] = String(v); } };
  }
  const storage = store();
  function legacy() {
    const nodes = [], edges = [], seen = new Set();
    for (const key of ['malten_notes', 'malten_finds']) {
      try {
        const items = JSON.parse(storage.getItem(key)) || [];
        for (const item of items) {
          const text = String(item.text || item.note || '').trim()
            || (key === 'malten_finds' ? 'A place I recorded' : '');
          if (!text || seen.has(item.id)) continue;
          seen.add(item.id);
          nodes.push({
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
    return { v: 1, nodes, edges };
  }
  function read() {
    if (volatileGraph) return volatileGraph;
    if (fallbackStorage) {
      try {
        const graph = JSON.parse(fallbackStorage.getItem(GRAPH));
        if (graph && graph.v === 1 && Array.isArray(graph.nodes) && Array.isArray(graph.edges)) return graph;
      } catch (_) {}
    }
    try {
      const graph = JSON.parse(storage.getItem(GRAPH));
      if (graph && graph.v === 1 && Array.isArray(graph.nodes) && Array.isArray(graph.edges)) return graph;
    } catch (_) {}
    const graph = legacy();
    if (graph.nodes.length) write(graph);
    return graph;
  }
  function write(graph) {
    const value = JSON.stringify(graph);
    try {
      storage.setItem(GRAPH, value);
      fallbackStorage = null; volatileGraph = null;
      return true;
    } catch (_) {}
    try {
      sessionStorage.setItem(GRAPH, value);
      fallbackStorage = sessionStorage; volatileGraph = null;
      return true;
    } catch (_) {}
    volatileGraph = graph;
    return false;
  }
  window.Malten = {
    getGraph: read,
    setGraph: write,
    newId: () => {
      const bytes = new Uint8Array(6); crypto.getRandomValues(bytes);
      return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
    },
  };
})();
