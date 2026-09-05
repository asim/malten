const {readFileSync}=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const {webcrypto}=require('node:crypto');
const elements=new Map();
function element(){return {children:[],value:'',style:{},elements:[],classList:{add(){},remove(){}},setAttribute(){},addEventListener(){},append(...v){this.children.push(...v)},appendChild(v){this.children.push(v)},replaceChildren(){this.children=[]},removeAttribute(){}}}
const document={getElementById(id){if(!elements.has(id))elements.set(id,element());return elements.get(id)},createElement:element,createTextNode:t=>({textContent:t})};
const data={points:[{id:'private',text:'old private thought',created_at:1}]};
const listeners={},window={location:{hash:'#x'},confirm:()=>true,addEventListener(name,fn){listeners[name]=fn}};
let sent,fail=false;
const storage=new Map();
const context={document,window,navigator:{},Intl,Date,Uint8Array,crypto:webcrypto,localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)},setInterval(){},Malten:{getNetwork:()=>data},fetch:async(url,opts)=>{
 if(opts.method==='POST'){sent=JSON.parse(opts.body);return {ok:!fail,text:async()=> 'Moderation unavailable'};}
 return {ok:true,json:async()=>sent?[{id:'shared',...sent,created_at:2,mine:true}]:[]};
}};
const source=readFileSync('server/web/page-map.html','utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
(async()=>{
 vm.runInNewContext(source,context);await new Promise(setImmediate);
 assert.equal(elements.get('stream').children.length,0,'private captures must not be public');
 elements.get('thought').value='a reflection';
 await elements.get('composer').onsubmit({preventDefault(){}});
 assert.equal(sent.stream,'x');assert.equal(sent.text,'a reflection');
 assert.equal(sent.location,undefined);assert.equal(sent.agent,undefined);
 assert.equal(data.points.length,1,'old data untouched');
 assert.equal(elements.get('stream').children.length,1);
 fail=true;elements.get('thought').value='keep my draft';
 await elements.get('composer').onsubmit({preventDefault(){}});
 assert.equal(elements.get('thought').value,'keep my draft');
 assert.equal(elements.get('status').textContent,'Moderation unavailable');
 window.location.hash='#%';listeners.hashchange();await new Promise(setImmediate);
 assert(source.includes('for(let i=0;i<e.results.length;i++)'),'voice retains finalized segments');
 assert(!source.includes('/api/location'),'exact location is never sent');
 console.log('Shared streams, draft retention and private migration: passed');
})().catch(err=>{console.error(err);process.exitCode=1;});
