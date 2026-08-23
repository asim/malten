// Malten client-side storage. The server keeps nothing: your finds and your
// Ordnance Survey API key live here, in this browser. Map tiles are requested
// straight from the OS APIs with your key — the server never sees it.
(function () {
  const FINDS = 'malten_finds';        // [{id, lat, lng, note, grid_ref, created_at}]
  const KEY = 'malten_oskey';          // your OS Data Hub project API key
  const TIMELINE = 'malten_timeline2'; // the trail so far, so a reload isn't a fresh start
  const TIMELINE_TTL = 7 * 24 * 3600e3; // a week; older than that, start a fresh trail
  const TIMELINE_V = 1;                // schema version of the stored log
  const SQUARES = 'malten_squares';    // OS grid squares you've been in: {code: {t, place}}

  // Storage that degrades instead of throwing. localStorage is unavailable or
  // throws in some private-browsing modes, so fall back to sessionStorage (the
  // trail then lasts the tab rather than the week) and to memory as a last
  // resort, so the app never breaks over a store it can't write.
  const memory = {};
  function store() {
    for (const s of [() => localStorage, () => sessionStorage]) {
      try {
        const st = s();
        const probe = '__malten_probe';
        st.setItem(probe, '1'); st.removeItem(probe);
        return st;
      } catch (_) { /* try the next one */ }
    }
    return {
      getItem: (k) => (k in memory ? memory[k] : null),
      setItem: (k, v) => { memory[k] = String(v); },
      removeItem: (k) => { delete memory[k]; },
    };
  }
  const S = store();

  function read(k, f) { try { return JSON.parse(S.getItem(k)) || f; } catch (_) { return f; } }
  function write(k, v) { S.setItem(k, JSON.stringify(v)); }

  window.Malten = {
    getFinds: () => read(FINDS, []),
    setFinds: (f) => write(FINDS, f),
    addFind: (f) => { const a = read(FINDS, []); a.push(f); write(FINDS, a); return f; },
    removeFind: (id) => { write(FINDS, read(FINDS, []).filter(f => f.id !== id)); },
    // The timeline is client-side too — the server holds no history, so this
    // browser is the only memory there is. It's an append-only log of typed
    // events, not rendered markup, so it can be read back for more than
    // redrawing: it's replayed to the agent as context (where you've been, what
    // was around, what was said). Keep it structural for that reason.
    //
    //   { v, saved_at, fix: {t, lat, lng}, events: [
    //       {id, t, k:'when',  place, lat, lng}          arrived somewhere
    //       {id, t, k:'entry', icon, title, sub}         live transport line
    //       {id, t, k:'place', poi:{name,kind,lat,lng}, dist}
    //       {id, t, k:'idea',  text}                     a nudge from the agent
    //       {id, t, k:'q',     text}                     you asked
    //       {id, t, k:'a',     text}                     the agent answered
    //       {id, t, k:'city'}                            out of coverage
    //   ]}
    getTimeline: () => {
      const t = read(TIMELINE, null);
      if (!t || t.v !== TIMELINE_V || !Array.isArray(t.events)) return { events: [], fix: null };
      if (!(Date.now() - (t.saved_at || 0) < TIMELINE_TTL)) return { events: [], fix: null };
      return { events: t.events, fix: t.fix || null };
    },
    // Saving must not be able to lose the trail: if the store is full, keep the
    // most recent slice rather than dropping everything.
    setTimeline: (events, fix) => {
      for (const keep of [events.length, 80, 30, 10]) {
        try {
          write(TIMELINE, { v: TIMELINE_V, saved_at: Date.now(), fix: fix || null, events: events.slice(-keep) });
          return true;
        } catch (_) { /* quota — try a shorter tail */ }
      }
      return false;
    },
    clearTimeline: () => S.removeItem(TIMELINE),

    // New ground: the OS National Grid squares you've actually been in, kept
    // forever (unlike the timeline, which ages out) because it's the map of
    // where you've been. { "TQ 15 68": {t, place} } — no score, no streak, just
    // the record. It never leaves the browser except as context for a nudge.
    getSquares: () => read(SQUARES, {}),
    markSquare: (code, place) => {
      const all = read(SQUARES, {});
      if (all[code]) return false; // been here before
      all[code] = { t: Date.now(), place: place || '' };
      try { write(SQUARES, all); } catch (_) { return true; }
      return true;
    },
    getKey: () => S.getItem(KEY) || '',
    setKey: (k) => k ? S.setItem(KEY, k) : S.removeItem(KEY),
    newId: () => { const a = new Uint8Array(6); crypto.getRandomValues(a); return 'F-' + Array.from(a, b => b.toString(16).padStart(2, '0')).join(''); },
  };
})();
