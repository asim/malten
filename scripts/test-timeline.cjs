// MALTEN_BINARY=/path/to/malten CHROMIUM_PATH=/path/to/chromium node scripts/test-timeline.cjs
const {chromium}=require('playwright');
const {spawn}=require('node:child_process');
const assert=require('node:assert/strict');
const fs=require('node:fs/promises');
(async()=>{
 const server=spawn(process.env.MALTEN_BINARY||'./malten',[],{env:{...process.env,OPENAI_API_KEY:'',WHATSAPP_CLIENT:'false'},stdio:'ignore'});let browser;
 try {
  for(let i=0;i<50;i++){try{if((await fetch('http://localhost:9090/')).ok)break;}catch{}await new Promise(r=>setTimeout(r,100));}
  browser=await chromium.launch({headless:true,executablePath:process.env.CHROMIUM_PATH||undefined,args:['--no-sandbox']});
  const context=await browser.newContext({viewport:{width:390,height:844}}),page=await context.newPage(),errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  // Older servers can return null for an existing stream with no messages.
  await page.route('**/messages?*',route=>route.fulfill({json:null}));
  await page.goto('http://localhost:9090/#empty');
  await page.waitForFunction(()=>!document.getElementById('prompt').disabled&&document.activeElement.id==='prompt');
  assert.equal(await page.locator('#words').textContent(),'');
  assert.equal(await page.locator('#messages li').count(),0);
  await page.unroute('**/messages?*');
  await page.goto('http://localhost:9090/manifest.webmanifest');
  // Seed the previous personal timeline schema, including its unfinished photo draft.
  await page.evaluate(async()=>{
   const png='data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a6XcAAAAASUVORK5CYII=';
   localStorage.setItem('malten-draft-v1',JSON.stringify({text:'Old draft'}));
   await new Promise((resolve,reject)=>{const r=indexedDB.open('malten-personal',2);r.onupgradeneeded=()=>{r.result.createObjectStore('entries',{keyPath:'id'});r.result.createObjectStore('drafts');};r.onerror=()=>reject(r.error);r.onsuccess=()=>{const db=r.result,tx=db.transaction(['entries','drafts'],'readwrite');tx.objectStore('entries').put({id:'old',text:'Existing thought',created:100,updated:100});tx.objectStore('drafts').put(png,'photo');tx.oncomplete=()=>{db.close();resolve();};};});
  });
  const requests=[];page.on('request',r=>{if(/\/(messages|commands|events|streams|ask)(\?|$)/.test(r.url()))requests.push(r.url());});
  await page.goto('http://localhost:9090/private');
  await page.waitForFunction(()=>document.activeElement.id==='prompt'&&document.getElementById('prompt').value==='Old draft');
  assert.equal(await page.locator('#messages .message').textContent(),'Existing thought');
  await page.waitForSelector('form .attached-photo:visible');
  await page.locator('#prompt').press('Enter');await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===2);
  await page.locator('#prompt').fill('Default-stream draft');
  await page.getByRole('link',{name:'New',exact:true}).click();await page.waitForFunction(()=>document.getElementById('prompt').value==='');
  const second=page.url();
  await page.locator('#prompt').fill('Only in the second stream');await page.locator('#prompt').press('Enter');
  await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===1);
  await page.locator('#prompt').fill('Line one');await page.locator('#prompt').press('Shift+Enter');
  assert.equal(await page.locator('#prompt').inputValue(),'Line one\n');
  await page.locator('#prompt').evaluate(el=>el.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',isComposing:true,bubbles:true})));
  assert.equal(await page.locator('#messages li').count(),1);
  await page.goto('http://localhost:9090/private');await page.waitForFunction(()=>document.getElementById('prompt').value==='Default-stream draft');
  assert.equal(await page.locator('#messages li').count(),2);
  await page.goto(second);await page.waitForFunction(()=>document.getElementById('prompt').value==='Line one\n');
  assert.equal(await page.locator('#messages li').count(),1);
  await page.evaluate(()=>{window.SpeechRecognition=class{start(){this.onresult({results:[[{transcript:'Spoken'}]]});this.onend();}stop(){this.onend();}};});
  await page.locator('#prompt').fill('');await page.getByRole('button',{name:'Talk',exact:true}).click();assert.equal(await page.locator('#prompt').inputValue(),'Spoken');
  const png=await page.evaluate(()=>{const c=document.createElement('canvas');c.width=8;c.height=8;return c.toDataURL().split(',')[1];});
  await page.locator('input[type=file][accept="image/*"]').first().setInputFiles({name:'photo.png',mimeType:'image/png',buffer:Buffer.from(png,'base64')});await page.waitForSelector('form .attached-photo:visible');
  await page.reload();await page.waitForSelector('form .attached-photo:visible');await page.locator('#prompt').press('Enter');await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===2);
  assert.deepEqual(requests,[]);
  let aiRequest;
  await page.route('**/ask',route=>{aiRequest=route.request().postDataJSON();return route.fulfill({json:{text:'A contextual reply'}});});
  await page.locator('#prompt').fill('/malten what did I say?');assert.equal(aiRequest,undefined);await page.locator('#prompt').press('Enter');
  await page.waitForFunction(()=>Array.from(document.querySelectorAll('#messages .message')).some(el=>el.textContent==='A contextual reply'));
  assert.equal(Object.keys(aiRequest).join(','),'messages');
  assert(aiRequest.messages.some(m=>m.Text==='Only in the second stream'));
  assert(!aiRequest.messages.some(m=>m.Text==='Existing thought'));
  assert(!JSON.stringify(aiRequest).includes(second.split('#')[1]));
  await page.reload();await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===4);
  const shell=await page.locator('main').boundingBox();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
  if(process.env.SCREENSHOT_PATH)await page.screenshot({path:process.env.SCREENSHOT_PATH,fullPage:true});
  await page.getByLabel('Open menu').click();const downloadWait=page.waitForEvent('download');await page.locator('#export').click();
  const data=JSON.parse(await fs.readFile(await(await downloadWait).path(),'utf8'));assert.equal(data.version,3);assert.equal(data.streams.length,2);assert.equal(data.entries.length,6);
  await page.getByLabel('Open menu').click();
  const other=await browser.newContext(),fresh=await other.newPage();await fresh.goto('http://localhost:9090/private');await fresh.waitForFunction(()=>!document.getElementById('prompt').disabled);
  await fresh.locator('#import-file').setInputFiles({name:'streams.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(data))});await fresh.waitForFunction(()=>document.getElementById('words').textContent==='Imported.');
  await fresh.goto(second);await fresh.waitForFunction(()=>document.querySelectorAll('#messages li').length===4);await other.close();
  await page.evaluate(()=>navigator.serviceWorker.ready);await page.waitForFunction(()=>!!navigator.serviceWorker.controller);
  const cdp=await context.newCDPSession(page);assert.deepEqual((await cdp.send('Page.getInstallabilityErrors')).installabilityErrors,[]);
  await context.setOffline(true);await page.reload();await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===4);
  await page.locator('#prompt').fill('Offline');await page.locator('#prompt').press('Enter');await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===5);await context.setOffline(false);
  // Local sharing requires confirmation; regular capture has used no public stream APIs.
  assert(!requests.some(u=>/\/(messages|commands|streams|events)(\?|$)/.test(u)));
  await page.getByRole('link',{name:'Share',exact:true}).click();await page.waitForSelector('#share-dialog[open]');
  await page.locator('#share-confirm').click();await page.waitForURL('http://localhost:9090/#*');await page.waitForFunction(()=>document.querySelectorAll('#messages li').length===5);
  assert.equal((await page.locator('main').boundingBox()).width,shell.width);
  const publicID=decodeURIComponent(new URL(page.url()).hash.slice(1));const listing=await(await fetch('http://localhost:9090/streams')).json();assert(!listing[publicID]);
  await page.locator('#prompt').fill('/malten a public question');await page.locator('#prompt').press('Enter');
  await page.waitForFunction(()=>Array.from(document.querySelectorAll('#messages .message')).some(el=>el.textContent==='/malten a public question'));
  await page.reload();await page.waitForFunction(()=>Array.from(document.querySelectorAll('#messages .message')).some(el=>el.textContent==='/malten a public question'));
  const beforeGoto=await page.locator('#form').boundingBox();
  assert.equal(await page.locator('.brand').textContent(),'~ malten');
  assert.equal(await page.locator('header #current').textContent(),'#'+publicID);
  assert.equal(await page.title(),'#'+publicID+' ~ malten');
  await page.locator('#goto-toggle').click();
  assert.deepEqual(await page.locator('#form').boundingBox(),beforeGoto);
  assert.equal(await page.locator('#gstream').evaluate(el=>getComputedStyle(el).outlineStyle),'none');
  await page.locator('#gstream').press('Escape');
  await page.locator('#prompt').fill('/malten');await page.locator('#prompt').press('Enter');
  await page.waitForFunction(()=>Array.from(document.querySelectorAll('#messages .message')).some(el=>el.textContent==='/malten'));
  const history=await(await fetch('http://localhost:9090/messages?stream='+encodeURIComponent(publicID)+'&limit=1024')).json();
  assert(history.some(m=>m.Type==='command'&&m.Text===''));
  const attack='<img src=x onerror="window.injected=true">';await page.locator('#prompt').fill(attack);await page.locator('#prompt').press('Enter');
  await page.waitForFunction(text=>Array.from(document.querySelectorAll('#messages .message')).some(el=>el.textContent===text),attack);assert.equal(await page.evaluate(()=>window.injected),undefined);
  assert.deepEqual(errors,[]);
  console.log('PASS: migration, stream isolation, per-stream drafts/photos, Enter, dictation, local AI context, export/import, offline/PWA, explicit sharing, shared layout, unlisted streams and public commands.');
 }finally{if(browser)await browser.close();server.kill();}
})().catch(e=>{console.error(e);process.exitCode=1;});
