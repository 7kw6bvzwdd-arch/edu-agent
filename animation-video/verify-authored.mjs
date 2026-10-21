// Trusted preflight; generated scene source never executes outside the sandbox.
import fs from 'node:fs';
import {createRequire} from 'node:module';
import {createHash} from 'node:crypto';
const require=createRequire(import.meta.url),puppeteer=require('puppeteer-core');
const input=JSON.parse(fs.readFileSync(0,'utf8'));
if(!Array.isArray(input.scenes)||input.scenes.length<1||input.scenes.length>12)throw Error('Invalid scenes');
const browser=await puppeteer.launch({headless:true,protocolTimeout:5000,executablePath:process.env.ANIMATION_CHROME_PATH||process.env.CHROME_PATH||(process.platform==='win32'?'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe':'/usr/bin/chromium'),args:['--site-per-process',...(process.env.CONTAINER_BROWSER==='1'?['--no-sandbox','--disable-dev-shm-usage']:[])]});
const watchdog=setTimeout(()=>{browser.process()?.kill();process.exitCode=1},40000);
try{
 const page=await browser.newPage();await page.setViewport({width:1280,height:720});const errors=[];
 page.on('pageerror',()=>errors.push('script_error'));page.on('console',m=>{if(m.type()==='error')errors.push('console_error')});
 await page.setRequestInterception(true);page.on('request',r=>{if(r.url().startsWith('data:')||['about:blank','about:srcdoc'].includes(r.url()))r.continue();else{errors.push('resource');r.abort()}});
 const results=[];
 for(const scene of input.scenes){
  if(typeof scene.html!=='string'||scene.html.length>200000||!Array.isArray(scene.frames))throw Error('Invalid scene source');
  await page.setContent('<body style="margin:0"><iframe sandbox="allow-scripts" style="border:0;width:1280px;height:720px"></iframe>');
  const csp="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; base-uri 'none'; form-action 'none';";
  await page.$eval('iframe',(f,v)=>f.srcdoc='<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="'+v.csp+'"><style>html,body{margin:0;width:1280px;height:720px;overflow:hidden}*,*::before,*::after{animation:none!important;transition:none!important}</style>'+v.html,{html:scene.html,csp});
  let frame;for(let i=0;i<60;i++){frame=page.frames().find(f=>f.url()==='about:srcdoc');if(frame)break;await new Promise(r=>setTimeout(r,30))}if(!frame)throw Error('Load timeout');
  await frame.waitForFunction(()=>document.readyState==='complete',{timeout:5000});
  await frame.evaluate(async picture=>{await document.fonts.ready;for(const el of document.querySelectorAll('[data-teaching-picture]')){if(!picture)throw Error('Missing teacher picture');el.src=picture;await el.decode()}},input.picture||null);
  const states=[],hashes=[],captures=[];
  const snapshot=state=>frame.evaluate(f=>{
   if(typeof window.renderFrame!=='function')throw Error('Missing renderFrame');
   const value=window.renderFrame(f);if(value?.then)throw Error('renderFrame must be synchronous');
   if([...document.querySelectorAll('svg,svg *')].some(el=>[...el.attributes].some(a=>['x','y','x1','y1','x2','y2','cx','cy','r','rx','ry','width','height','points','d','transform','viewBox'].includes(a.name)&&/(?:NaN|Infinity|undefined)/.test(a.value))))throw Error('invalid_geometry');
   const visit=node=>{
    if(node.nodeType===Node.TEXT_NODE)return ['text',node.data];
    if(node.nodeType!==Node.ELEMENT_NODE)return null;
    const attrs=[...node.attributes].filter(a=>a.name.toLowerCase()!=='style').map(a=>[a.name,a.value]).sort((a,b)=>a[0].localeCompare(b[0]));
    const styles=node.style?[...node.style].sort().map(name=>[name,node.style.getPropertyValue(name),node.style.getPropertyPriority(name)]):null;
    return ['element',node.namespaceURI,node.localName,attrs,styles,[...node.childNodes].map(visit)];
   };
   return JSON.stringify(visit(document.body));
  },state);
  for(const value of scene.frames){
   states.push(await snapshot(value));
   await frame.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
   const capture=await page.screenshot({type:'png'});captures.push(capture);hashes.push(createHash('sha256').update(capture).digest('hex'));
  }
  const mid=Math.floor(scene.frames.length/2);
  const back=await snapshot(scene.frames[mid]);
  await frame.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
  const backCapture=await page.screenshot({type:'png'}),backHash=createHash('sha256').update(backCapture).digest('hex');if(process.env.DIRECT_FRAME_DEBUG){fs.writeFileSync(process.env.DIRECT_FRAME_DEBUG+'/before-'+scene.id+'.png',captures[mid]);fs.writeFileSync(process.env.DIRECT_FRAME_DEBUG+'/after-'+scene.id+'.png',backCapture)}
  let pixelMatch=backHash===hashes[mid];
  if(!pixelMatch)pixelMatch=await page.evaluate(async pair=>{
   const pixels=await Promise.all(pair.map(async value=>{const img=new Image();img.src='data:image/png;base64,'+value;await img.decode();const canvas=document.createElement('canvas');canvas.width=1280;canvas.height=720;const ctx=canvas.getContext('2d');ctx.drawImage(img,0,0);return ctx.getImageData(0,0,1280,720).data}));
   // Chromium Canvas antialiasing may vary by one channel level after a clear.
   for(let i=0;i<pixels[0].length;i++)if(Math.abs(pixels[0][i]-pixels[1][i])>2)return false;
   return true;
  },[captures[mid].toString('base64'),backCapture.toString('base64')]);
  if(back!==states[mid]||!pixelMatch)throw Error('Nondeterministic reverse seek in '+scene.id);
  if(errors.length)throw Error(errors.includes('resource')?'resource_blocked':errors.includes('script_error')?'script_error':'console_error');
  const changed=new Set(hashes).size>1;
  if(scene.id===input.representative&&!changed)throw Error('Representative scene has no visible change at sampled cue times');
  results.push({id:scene.id,changed,samples:hashes.length,reverse_seek:true});
 }
 process.stdout.write(JSON.stringify({version:'authored-animation-v1',scenes:results}));
}catch(error){const text=String(error?.message||error);const reason=text.includes('invalid_geometry')||/NaN|Infinity|Expected number/.test(text)?'invalid_geometry':text.includes('Nondeterministic reverse seek')?'nondeterministic_reverse_seek':text.includes('no visible change')?'no_visible_change':text.includes('resource_blocked')?'resource_blocked':text.includes('script_error')?'script_error':text.includes('console_error')?'console_error':'browser_validation_failed';process.stdout.write(JSON.stringify({error:reason}))}finally{clearTimeout(watchdog);await browser.close()}
