// Run against a locally built binary: MALTEN_BINARY=/path/to/malten node scripts/test-timeline.cjs
const {chromium} = require('playwright');
const {spawn} = require('node:child_process');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
(async () => {
  const server = spawn(process.env.MALTEN_BINARY || './malten', [], {env:{...process.env, OPENAI_API_KEY:'', WHATSAPP_CLIENT:'false'},stdio:'ignore'});
  let browser;
  try {
    for (let i=0;i<50;i++) { try { if ((await fetch('http://localhost:9090/')).ok) break; } catch {} await new Promise(r=>setTimeout(r,100)); }
    browser = await chromium.launch({headless:true, executablePath:process.env.CHROMIUM_PATH || undefined, args:['--no-sandbox']});
    const context = await browser.newContext({viewport:{width:390,height:844}});
    const page = await context.newPage();
    // Optional local CDN fixture for network-restricted test environments.
    if (process.env.JQUERY_PATH) await page.route('**/ajax/libs/jquery/2.1.3/jquery.min.js', route => route.fulfill({path:process.env.JQUERY_PATH,contentType:'application/javascript'}));
    const apiRequests = [], errors=[];
    page.on('request', r=>{if (/\/(messages|events|commands|streams)(\?|$)/.test(r.url())) apiRequests.push(r.url());});
    page.on('pageerror', e=>errors.push(e.message));
    await page.goto('http://localhost:9090/');
    await page.waitForFunction(()=>!document.getElementById('thought').disabled);
    await page.locator('#thought').fill('An unfinished thought');
    await page.reload();
    await page.waitForFunction(()=>document.getElementById('thought').value==='An unfinished thought');
    await page.locator('#save').click();
    await page.waitForFunction(()=>document.querySelector('.entry-text')?.textContent==='An unfinished thought');
    await page.reload();
    await page.waitForSelector('.entry');
    assert.equal(await page.locator('#thought').inputValue(),'');
    await page.locator('.entry').getByRole('button',{name:'Edit',exact:true}).click();
    await page.locator('#thought').fill('I need a little room to think.');
    await page.locator('#save').click();
    await page.waitForFunction(()=>document.querySelector('.entry-text')?.textContent==='I need a little room to think.');
    // A failed local write must retain the user's draft.
    await page.evaluate(()=>{window.originalTransaction=IDBDatabase.prototype.transaction;IDBDatabase.prototype.transaction=function(names,mode,...rest){if(mode==='readwrite')throw new Error('Simulated disk failure');return window.originalTransaction.call(this,names,mode,...rest);};});
    await page.locator('#thought').fill('Keep this draft if storage fails.'); await page.locator('#save').click();
    await page.waitForFunction(()=>document.getElementById('status').textContent.startsWith('Could not save'));
    assert.equal(await page.locator('#thought').inputValue(),'Keep this draft if storage fails.');
    await page.evaluate(()=>{IDBDatabase.prototype.transaction=window.originalTransaction;});
    const attack='<img src=x onerror="window.injected=true">';
    await page.locator('#thought').fill(attack);
    await page.locator('#save').click();
    await page.waitForFunction(()=>document.querySelectorAll('.entry').length===2);
    assert.equal(await page.locator('.entry img').count(),0);
    assert.equal(await page.evaluate(()=>window.injected),undefined);
    await page.locator('#search').fill('little room');
    await page.waitForFunction(()=>document.querySelectorAll('.entry').length===1);
    await page.locator('#search').fill('');
    await page.waitForFunction(()=>document.querySelectorAll('.entry').length===2);
    // Export can move the timeline to a fresh browser profile, and repeated import is idempotent.
    await page.locator('summary').click();
    const downloadWait=page.waitForEvent('download'); await page.locator('#export').click();
    const download=await downloadWait; const exported=JSON.parse(await fs.readFile(await download.path(),'utf8'));
    assert.equal(exported.entries.length,2);
    await page.locator('summary').click();
    await page.locator('#import-file').setInputFiles({name:'thoughts.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(exported))});
    await page.waitForFunction(()=>document.getElementById('status').textContent.startsWith('Import complete'));
    assert.equal(await page.locator('.entry').count(),2);
    const other=await browser.newContext(); const fresh=await other.newPage(); await fresh.goto('http://localhost:9090/');
    await fresh.waitForFunction(()=>!document.getElementById('thought').disabled);
    assert.equal(await fresh.locator('.entry').count(),0);
    await fresh.locator('#import-file').setInputFiles({name:'thoughts.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(exported))});
    await fresh.waitForFunction(()=>document.querySelectorAll('.entry').length===2);
    await other.close();
    // Local capture never calls the ephemeral APIs, including while offline.
    assert.deepEqual(apiRequests,[]);
    await page.evaluate(()=>navigator.serviceWorker.ready);
    await page.waitForFunction(()=>!!navigator.serviceWorker.controller);
    const cdp=await context.newCDPSession(page);
    assert.deepEqual((await cdp.send('Page.getInstallabilityErrors')).installabilityErrors,[]);
    await context.setOffline(true); await page.reload();
    await page.waitForFunction(()=>!document.getElementById('thought').disabled);
    await page.locator('#thought').fill('A thought saved without a connection.'); await page.locator('#save').click();
    await page.waitForFunction(()=>document.querySelectorAll('.entry').length===3);
    await page.reload(); await page.waitForFunction(()=>document.querySelectorAll('.entry').length===3);
    assert.deepEqual(apiRequests,[]); await context.setOffline(false);
    page.once('dialog', d=>d.accept()); await page.locator('.entry').first().getByRole('button',{name:'Delete',exact:true}).click();
    await page.waitForFunction(()=>document.querySelectorAll('.entry').length===2);
    // Mobile layout and screenshot for review.
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),true);
    if (process.env.SCREENSHOT_PATH) await page.screenshot({path:process.env.SCREENSHOT_PATH,fullPage:true});
    // Sharing stages a draft; it must never submit it on the user's behalf.
    await page.locator('.entry').filter({hasText:'little room'}).getByRole('button',{name:'Share…',exact:true}).click();
    const posts=[]; page.on('request',r=>{if(r.method()==='POST')posts.push(r.url());});
    await page.locator('#share-continue').click();
    await page.waitForURL('**/streams.html#_');
    await page.waitForFunction(()=>document.getElementById('prompt').value==='I need a little room to think.');
    assert.deepEqual(posts,[]);
    // Public messages and metadata must not execute scripts in the local-storage origin.
    await page.evaluate(text=>displayMessages([{Id:'xss-test',Text:text,Created:String(Date.now()*1e6),Metadata:{Type:'article',Site:'test',Title:text,Description:text,Image:'javascript:window.injected=true',Url:'javascript:window.injected=true'}}],1),attack);
    assert.equal(await page.evaluate(()=>window.injected),undefined);
    assert.equal(await page.locator('#messages [onerror]').count(),0);
    await page.goto('http://localhost:9090/#old-stream'); await page.waitForURL('**/streams.html#old-stream');
    assert.deepEqual(errors,[]);
    console.log('PASS: drafts, save/reload, edits, search, safe rendering, export/import, isolated device stores, offline capture, deletion, mobile layout, explicit sharing and legacy stream links.');
  } finally {if(browser) await browser.close(); server.kill();}
})().catch(error=>{console.error(error);process.exitCode=1;});
