// Rendered playback checks, including a parabola through a=0 and disconnected inverse branches.
const assert=require('node:assert/strict'),path=require('node:path'),fs=require('node:fs/promises');
const {pathToFileURL}=require('node:url');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const root=path.resolve(__dirname,'..'),out=path.join(root,'work/function-families');
 const browser=await puppeteer.launch({executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--disable-background-networking'],defaultViewport:{width:1360,height:1000}});
 try{
  const page=await browser.newPage(),errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto(pathToFileURL(process.argv[2]||path.join(out,'functions.html')).href);
  await page.waitForFunction(()=>!!window.__anim);
  const spans=await page.evaluate(()=>JSON.parse(document.getElementById('anim-data').textContent).timeline.scenes);
  const snapshot=async(time)=>page.evaluate(t=>{
   __anim.seek(t);const p=document.querySelector('[data-function]'),v=document.querySelector('[data-feature="顶点"]');
   return {path:p?.getAttribute('d'),stroke:p?.getAttribute('stroke'),vertex:v?[+v.dataset.x,+v.dataset.y]:null,axis:document.querySelector('[data-symmetry]')?.getAttribute('data-symmetry'),text:document.getElementById('stage').textContent};
  },time);
  const action=spans[0].actions[0],start=await snapshot(action.start),middle=await snapshot((action.start+action.end)/2),end=await snapshot(action.end);
  assert.deepEqual(start.vertex,[1,-2]);assert.equal(start.axis,'1');assert.deepEqual(end.vertex,[-1,0]);
  assert.ok(start.path.length>100 && end.path.length>100 && start.path!==end.path);
  assert.equal(start.stroke,'#1767bf');assert.equal(middle.vertex,null);assert.ok(middle.text.includes('非二次函数'));
  await snapshot(action.start);await page.screenshot({path:path.join(out,'parabola-up.png')});
  await snapshot(action.end);await page.screenshot({path:path.join(out,'parabola-down.png')});
  const inverse=await snapshot(spans[1].actions[0].start);
  assert.equal((inverse.path.match(/M/g)||[]).length,2,'inverse branches stay disconnected');
  const inverseEnd=await snapshot(spans[1].actions[0].end);assert.notEqual(inverse.path,inverseEnd.path);
  await page.screenshot({path:path.join(out,'inverse.png')});
  const back=await snapshot(action.start);assert.equal(back.path,start.path,'backward seek restores exact curve');
  assert.deepEqual(errors,[]);
  await fs.writeFile(path.join(out,'browser.json'),JSON.stringify({checks:['parabola and computed vertex/symmetry','a=0 labelled as non-quadratic','inverse branches disconnected','parameter changes and backward seek','no player exceptions'],errors},null,2));
  console.log('PASS: rendered function curves, features, degeneracy, branches and seeking');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
