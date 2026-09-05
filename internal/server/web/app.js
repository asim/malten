// Malten's mental map lives entirely in this browser. The server has no copy.
(function () {
  const GRAPH = 'malten_graph1';
  const memory = {};
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
          const text = String(item.text || item.note || '').trim();
          if (!text || seen.has(item.id)) continue;
          seen.add(item.id);
          nodes.push({
            id: item.id || window.crypto.randomUUID(),
            text,
            created_at: item.t || Date.parse(item.created_at) || Date.now(),
            location: Number.isFinite(item.lat) && Number.isFinite(item.lng)
              ? { lat: item.lat, lng: item.lng, accuracy: null } : null,
          });
        }
      } catch (_) {}
    }
    return { v: 1, nodes, edges };
  }
  function read() {
    try {
      const graph = JSON.parse(storage.getItem(GRAPH));
      if (graph && graph.v === 1 && Array.isArray(graph.nodes) && Array.isArray(graph.edges)) return graph;
    } catch (_) {}
    const graph = legacy();
    if (graph.nodes.length) storage.setItem(GRAPH, JSON.stringify(graph));
    return graph;
  }
  window.Malten = {
    getGraph: read,
    setGraph: graph => storage.setItem(GRAPH, JSON.stringify(graph)),
    newId: () => {
      const bytes = new Uint8Array(6); crypto.getRandomValues(bytes);
      return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
    },
  };
})();
