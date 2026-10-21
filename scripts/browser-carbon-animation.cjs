// Fixed author fixture + real offline player and rendered model pixels.
const assert=require('node:assert/strict'),fs=require('node:fs/promises'),path=require('node:path');
const {pathToFileURL}=require('node:url');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[3]||path.join(root,'work/animation-carbon-20261020'));
 await fs.mkdir(out,{recursive:true});
 const browser=await puppeteer.launch({executablePath:process.env.ANIMATION_CHROME_PATH||'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--disable-background-networking'],defaultViewport:{width:1440,height:1080}});
 try{
  const page=await browser.newPage(),errors=[],requests=[];
  page.on('pageerror',e=>errors.push(e.message));
  page.on('request',r=>{if(!/^(file|data|blob):/.test(r.url()))requests.push(r.url())});
  await page.setOfflineMode(true);
  await page.goto(pathToFileURL(path.resolve(process.argv[2]||path.join(out,'carbon.html'))).href);
  await page.waitForFunction(()=>!!window.__anim);
  const scenes=await page.evaluate(()=>JSON.parse(document.getElementById('anim-data').textContent).timeline.scenes);
  const report=[];
  for(const [i,span] of scenes.entries()){
   const action=span.actions.find(a=>a.type==='tween');assert.ok(action);
   const snapshot=t=>page.evaluate(t=>{__anim.seek(t);const image=document.querySelector('[data-scientific]');return{frame:image.dataset.frame,image:image.getAttribute('href'),subtitle:document.querySelector('#subtitle').textContent}},t);
   const start=await snapshot(action.start);
   await page.screenshot({path:path.join(out,span.id+'-start.png')});
   const end=await snapshot(action.end);
   assert.notEqual(start.image,end.image);assert.equal(start.frame,'0');assert.equal(end.frame,'25');
   await page.screenshot({path:path.join(out,span.id+'-end.png')});
   assert.deepEqual(await snapshot(action.start),start);
   report.push({scene:span.id,first:Number(start.frame),last:Number(end.frame),changed:true,backwardSeek:true});
   if(i===1){await snapshot(action.end);await page.screenshot({path:path.join(out,'preview.png')})}
  }
  assert.deepEqual(errors,[]);assert.deepEqual(requests,[]);
  await fs.writeFile(path.join(out,'browser-report.json'),JSON.stringify({mode:'fixed fixture; real renderer; no model or speech provider',report,errors,requests},null,2));
  console.log('PASS: four scenes, visible frame changes, deterministic seek and offline playback');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
