/* The personal timeline never uses the stream APIs. */
(async function () {
  'use strict';
  const $ = id => document.getElementById(id);
  const draftKey = 'malten-draft-v1';
  const handoffKey = 'malten-stream-draft-v1';
  let composer;
  let db, editing = null, sharing = null, renderVersion = 0, busy = false;
  const status = (message, error = false) => { $('status').textContent = message; $('status').className = error ? 'error' : ''; };
  const resize = () => { $('thought').style.height = 'auto'; $('thought').style.height = Math.max(64, $('thought').scrollHeight) + 'px'; };
  function persistDraft() {
    try {
      localStorage.setItem(draftKey, JSON.stringify({text: $('thought').value, editing}));
      $('draft-status').textContent = $('thought').value ? 'Draft saved on this device' : '';
    } catch (_) { $('draft-status').textContent = 'Draft could not be saved. Keep this page open.'; }
  }
  function resetComposer() {
    editing = null;
    composer.setImage('');
    $('thought').value = '';
    $('save').textContent = 'Save';
    $('composer-label').textContent = 'What’s on your mind?';
    $('cancel-edit').hidden = true;
    persistDraft(); resize();
  }
  function transaction(mode, work, name = 'entries') {
    return new Promise((resolve, reject) => {
      const tx = db.transaction(name, mode);
      const store = tx.objectStore(name);
      let request;
      tx.oncomplete = () => resolve(request && request.result);
      tx.onerror = () => reject(tx.error || new Error('Storage failed'));
      tx.onabort = () => reject(tx.error || new Error('Storage was interrupted'));
      try { request = work(store); } catch (error) { tx.abort(); reject(error); }
    });
  }
  function localDay(value) {
    const date = new Date(value);
    return date.getFullYear() + '-' + date.getMonth() + '-' + date.getDate();
  }
  function dayLabel(value) {
    const now = new Date(), yesterday = new Date(); yesterday.setDate(now.getDate() - 1);
    if (localDay(value) === localDay(now)) return 'Today';
    if (localDay(value) === localDay(yesterday)) return 'Yesterday';
    return new Date(value).toLocaleDateString(undefined, {day:'numeric', month:'long', year:'numeric'});
  }
  async function render() {
    const version = ++renderVersion;
    const entries = await transaction('readonly', store => store.getAll());
    if (version !== renderVersion) return;
    const query = $('search').value.toLocaleLowerCase().trim();
    const filtered = entries.filter(e => e.text.toLocaleLowerCase().includes(query)).sort((a,b) => b.created - a.created || b.id.localeCompare(a.id));
    const fragment = document.createDocumentFragment();
    let day;
    for (const entry of filtered) {
      if (localDay(entry.created) !== day) {
        day = localDay(entry.created);
        const heading = document.createElement('h2'); heading.className = 'day-heading'; heading.textContent = dayLabel(entry.created); fragment.append(heading);
      }
      const article = document.createElement('article'); article.className = 'entry';
      const text = document.createElement('p'); text.className = 'entry-text'; text.textContent = entry.text;
      const meta = document.createElement('div'); meta.className = 'entry-meta';
      const time = document.createElement('time'); time.dateTime = new Date(entry.created).toISOString(); time.textContent = new Date(entry.created).toLocaleTimeString(undefined, {hour:'numeric',minute:'2-digit'}); if (entry.updated > entry.created) time.textContent += ' · edited';
      const actions = document.createElement('div'); actions.className = 'entry-actions';
      for (const [label, action] of [['Edit', () => edit(entry)], ['Share…', () => share(entry)], ['Delete', () => remove(entry)]]) {
        const button = document.createElement('button'); button.type = 'button'; button.textContent = label;
        button.onclick = () => { if (!busy) Promise.resolve(action()).catch(() => status('That did not complete. Your thought is still here.', true)); }; actions.append(button);
      }
      if (validPhoto(entry.image)) { const img = document.createElement('img'); img.src = entry.image; img.alt = 'Photo'; img.className = 'attached-photo'; article.append(img); }
      meta.append(time, actions); article.append(text, meta); fragment.append(article);
    }
    if (!filtered.length) {
      const empty = document.createElement('p'); empty.className = 'empty'; empty.textContent = query ? 'No matches.' : 'No entries yet.'; fragment.append(empty);
    }
    $('timeline').replaceChildren(fragment);
  }
  function edit(entry) {
    if (($('thought').value || composer.image) && !confirm('Replace the current draft with this thought?')) return;
    composer.setImage(entry.image); editing = entry.id; $('thought').value = entry.text; $('save').textContent = 'Save changes'; $('composer-label').textContent = 'Edit your thought'; $('cancel-edit').hidden = false;
    persistDraft(); resize(); $('thought').focus(); $('capture').scrollIntoView({block:'center'});
  }
  async function remove(entry) {
    if (!confirm('Delete this thought from this device? This cannot be undone.')) return;
    await transaction('readwrite', store => store.delete(entry.id));
    if (editing === entry.id) resetComposer();
    status('Thought deleted from this device.'); await render();
  }
  function share(entry) {
    sharing = entry; $('share-stream').value = '_'; $('share-error').textContent = ''; $('share-dialog').showModal();
  }
  $('share-continue').onclick = () => {
    const name = $('share-stream').value.trim() || '_';
    // The existing stream transport limits messages to 1024 UTF-8 bytes.
    if (new TextEncoder().encode(sharing.text).length > 1024) {
      $('share-error').textContent = 'This thought is too long for a stream message (1,024 bytes). Copy a shorter excerpt into Public streams. Your full thought stays here.'; return;
    }
    try { sessionStorage.setItem(handoffKey, JSON.stringify({text:sharing.text, image:sharing.image, stream:name})); }
    catch (_) { $('share-error').textContent = 'Could not prepare a copy. Nothing has been posted.'; return; }
    location.href = '/#' + encodeURIComponent(name);
  };
  $('thought').oninput = () => { persistDraft(); resize(); };
  $('cancel-edit').onclick = () => { if (confirm('Discard these edits? The saved thought will stay unchanged.')) resetComposer(); };
  $('capture').onsubmit = async event => {
    event.preventDefault();
    if (busy || !db || composer.loading || (!$('thought').value.trim() && !composer.image)) return;
    const text = $('thought').value, id = editing;
    busy = true; composer.setBusy(true); $('save').disabled = true; $('thought').disabled = true;
    try {
      const previous = id ? await transaction('readonly', store => store.get(id)) : null;
      const now = Date.now();
      await transaction('readwrite', store => store.put({id:previous ? previous.id : crypto.randomUUID(), text, image:composer.image, created:previous ? previous.created : now, updated:now}));
      resetComposer(); status('Saved on this device.');
      // Ask the browser to protect storage from automatic eviction; a denial is OK.
      if (navigator.storage && navigator.storage.persist) navigator.storage.persist().catch(() => {});
      await render().catch(() => status('Saved on this device. Reopen the app to refresh the timeline.'));
    } catch (_) { status('Could not save. Your text is still in the box. Export existing thoughts if storage is full.', true); }
    finally { busy = false; composer.setBusy(false); $('save').disabled = false; $('thought').disabled = false; }
  };
  $('search').oninput = () => render().catch(() => status('Could not read your thoughts. Try reopening the app.', true));
  $('export').onclick = async () => {
    try {
      const entries = await transaction('readonly', store => store.getAll());
      const blob = new Blob([JSON.stringify({format:'malten-timeline',version:2,exported:new Date().toISOString(),entries}, null, 2)], {type:'application/json'});
      const url = URL.createObjectURL(blob), a = document.createElement('a'); a.href = url; a.download = 'malten-thoughts-' + new Date().toISOString().slice(0,10) + '.json'; a.click(); setTimeout(() => URL.revokeObjectURL(url), 60000);
      status('Export prepared. Keep the file somewhere safe.');
    } catch (_) { status('Could not export your thoughts. Please try again.', true); }
  };
  $('import').onclick = () => $('import-file').click();
  $('import-file').onchange = async () => {
    const file = $('import-file').files[0]; if (!file || busy) return;
    busy = true; $('save').disabled = true;
    try {
      if (file.size > 100 * 1024 * 1024) throw new Error('File is too large (100 MB maximum).');
      const data = JSON.parse(await file.text());
      if (data.format !== 'malten-timeline' || ![1,2].includes(data.version) || !Array.isArray(data.entries) || data.entries.length > 100000) throw new Error('Choose a Malten timeline export.');
      const entries = data.entries.map(e => {
        if (!e || typeof e.id !== 'string' || !e.id || e.id.length > 200 || typeof e.text !== 'string' || (!e.text.trim() && !e.image) || (e.image && !validPhoto(e.image)) || !Number.isSafeInteger(e.created) || e.created < 0 || e.created > 8640000000000000 || !Number.isSafeInteger(e.updated) || e.updated < e.created || e.updated > 8640000000000000) throw new Error('The export contains an invalid thought. Nothing was imported.');
        return {id:e.id,text:e.text,image:e.image || '',created:e.created,updated:e.updated};
      });
      // Keep existing IDs untouched; repeated imports cannot overwrite local edits.
      await transaction('readwrite', store => { for (const entry of entries) { const req = store.get(entry.id); req.onsuccess = () => { if (!req.result) store.put(entry); }; } });
      await render(); status('Import complete. Existing thoughts were kept unchanged.');
    } catch (error) { status('Could not import: ' + error.message, true); }
    finally { busy = false; $('save').disabled = !db; $('import-file').value = ''; }
  };
  try {
    db = await new Promise((resolve,reject) => {
      const request = indexedDB.open('malten-personal', 2);
      request.onupgradeneeded = () => { if (!request.result.objectStoreNames.contains('entries')) request.result.createObjectStore('entries', {keyPath:'id'}); request.result.createObjectStore('drafts'); };
      request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error);
      request.onblocked = () => status('Close other Malten tabs, then reopen this page to finish opening storage.', true);
    });
    composer = createComposer($('thought'), message => status(message, true), image => { transaction('readwrite', store => store.put(image, 'photo'), 'drafts').catch(() => status('Photo draft could not be saved. Keep this page open.', true)); });
    composer.setImage(await transaction('readonly', store => store.get('photo'), 'drafts'));
    db.onversionchange = () => { db.close(); location.reload(); };
    for (const id of ['thought','save','search','export','import']) $(id).disabled = false;
    try {
      const draft = JSON.parse(localStorage.getItem(draftKey));
      if (draft && typeof draft.text === 'string') {
        $('thought').value = draft.text;
        if (typeof draft.editing === 'string' && await transaction('readonly', store => store.get(draft.editing))) {
          editing = draft.editing; $('save').textContent = 'Save changes'; $('composer-label').textContent = 'Edit your thought'; $('cancel-edit').hidden = false;
        }
      }
    } catch (_) { /* Entries remain usable when browser draft storage is unavailable. */ }
    persistDraft(); resize(); await render();
  } catch (_) { status('This browser could not open local storage. Your thoughts have not been sent anywhere. Try a regular browser window with site storage enabled.', true); $('timeline').textContent = ''; }
  if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js').catch(() => {});
  window.addEventListener('pageshow', () => { if (db) render().catch(() => {}); });
}());
