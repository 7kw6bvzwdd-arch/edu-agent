// Trusted bridge: generated code executes only inside an opaque browser frame.
import fs from 'node:fs';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url), puppeteer=require('puppeteer-core');
const input=JSON.parse(fs.readFileSync(0,'utf8'));
if(typeof input.html!=='string'||input.html.length>200000)throw Error('Invalid authored page');
const executablePath=process.env.ANIMATION_CHROME_PATH||process.env.CHROME_PATH||(process.platform==='win32'?'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe':'/usr/bin/chromium');
const browser=await puppeteer.launch({executablePath,headless:true,protocolTimeout:5000,args:['--site-per-process',...(process.env.CONTAINER_BROWSER==='1'?['--no-sandbox','--disable-dev-shm-usage']:[])]});
const watchdog=setTimeout(()=>{browser.process()?.kill();process.exitCode=1;},14000);
try {
 const page=await browser.newPage();await page.setViewport({width:1280,height:720,deviceScaleFactor:1});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});
 await page.setRequestInterception(true);page.on('request',r=>{if(r.url().startsWith('data:')||r.url()==='about:blank'||r.url()==='about:srcdoc')r.continue();else{errors.push('External resource denied');r.abort()}});
 await page.setContent('<html><body style="margin:0"><iframe sandbox="allow-scripts" style="border:0;width:1280px;height:720px"></iframe></body></html>');
 const csp="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; media-src 'none'; base-uri 'none'; form-action 'none';";
 await page.$eval('iframe',(f,v)=>f.srcdoc='<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="'+v.csp+'"><style>html,body{margin:0;width:1280px;height:720px;overflow:hidden}*,*::before,*::after{box-sizing:border-box;animation:none!important;transition:none!important}</style>'+v.html,{html:input.html,csp});
 let frame;for(let n=0;n<60;n++){frame=page.frames().find(f=>f.url()==='about:srcdoc');if(frame)break;await new Promise(r=>setTimeout(r,50))}
 if(!frame)throw Error('Author frame did not load');
 await frame.waitForFunction(()=>document.readyState==='complete',{timeout:5000});
 await frame.evaluate(async picture=>{
  await document.fonts.ready;
  const images=[...document.querySelectorAll('[data-teaching-picture]')];
  if(picture&&!images.length)throw Error('Approved picture needs data-teaching-picture placement');
  for(const image of images){
   if(image.tagName!=='IMG')throw Error('Picture placement must be an img');
   image.src=picture||'data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="400" height="300"><rect x="2" y="2" width="396" height="296" fill="#edf2f3" stroke="#9aabb0" stroke-width="3" stroke-dasharray="10 8"/></svg>');
   await image.decode();
  }
  if(typeof window.renderFrame==='function')await window.renderFrame({time:0,progress:0,duration:0,cues:{}});
 },input.picture||null);
 const elements=await frame.evaluate(()=>{
  const items=[],ids=new Set(),walker=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);let node;
  while(node=walker.nextNode()){
   if(!node.textContent.trim()||node.parentElement.closest('script,style'))continue;
   const s=getComputedStyle(node.parentElement),range=document.createRange();range.selectNodeContents(node);const r=range.getBoundingClientRect();
   if(s.visibility!=='hidden'&&s.display!=='none'&&r.width>0&&r.height>0&&!node.parentElement.closest('[data-editable]'))throw Error('All visible text needs data-editable');
  }
  for(const el of document.querySelectorAll('[data-editable]')){
   const id=el.getAttribute('data-editable'),r=el.getBoundingClientRect(),s=getComputedStyle(el);
   if(!id||ids.has(id)||el.querySelector('[data-editable]'))throw Error('Editable IDs must be unique and unnested');ids.add(id);
   if(s.display==='none'||s.visibility==='hidden')continue;
   if(r.x<-.5||r.y<-.5||r.right>1280.5||r.bottom>720.5||r.width<1||r.height<1)throw Error('Editable element outside page');
   if(el.scrollWidth>el.clientWidth+2&&el.clientWidth>0||el.scrollHeight>el.clientHeight+2&&el.clientHeight>0)throw Error('Editable text overflows');
   for(let parent=el;parent;parent=parent.parentElement)if(getComputedStyle(parent).transform!=='none')throw Error('Editable text cannot use transforms');
   const text=el.innerText??el.textContent;if(!text.trim())throw Error('Empty editable element');
   const lines=[],tw=document.createTreeWalker(el,NodeFilter.SHOW_TEXT);let tn;
   while(tn=tw.nextNode()){
    const cs=getComputedStyle(tn.parentElement);if(cs.display==='none'||cs.visibility==='hidden')continue;
    const style={font:cs.fontFamily,size:parseFloat(cs.fontSize),color:tn.parentElement instanceof SVGElement?cs.fill:cs.color,bold:parseInt(cs.fontWeight)>=600,italic:cs.fontStyle==='italic'};
    for(let i=0;i<tn.length;i++){
     const range=document.createRange();range.setStart(tn,i);range.setEnd(tn,i+1);const cr=range.getBoundingClientRect();if(cr.width<.1||cr.height<1)continue;
     let line=lines.find(l=>Math.abs(l.y-cr.y)<3);if(!line){line={x:cr.x,y:cr.y,right:cr.right,height:cr.height,runs:[]};lines.push(line)}
     line.x=Math.min(line.x,cr.x);line.right=Math.max(line.right,cr.right);line.height=Math.max(line.height,cr.height);
     const key=JSON.stringify(style),last=line.runs.at(-1);if(last?.key===key)last.text+=tn.textContent[i];else line.runs.push({...style,key,text:tn.textContent[i]});
    }
   }
   if(!lines.length)throw Error('Editable text has no measurable characters');
   items.push({id,text,lines:lines.map(l=>({...l,width:l.right-l.x}))});
   for(const part of [el,...el.querySelectorAll('*')]){part.style.setProperty('color','transparent','important');part.style.setProperty('-webkit-text-fill-color','transparent','important');part.style.setProperty('text-shadow','none','important');if(part instanceof SVGElement){part.style.setProperty('fill','transparent','important');part.style.setProperty('stroke','transparent','important')}}
  }
  const occupied=items.flatMap(item=>item.lines.map(line=>({...line,id:item.id})));
  for(let i=0;i<occupied.length;i++)for(let j=i+1;j<occupied.length;j++){
   const a=occupied[i],b=occupied[j];if(a.id===b.id)continue;
   const w=Math.min(a.right,b.right)-Math.max(a.x,b.x),h=Math.min(a.y+a.height,b.y+b.height)-Math.max(a.y,b.y);
   if(w>2&&h>Math.min(a.height,b.height)*.35)throw Error('Editable labels overlap');
  }
  return items;
 });
 if(!elements.length)throw Error('Page needs editable text');
 const screenshot=await page.screenshot({type:'png'});
 if(errors.length)throw Error(errors.slice(0,3).join('; '));
 process.stdout.write(JSON.stringify({elements,png:screenshot.toString('base64'),version:'authored-html-v2'}));
}catch(error){
 // Return only an allowlisted repair category, never source or browser traces.
 const categories={
  'All visible text needs data-editable':'authored_text_unmarked',
  'Editable labels overlap':'authored_text_overlap',
  'Editable element outside page':'authored_text_outside',
  'Editable text overflows':'authored_text_overflow',
  'Editable text cannot use transforms':'authored_text_transform',
  'Editable IDs must be unique and unnested':'authored_text_ids',
  'Empty editable element':'authored_text_empty',
  'Editable text has no measurable characters':'authored_text_unmeasurable',
 };
 process.stdout.write(JSON.stringify({error:categories[error.message]||'authored_page_invalid'}));
}finally{clearTimeout(watchdog);await browser.close()}
