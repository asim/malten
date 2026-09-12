// Durable storage for posts waiting to send.
(function () {
  // One record per capture: photos do not compete with localStorage's small quota.
  const databases={};
  function dbStore(name, mode, action) {
    if (!databases[name]) databases[name] = new Promise((resolve, reject) => {
      const request = indexedDB.open(name==='posts'?'malten_outbox':'malten_memory', 1);
      request.onupgradeneeded = () => request.result.createObjectStore(name,{keyPath:'id'});
      request.onsuccess = () => {request.result.onversionchange=()=>{request.result.close();delete databases[name];};resolve(request.result);};
      request.onerror = () => { delete databases[name]; reject(request.error); };
    });
    return databases[name].then(db => new Promise((resolve, reject) => {
      const transaction = db.transaction(name, mode);
      const request = action(transaction.objectStore(name));
      transaction.oncomplete = () => resolve(request.result);
      transaction.onabort = () => reject(transaction.error || new Error('Storage unavailable'));
      transaction.onerror = () => reject(transaction.error);
    }));
  }

  // Keep what was seen, not an unbounded archive: 100 per stream, 500 total,
  // at most 50 MB of text and cached image data. Oldest memories leave first.
  function boundMemory(posts){
    const counts=new Map(),kept=[];let size=0;
    for(const p of [...posts].sort((a,b)=>b.created_at-a.created_at)){
      const bytes=new TextEncoder().encode(JSON.stringify(p)).length;
      if(kept.length>=500||(counts.get(p.stream)||0)>=100||size+bytes>50*1024*1024)continue;
      kept.push(p);size+=bytes;counts.set(p.stream,(counts.get(p.stream)||0)+1);
    }
    return kept.sort((a,b)=>a.created_at-b.created_at);
  }
  function mergeMemory(cached,current,now=Date.now()){
    const seen=new Set(current.map(p=>p.id));
    // The server was asked for known IDs too: a missing, unexpired post was
    // removed or hidden. Expired copies remain local memories.
    const old=cached.filter(p=>!seen.has(p.id)&&now-p.created_at>=86400000).map(p=>({...p,local_only:true}));
    const prior=new Map(cached.map(p=>[p.id,p]));
    return boundMemory([...old,...current.map(p=>({...p,photo_data:prior.get(p.id)?.photo_data,local_only:false}))]);
  }
  window.Malten = {
    timeline:{bound:boundMemory,merge:mergeMemory},
    memory:{
      list:()=>dbStore('memory','readonly',store=>store.getAll()),
      save:posts=>dbStore('memory','readwrite',store=>{store.clear();for(const p of boundMemory(posts))store.put(p);return {}; }),
    },
    outbox: {
      list: () => dbStore('posts','readonly', store => store.getAll()),
      put: post => dbStore('posts','readwrite', store => store.put(post)),
      remove: id => dbStore('posts','readwrite', store => store.delete(id)),
    },
  };
})();
