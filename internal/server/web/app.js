// Malten client-side storage. The server keeps nothing about you; your
// conversations and issues live here, in this browser, and travel up with each
// request as context. Clearing your browser data clears everything.
(function () {
  const CONVOS = 'malten_convos';   // [{id, title, updatedAt, messages:[{role:'user'|'agent', text}]}]
  const ISSUES = 'malten_issues';   // [{id, title, plan, status, created_at}]
  const CURRENT = 'malten_current'; // id of the open conversation

  function read(key, fallback) {
    try { return JSON.parse(localStorage.getItem(key)) || fallback; } catch (_) { return fallback; }
  }
  function write(key, value) { localStorage.setItem(key, JSON.stringify(value)); }

  function randId(prefix) {
    const a = new Uint8Array(8); crypto.getRandomValues(a);
    return prefix + Array.from(a, b => b.toString(16).padStart(2, '0')).join('');
  }

  const Malten = {
    getConvos: () => read(CONVOS, []),
    setConvos: (c) => write(CONVOS, c),
    getIssues: () => read(ISSUES, []),
    setIssues: (i) => write(ISSUES, i),
    openIssues: () => read(ISSUES, []).filter(i => i.status !== 'closed'),
    getCurrent: () => localStorage.getItem(CURRENT) || '',
    setCurrent: (id) => id ? localStorage.setItem(CURRENT, id) : localStorage.removeItem(CURRENT),
    newConvoId: () => randId('C-'),

    // Merge the agent's issue changes from a turn into local storage.
    mergeIssueChanges(changes) {
      if (!changes || !changes.length) return;
      const issues = read(ISSUES, []);
      const byId = new Map(issues.map(i => [i.id, i]));
      for (const c of changes) byId.set(c.id, Object.assign(byId.get(c.id) || {}, c));
      write(ISSUES, Array.from(byId.values()));
    },

    // Wipe everything stored locally.
    clearAll() { localStorage.removeItem(CONVOS); localStorage.removeItem(ISSUES); localStorage.removeItem(CURRENT); },
  };

  window.Malten = Malten;
})();
