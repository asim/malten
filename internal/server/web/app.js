// Malten client-side storage. The server keeps nothing: your finds and your
// Ordnance Survey API key live here, in this browser. Map tiles are requested
// straight from the OS APIs with your key — the server never sees it.
(function () {
  const FINDS = 'malten_finds';       // [{id, lat, lng, note, grid_ref, created_at}]
  const KEY = 'malten_oskey';         // your OS Data Hub project API key
  const TIMELINE = 'malten_timeline2'; // the trail so far, so a reload isn't a fresh start
  const TIMELINE_TTL = 36 * 3600e3;    // a day and a half; older than that, start over

  function read(k, f) { try { return JSON.parse(localStorage.getItem(k)) || f; } catch (_) { return f; } }
  function write(k, v) { localStorage.setItem(k, JSON.stringify(v)); }

  window.Malten = {
    getFinds: () => read(FINDS, []),
    setFinds: (f) => write(FINDS, f),
    addFind: (f) => { const a = read(FINDS, []); a.push(f); write(FINDS, a); return f; },
    removeFind: (id) => { write(FINDS, read(FINDS, []).filter(f => f.id !== id)); },
    // The timeline is client-side too — the server holds no history, so this
    // browser is the only memory there is. It's an append-only log of events
    // (place fixes, nearby things, nudges, the conversation) plus the last fix,
    // so a reload continues the same trail instead of starting a new one.
    getTimeline: () => {
      const t = read(TIMELINE, null);
      if (!t || !Array.isArray(t.events) || !(Date.now() - (t.saved_at || 0) < TIMELINE_TTL)) return { events: [], fix: null };
      return { events: t.events, fix: t.fix || null };
    },
    setTimeline: (events, fix) => {
      try { write(TIMELINE, { saved_at: Date.now(), events: events, fix: fix || null }); }
      catch (_) { try { localStorage.removeItem(TIMELINE); } catch (_) {} } // quota: drop it rather than break
    },
    clearTimeline: () => localStorage.removeItem(TIMELINE),
    getKey: () => localStorage.getItem(KEY) || '',
    setKey: (k) => k ? localStorage.setItem(KEY, k) : localStorage.removeItem(KEY),
    newId: () => { const a = new Uint8Array(6); crypto.getRandomValues(a); return 'F-' + Array.from(a, b => b.toString(16).padStart(2, '0')).join(''); },
  };
})();
