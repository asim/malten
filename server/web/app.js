// Durable storage for posts waiting to send.
(function () {
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
  };
})();
