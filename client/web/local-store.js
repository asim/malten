/* One local database; original entries and drafts are migrated in-place. */
class LocalStreams {
  static async open() {
    const db = await new Promise((resolve, reject) => {
      const req = indexedDB.open('malten-personal', 3);
      req.onupgradeneeded = () => {
        const db = req.result, tx = req.transaction;
        if (!db.objectStoreNames.contains('entries')) db.createObjectStore('entries', {keyPath:'id'});
        if (!db.objectStoreNames.contains('drafts')) db.createObjectStore('drafts');
        if (!db.objectStoreNames.contains('streams')) db.createObjectStore('streams', {keyPath:'id'});
        const entries = tx.objectStore('entries');
        if (!entries.indexNames.contains('stream')) entries.createIndex('stream', 'stream');
        entries.openCursor().onsuccess = event => {
          const cursor = event.target.result; if (!cursor) return;
          const entry = cursor.value;
          if (!entry.stream) { entry.stream = '_'; cursor.update(entry); }
          cursor.continue();
        };
        tx.objectStore('streams').put({id:'_', created:Date.now()});
        let old;
        try { old = JSON.parse(localStorage.getItem('malten-draft-v1')); } catch (_) {}
        const drafts = tx.objectStore('drafts'), photo = drafts.get('photo');
        photo.onsuccess = () => drafts.put({text:old?.text || '', editing:old?.editing || null, image:photo.result || ''}, '_');
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
      req.onblocked = () => reject(Error('Close older Malten tabs, then reload to open local streams.'));
    });
    db.onversionchange = () => { db.close(); location.reload(); };
    return new LocalStreams(db);
  }
  constructor(db) { this.db = db; }
  tx(name, mode, work) {
    return new Promise((resolve,reject) => {
      const tx=this.db.transaction(name,mode); let req;
      tx.oncomplete=()=>resolve(req?.result);
      tx.onerror=()=>reject(tx.error); tx.onabort=()=>reject(tx.error || Error('Storage interrupted'));
      try { req=work(tx.objectStore(name)); } catch(e) { tx.abort(); reject(e); }
    });
  }
  streams() { return this.tx('streams','readonly',s=>s.getAll()); }
  ensure(id) { return this.tx('streams','readwrite',s=> { const r=s.get(id);r.onsuccess=()=>{if(!r.result)s.put({id,created:Date.now()});}; }); }
  entries(stream) { return this.tx('entries','readonly',s=>s.index('stream').getAll(stream)); }
  put(entry) { return this.tx('entries','readwrite',s=>s.put(entry)); }
  remove(id) { return this.tx('entries','readwrite',s=>s.delete(id)); }
  draft(stream, value) { return value === undefined ? this.tx('drafts','readonly',s=>s.get(stream)) : this.tx('drafts','readwrite',s=>s.put(value,stream)); }
  async export() { return {format:'malten-timeline',version:3,streams:await this.streams(),entries:await this.tx('entries','readonly',s=>s.getAll())}; }
  async import(data) {
    if(data.format!=='malten-timeline' || ![1,2,3].includes(data.version) || !Array.isArray(data.entries) || data.entries.length>100000) throw Error('Choose a Malten export.');
    const entries=data.entries.map(e=> {
      if(!e || typeof e.id!=='string' || !e.id || e.id.length>200 || typeof e.text!=='string' || (!e.text.trim()&&!e.image) || (e.image&&!validPhoto(e.image)) || !Number.isSafeInteger(e.created) || e.created<0 || e.created>8640000000000000 || !Number.isSafeInteger(e.updated) || e.updated<e.created || e.updated>8640000000000000 || (e.stream!==undefined && (typeof e.stream!=='string'||!e.stream||e.stream.length>100))) throw Error('Invalid entry. Nothing imported.');
      return {id:e.id,text:e.text,image:e.image||'',stream:e.stream||'_',role:e.role==='assistant'?'assistant':'user',created:e.created,updated:e.updated};
    });
    const streams=new Set(entries.map(e=>e.stream));
    if(data.version===3 && Array.isArray(data.streams)) for(const s of data.streams) {
      if(typeof s.id!=='string'||!s.id||s.id.length>100) throw Error('Invalid stream.');
      streams.add(s.id);
    }
    await new Promise((resolve,reject)=>{
      const tx=this.db.transaction(['entries','streams'],'readwrite');tx.oncomplete=resolve;tx.onerror=()=>reject(tx.error);
      for(const id of streams) { const r=tx.objectStore('streams').get(id);r.onsuccess=()=>{if(!r.result)tx.objectStore('streams').put({id,created:Date.now()});}; }
      for(const entry of entries) { const r=tx.objectStore('entries').get(entry.id);r.onsuccess=()=>{if(!r.result)tx.objectStore('entries').put(entry);}; }
    });
  }
}
