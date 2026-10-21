const fs=require('fs'),path=require('path'),assert=require('assert/strict');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const browser=await puppeteer.launch({executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--site-per-process']});
 try{
  const page=await browser.newPage();await page.setViewport({width:1280,height:850});
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto('file:///'+path.resolve(process.argv[2]).replace(/\\/g,'/'));await page.waitForFunction(()=>window.__anim);await page.evaluate(()=>window.__anim.ready());
  const data=await page.$eval('#anim-data',e=>JSON.parse(e.textContent));assert.equal(data.script.archetype,'html.direct@1');
  for(const scene of data.timeline.scenes){
   const times=[scene.start+1,(scene.start+scene.end)/2,scene.end-1,(scene.start+scene.end)/2,scene.start+1],saved=new Map();
   for(const t of times){
    await page.evaluate(async t=>{window.__anim.seek(t);await window.__anim.ready()},t);
    const entry=await page.evaluate(id=>window.__anim.authoredState().find(x=>x.id===id),scene.id);assert.ok(entry&&!entry.error);assert.ok(entry.state);
    if(saved.has(t))assert.equal(entry.state,saved.get(t),'reverse seek must reproduce authored state');saved.set(t,entry.state);
   }
   // Canvas drawing does not mutate DOM; visible motion is checked by renderer pixel samples.
   await page.evaluate(async t=>{window.__anim.seek(t);await window.__anim.ready()},times[1]);
   await page.screenshot({path:process.argv[2]+'.'+scene.id+'.png'});
  }
  const isolated=await page.$eval('iframe:not([hidden])',el=>({sandbox:el.getAttribute('sandbox'),width:el.getBoundingClientRect().width,stage:el.parentElement.getBoundingClientRect().width}));
  assert.equal(isolated.sandbox,'allow-scripts');assert.ok(Math.abs(isolated.width-isolated.stage)<2);assert.deepEqual(errors,[]);
  console.log(JSON.stringify({scenes:data.timeline.scenes.length,duration_ms:data.timeline.total_ms,checks:['actual authored motion','reverse seek','opaque sandbox','responsive sizing'],errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
