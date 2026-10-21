// Local browser acceptance: authored pages must remain actual pages in presentation mode.
const fs=require('fs'),path=require('path'),assert=require('node:assert/strict');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const base=process.argv[2]||'http://127.0.0.1:18337';
 const dir=path.resolve(process.argv[3]||'work/animation-direct-20261021/courseware-final-delivery');
 const deck=JSON.parse(fs.readFileSync(path.join(dir,'deck.json'),'utf8'));
 deck.slides.forEach((s,i)=>s.id='page'+i);
 const images={};for(let i=0;i<2;i++)images['page'+i]='data:image/png;base64,'+fs.readFileSync(path.join(dir,'plants','slide-'+(i+1)+'.png')).toString('base64');
 const browser=await puppeteer.launch({headless:true,executablePath:process.env.ANIMATION_CHROME_PATH||'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'});
 try{
  const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.setRequestInterception(true);page.on('request',r=>new URL(r.url()).pathname==='/__direct-check'?r.respond({status:200,contentType:'text/html',body:'<!doctype html><meta charset="utf-8"><div id="direct-root"></div>'}):r.continue());
  await page.goto(base+'/__direct-check');
  await page.evaluate(async({deck,images})=>{const {createApp}=await import('/node_modules/.vite/deps/vue.js');const {default:Component}=await import('/src/ClassroomPresentation.vue');createApp(Component,{deck,images,saved:true,token:''}).mount('#direct-root')},{deck,images});
  await page.waitForSelector('.presentation-page > img');
  assert.equal(await page.$eval('.presentation-page > img',e=>e.src),images.page0);
  assert.equal(await page.$eval('.presentation-page',e=>!!e.querySelector('ol')),false);
  assert.equal(await page.$eval('header',e=>e.textContent.includes('逐步讲解')),false);
  const next=async()=>{const handles=await page.$$('footer button');for(const h of handles){if((await h.evaluate(e=>e.textContent)).trim()==='下一步'){await h.click();return}}throw Error('Missing next control')};
  await next();assert.equal(await page.$eval('.presentation-page > img',e=>e.src),images.page1);
  await next();assert.equal(await page.$eval('.presentation-page',e=>e.textContent.includes('实际生成页面尚未加载')),true);
  assert.equal(await page.$eval('.presentation-page',e=>!!e.querySelector('ol')),false);
  assert.deepEqual(errors,[]);console.log(JSON.stringify({checks:['actual generated page','single next switches page','no summary template fallback','missing screenshot explicit','no student answer reveal'],errors}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
