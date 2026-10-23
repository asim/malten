(async function () {
  'use strict';
  const $=id=>document.getElementById(id), local=location.pathname==='/private';
  let db, composer, active, version=0, source, poll, sending=false, restoring=false, editing=null, entries=[], aiBusy=new Set(), shared=null;
  const report=message=>{$('words').textContent=message;};
  const name=()=>{try{return decodeURIComponent(location.hash.slice(1))||'_';}catch(_){return '_';}};
  const route=id=>(local?'/private':'/')+(id==='_'?'':'#'+encodeURIComponent(id));
  const safeURL=value=>{try{const url=new URL(value);return /^https?:$/.test(url.protocol)?url.href:'';}catch(_){return '';}};
  const normal=m=>({id:m.Id,text:(m.Type==='command'?'/malten'+(m.Text?' ':''):'')+m.Text,image:m.Image||'',role:m.Role||'user',metadata:m.Metadata,created:Number(BigInt(m.Created)/1000000n),updated:Number(BigInt(m.Created)/1000000n)});
  async function request(url, options) {
    const response=await fetch(url,options);
    if(!response.ok) throw Error((await response.text()).trim()||'Could not complete the request.');
    return response;
  }
  const post=(url,data)=>request(url,{method:'POST',body:new URLSearchParams(data)});
  function setBusy(value) { sending=value;composer.setBusy(value);$('prompt').disabled=value;$('send').disabled=value; }
  function persistDraft() {
    if(restoring||!active)return;
    const draft={text:$('prompt').value,image:composer.image,editing};
    if(local) db.draft(active,draft).catch(()=>report('Could not save the draft. Keep this page open.'));
    else try{sessionStorage.setItem('malten-public-draft:'+active,JSON.stringify(draft));}catch(_){report('Could not save the draft. Keep this page open.');}
  }
  function clearComposer() { editing=null;$('prompt').value='';composer.setImage('');$('cancel-edit').hidden=true;$('send').textContent='Send';$('prompt').dispatchEvent(new Event('input')); }
  function render() {
    const fragment=document.createDocumentFragment();
    for(const entry of [...entries].sort((a,b)=>b.created-a.created || b.id.localeCompare(a.id))) {
      const item=document.createElement('li');item.dataset.id=entry.id;
      const time=document.createElement('div');time.className='time';time.textContent=(entry.role==='assistant'?'Malten · ':'')+new Date(entry.created).toLocaleString(undefined,{month:'short',day:'numeric',hour:'numeric',minute:'2-digit'});
      const message=document.createElement('div');message.className='message';message.textContent=entry.text;
      // Link only explicit web URLs; all other message content remains plain text.
      const parts=entry.text.split(/(https?:\/\/[^\s]+)/g);message.replaceChildren();
      for(const part of parts) {
        if(/^https?:\/\//.test(part)){const a=document.createElement('a');a.href=part;a.textContent=part;a.rel='noopener noreferrer';message.append(a);}else message.append(document.createTextNode(part));
      }
      item.append(time,message);
      if(validPhoto(entry.image)){const img=document.createElement('img');img.src=entry.image;img.alt='Photo';img.className='attached-photo';item.append(img);}
      if(!local && entry.metadata && safeURL(entry.metadata.Url)) {
        const preview=document.createElement('div');preview.className='link-preview';
        const link=document.createElement('a');link.href=safeURL(entry.metadata.Url);link.textContent=entry.metadata.Title||entry.metadata.Url;link.rel='noopener noreferrer';preview.append(link);
        if(safeURL(entry.metadata.Image)){const img=document.createElement('img');img.src=safeURL(entry.metadata.Image);img.alt='';img.loading='lazy';img.referrerPolicy='no-referrer';preview.append(img);}
        if(entry.metadata.Description){const description=document.createElement('p');description.textContent=entry.metadata.Description;preview.append(description);}
        item.append(preview);
      }
      if(local) {
        const actions=document.createElement('details');actions.className='message-actions';
        const summary=document.createElement('summary');summary.textContent='…';summary.setAttribute('aria-label','Message actions');actions.append(summary);
        for(const [label,action] of [
          ['Edit',()=>{if(sending)return;if(($('prompt').value||composer.image)&&!confirm('Replace the current draft?'))return;editing=entry.id;$('prompt').value=entry.text;composer.setImage(entry.image);$('cancel-edit').hidden=false;$('send').textContent='Save';$('prompt').dispatchEvent(new Event('input'));$('prompt').focus();}],
          ['Delete',async()=>{if(!confirm('Delete this message from this device?'))return;await db.remove(entry.id);if(editing===entry.id)clearComposer();await refresh();}],
          ...(/^\/malten(?:\s|$)/.test(entry.text)?[['Ask again',()=>ask(entry,active)]]:[])
        ]) {const button=document.createElement('button');button.type='button';button.textContent=label;button.onclick=()=>Promise.resolve(action()).catch(e=>report(e.message));actions.append(button);}
        item.append(actions);
      }
      fragment.append(item);
    }
    $('messages').replaceChildren(fragment);
  }
  async function refresh() {
    const target=active, stamp=version;
    const data=local ? await db.entries(target) : await (await request('/messages?stream='+encodeURIComponent(target)+'&limit=1024')).json();
    if(stamp!==version)return;
    entries=local?data:data.map(normal);render();
    if(local) {
      const streams=await db.streams();$('local-streams').replaceChildren(...streams.map(s=>{const o=document.createElement('option');o.value=s.id;return o;}));
    } else {
      const streams=await (await request('/streams?stream='+encodeURIComponent(target))).json();
      if(stamp===version)$('present').textContent=(streams[target]?.Observers||0)+' here';
    }
  }
  async function openStream() {
    if(!composer)return;
    persistDraft();active=name();const stamp=++version;restoring=true;editing=null;entries=[];render();report('');
    if(source){source.close();source=null;}clearInterval(poll);
    $('goto').hidden=true;$('goto-toggle').setAttribute('aria-expanded','false');
    $('current').textContent=active==='_'?'':'#'+active;
    $('current').href=route(active);
    $('current').title=active==='_'?'':'#'+active;
    document.title=active==='_'?'~ malten':'#'+active+' ~ malten';
    try { sessionStorage.setItem(local?'malten-local-route':'malten-public-route',route(active)); } catch(_) {}
    try {
      if(local) await db.ensure(active);
      let draft;
      if(local) draft=await db.draft(active);
      else {try{draft=JSON.parse(sessionStorage.getItem('malten-public-draft:'+active));const handoff=JSON.parse(sessionStorage.getItem('malten-stream-draft-v1'));if(handoff?.stream===active){draft=handoff;sessionStorage.removeItem('malten-stream-draft-v1');}}catch(_){}}
      if(stamp!==version)return;
      $('prompt').value=draft?.text||'';composer.setImage(draft?.image);editing=draft?.editing||null;
      $('cancel-edit').hidden=!editing;$('send').textContent=editing?'Save':'Send';
      restoring=false;$('prompt').dispatchEvent(new Event('input'));
      await refresh();
      if(!local && stamp===version) {
        source=new WebSocket(location.origin.replace(/^http/,'ws')+'/events?stream='+encodeURIComponent(active));
        source.onmessage=event=>{if(stamp!==version)return;const m=JSON.parse(event.data);if(m.Type==='message'||m.Type==='command'){const entry=normal(m);if(!entries.some(e=>e.id===entry.id)){entries.push(entry);render();}}else refresh().catch(()=>{});};
        poll=setInterval(()=>refresh().catch(()=>{}),5000);
      }
      if(stamp===version && !sending)$('prompt').focus({preventScroll:true});
    }catch(e){restoring=false;report(e.message);}
  }
  function contextFor(history) {
    let bytes=0, photos=0;const selected=[];
    for(const e of [...history].sort((a,b)=>b.created-a.created).slice(0,40)) {
      if(bytes+new TextEncoder().encode(e.text).length>32000)break;
      bytes+=new TextEncoder().encode(e.text).length;
      const image=validPhoto(e.image)&&photos++<2?e.image:'';
      selected.unshift({Text:e.text,Image:image,Role:e.role==='assistant'?'assistant':'user'});
    }
    return selected;
  }
  async function ask(entry,target) {
    if(aiBusy.has(target)){if(target===active)report('Malten is still replying.');return;}
    aiBusy.add(target);if(target===active)report('Malten is replying…');
    try {
      const history=(await db.entries(target)).filter(e=>e.created<=entry.created);
      const response=await request('/ask',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({messages:contextFor(history)}),signal:AbortSignal.timeout(65000)});
      const answer=await response.json(),now=Date.now();
      await db.put({id:crypto.randomUUID(),stream:target,text:answer.text,image:'',role:'assistant',created:now,updated:now});
      if(target===active){report('');await refresh();}
    } catch(e) {if(target===active)report('Your question is saved. '+e.message+' Use Ask again on the message to retry.');}
    finally {aiBusy.delete(target);}
  }
  $('form').onsubmit=async event=>{
    event.preventDefault();if(sending||composer.loading||(!$('prompt').value.trim()&&!composer.image))return;
    const target=active,text=$('prompt').value,image=composer.image,id=editing,command=/^\/malten(?:\s|$)/.test(text);
    if(!local && new TextEncoder().encode(text).length>1024){report('Message is too long (1,024 bytes maximum).');return;}
    if(!local && command && image){report('Send the photo first, then ask Malten about it.');return;}
    setBusy(true);
    try {
      let entry;
      if(local) {
        const previous=id?(await db.entries(target)).find(e=>e.id===id):null,now=Date.now();
        entry={id:previous?.id||crypto.randomUUID(),stream:target,text,image,role:previous?.role||'user',created:previous?.created||now,updated:now};
        await db.put(entry);
      }else await post(command?'/commands':'/messages',command?{stream:target,prompt:text.replace(/^\/malten(?:\s+|$)/,'')}:{stream:target,message:text,image});
      if(target===active){clearComposer();report('');await refresh();}
      if(local && command && !id)ask(entry,target);
    }catch(e){report(e.message);}
    finally{setBusy(false);$('prompt').focus({preventScroll:true});}
  };
  $('cancel-edit').onclick=()=>clearComposer();
  $('new').onclick=async event=>{
    if(!local)return;event.preventDefault();const id=crypto.randomUUID();await db.ensure(id);location.hash=id;
  };
  $('goto-toggle').onclick=event=>{event.preventDefault();$('goto').hidden=!$('goto').hidden;$('goto-toggle').setAttribute('aria-expanded',String(!$('goto').hidden));if(!$('goto').hidden)$('gstream').focus();};
  $('gstream').onkeydown=event=>{if(event.key==='Escape'){$('goto').hidden=true;$('goto-toggle').setAttribute('aria-expanded','false');$('goto-toggle').focus();}};
  $('goto').onsubmit=event=>{event.preventDefault();const id=$('gstream').value.trim().replace(/^#/,'')||'_';if(id.length>100){report('Stream names can be up to 100 characters.');return;}location.href=route(id);$('gstream').value='';};
  $('share').onclick=async event=>{
    event.preventDefault();
    if(local){$('share-error').textContent='';shared=null;$('share-dialog').showModal();return;}
    try {if(navigator.share)await navigator.share({title:'Malten',url:location.href});else{await navigator.clipboard.writeText(location.href);report('Link copied.');}}catch(e){if(e.name!=='AbortError')window.prompt('Copy stream link',location.href);}
  };
  $('share-confirm').onclick=async()=>{
    const button=$('share-confirm');button.disabled=true;
    try {
      if(!shared){const items=await db.entries(active);if(!items.length)throw Error('This stream is empty.');
        if(items.length>1024 || items.some(e=>new TextEncoder().encode((e.role==='assistant'?'Malten: ':'')+e.text).length>1024) || items.reduce((n,e)=>n+e.text.length+(e.image||'').length,0)>8*1024*1024)throw Error('This stream is too large to share. Export a copy instead.');
        const result=await (await post('/streams',{private:'true',stream:crypto.randomUUID()})).json();shared={id:result.stream,items:items.sort((a,b)=>a.created-b.created),index:0};}
      for(;shared.index<shared.items.length;shared.index++){const e=shared.items[shared.index];await post('/messages',{stream:shared.id,message:(e.role==='assistant'?'Malten: ':'')+e.text,image:e.image||''});}
      location.href='/#'+encodeURIComponent(shared.id);
    }catch(e){$('share-error').textContent=e.message;}
    finally{button.disabled=false;}
  };
  $('export').onclick=async()=>{try{const data=await db.export(),url=URL.createObjectURL(new Blob([JSON.stringify(data)],{type:'application/json'})),a=document.createElement('a');a.href=url;a.download='malten-streams.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),60000);}catch(e){report(e.message);}};
  $('import').onclick=()=>$('import-file').click();
  $('import-file').onchange=async()=>{try{const file=$('import-file').files[0];if(!file)return;if(file.size>100*1024*1024)throw Error('File exceeds 100 MB.');await db.import(JSON.parse(await file.text()));await refresh();report('Imported.');}catch(e){report(e.message);}finally{$('import-file').value='';}};
  try { const other=sessionStorage.getItem(local?'malten-public-route':'malten-local-route'); if(other && (local ? /^\/(?:#|$)/.test(other) : /^\/private(?:#|$)/.test(other))) $(local?'public-tab':'local-tab').href=other; }catch(_){}
  $('boundary').textContent=local?'On this device. /malten sends recent messages to AI.':'Anonymous and ephemeral. Anyone with the link can read this stream.';
  $(local?'local-tab':'public-tab').className='selected';$(local?'local-tab':'public-tab').setAttribute('aria-current','page');
  for(const id of ['export','import','storage-info'])$(id).hidden=!local;
  try {
    if(local)db=await LocalStreams.open();
    composer=createComposer($('prompt'),report,persistDraft);$('prompt').addEventListener('input',persistDraft);
    $('send').disabled=false;$('prompt').disabled=false;
    await openStream();
  }catch(e){report(e.message||'Could not open local storage.');}
  window.addEventListener('hashchange',openStream);
  window.addEventListener('pagehide',()=>{persistDraft();if(source)source.close();clearInterval(poll);});
  window.addEventListener('pageshow',e=>{if(e.persisted)openStream();});
}());
