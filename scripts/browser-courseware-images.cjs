// Browser checks use explicit mocked business/model APIs; HTTP integration separately checks the real backend.
const assert=require('node:assert/strict');
const fs=require('node:fs/promises');
const path=require('node:path');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const web=process.env.TEST_IMAGE_SOURCE==='web';
 const out=path.resolve(__dirname,web?'../work/web-images':'../work/commons-images');await fs.mkdir(out,{recursive:true});
 const browser=await puppeteer.launch({executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--disable-background-networking'],defaultViewport:{width:1440,height:1080}});
 const errors=[],checks=[];let page;
 try{
  const need={purpose:'观察城墙与敌台的连接',queries:['Great Wall Badaling'],constraints:'建筑照片'};
  const task={id:'commons-fixture',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,input:{topic:'长城建筑观察',pages:8,theme:'ocean',preview_first:true},events:[],sources:[],preview_images_available:false,slide_reviews:[],result:{title:'配图工作台固定验收',theme:'ocean',lesson:'固定样例，不代表真实模型结果',slides:Array.from({length:3},(_,i)=>({id:'slide-'+(i+1),title:['观察城墙','比较敌台','解释连接'][i],layout:'concept',bullets:['观察建筑线索并解释。'],notes:'固定验收样例，请教师核实具体历史背景。',image_need:need}))}};
  const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64');
  const provenance={provider:'wikimedia_commons',title:'File:Great Wall fixture.png',author:'Test Photographer',license:'CC BY-SA 4.0',page_url:'https://commons.wikimedia.org/wiki/File:Great_Wall.png',license_url:'https://creativecommons.org/licenses/by-sa/4.0/',description:'明确标注的来源测试响应',retrieved_at:'2026-10-19T06:00:00Z',sha256:'a'.repeat(64)};
  if(web)Object.assign(provenance,{provider:'web_page',title:'网页照片固定验收',page_url:'https://museum.example/collection/wall',license:'使用权限待确认',license_url:'',rights_status:'pending_confirmation'});
  let saved=0,searches=0,confirmations=0,delay=false;
  page=await browser.newPage();page.on('pageerror',e=>errors.push(e.message));await page.setRequestInterception(true);
  page.on('request',async r=>{
   if(!r.url().includes('/api/'))return r.continue();
   const p=new URL(r.url()).pathname;let data=[],body,contentType='application/json',status=200;
   try{
    if(p.endsWith('/images/search')){const q=JSON.parse(r.postData());assert.equal(q.slide_id,'slide-1');assert.equal(q.revision,task.revision);assert.ok(q.need.purpose);searches++;if(delay)await new Promise(resolve=>setTimeout(resolve,1200));data={status:'found',message:'固定检索候选',candidates:[{picture:{asset_id:'a'.repeat(32),caption:q.need.purpose,source:'Wikimedia Commons · CC BY-SA 4.0'},provenance,reason:'固定相关性审核结果'}]};}
    else if(p.endsWith('/images/confirm')){const q=JSON.parse(r.postData());assert.equal(q.revision,task.revision);assert.equal(q.slide_id,'slide-1');assert.ok(q.rights_note.length>=5);confirmations++;Object.assign(provenance,{rights_status:'teacher_confirmed',rights_note:q.rights_note,rights_confirmed_at:'2026-10-19T08:00:00Z',license:'教师确认使用权限（非平台授权认证）'});data={picture:{asset_id:'a'.repeat(32),caption:need.purpose,source:'网页配图 · 教师确认使用权限'},provenance,reason:'固定权限确认响应'};}
    else if(p==='/api/media'&&r.method()==='POST')data={asset_id:'b'.repeat(32),width:320,height:240};
    else if(p.startsWith('/api/media/')&&p.endsWith('/metadata'))data=p.includes('b'.repeat(32))?{asset_id:'b'.repeat(32)}:{asset_id:'a'.repeat(32),provenance};
    else if(p.startsWith('/api/media/')){body=png;contentType='image/png';}
    else if(p.endsWith('/edit')||p.endsWith('/draft')){task.result=JSON.parse(r.postData()).result;task.revision++;saved++;data={revision:task.revision,ok:true};}
    else if(p==='/api/health')data={status:'ok',mode:'sample',courseware_canvas_version:'canvas-v3',courseware_studio_version:'studio-v1',teaching_features_version:'teaching-v1',works_view_version:'views-v1'};
    else if(p==='/api/account')data={username:'teacher',name:'测试教师'};
    else if(p==='/api/auth/me')data={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate','kb.manage']};
    else if(p==='/api/tenants')data=[{id:1,name:'测试'}];
    else if(p==='/api/billing')data={balance:100,reserved:0,available:100,plan:'basic',ledger:[]};
    else if(p==='/api/tasks')data=[task];
    else if(p==='/api/tasks/'+task.id)data=task;
    else if(p==='/api/dashboard')data={totals:{},tasks:[],usage:[]};
    else if(p==='/api/usage')data={items:[],total:0};
    else if(p.endsWith('/preview')){body=png;contentType='image/png';}
    else if(r.method()!=='GET')throw Error('Unexpected write: '+p);
    await r.respond({status,contentType,body:body||JSON.stringify(data)});
   }catch(e){if(!/already handled|Target closed|Session closed/.test(e.message))errors.push(e.message);}
  });
  await page.evaluateOnNewDocument(()=>sessionStorage.setItem('edu_token','fixture-token'));
  await page.goto(process.env.TEST_APP_URL||'http://127.0.0.1:15173');
  const click=async(text,scope='body')=>page.evaluate(({text,scope})=>{const b=[...document.querySelector(scope).querySelectorAll('button')].find(b=>b.textContent.trim().includes(text));if(!b||b.disabled)throw Error('Missing button: '+text);b.click()},{text,scope});
  const scope='.studio-slide.selected';
  await page.waitForFunction(()=>[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品')));
  await click('我的作品');await page.waitForSelector('.task-open');await page.click('.task-open');
  await page.waitForFunction(()=>document.body.innerText.includes('手动编辑内容'));await click('手动编辑内容');
  await page.waitForSelector(scope+' .commons-image-search');
  const openSearch=async()=>{await page.waitForSelector(scope+' .commons-image-search details');await page.$eval(scope+' .commons-image-search details',e=>{for(let p=e;p;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;});};
  await openSearch();await click('检索并审核候选图片',scope);
  await page.waitForSelector(scope+' .commons-candidate img');
  if(web){
   assert.equal(await page.$$eval(scope+' .commons-candidate button',els=>els.find(e=>e.textContent.trim()==='使用此图').disabled),true);
   await page.$eval(scope+' .image-rights textarea',e=>{e.value='已取得作者用于教学课件及下载分发的许可';e.dispatchEvent(new Event('input',{bubbles:true}));});
   await click('记录使用权限依据',scope);
   await page.waitForFunction(scope=>[...document.querySelectorAll(scope+' .commons-candidate button')].some(e=>e.textContent.trim()==='使用此图'&&!e.disabled),{},scope);
   checks.push('Unknown webpage rights block selection until teacher declaration is recorded');
  }
  await click('使用此图',scope);
  await page.waitForSelector(scope+' .commons-attribution');
  await page.$eval(scope+' .commons-attribution',e=>{for(let p=e;p;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;});
  assert.ok((await page.$eval(scope+' .commons-attribution',e=>e.innerText)).includes('Test Photographer'));
  await click('保存本页及其他修改');await page.waitForFunction(()=>!document.querySelector('.unsaved-note'));
  assert.equal(task.result.slides[0].picture.asset_id,'a'.repeat(32));assert.equal(task.result.slides[0].image_search.status,'selected');
  checks.push('Search, authenticated preview, choose, trusted attribution and save reference');
  await page.waitForSelector(scope+' .commons-attribution');await page.$eval(scope+' .commons-attribution',e=>{for(let p=e;p;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;});await openSearch();await (await page.$(scope+' .commons-image-search')).screenshot({path:path.join(out,'commons-editor.png')});
  await click('移除图片',scope);await click('保存本页及其他修改');await page.waitForFunction(()=>!document.querySelector('.unsaved-note'));
  assert.equal(task.result.slides[0].picture,null);assert.equal(task.result.slides[0].image_search.status,'removed');
  checks.push('Remove image persists explicit teacher decision');
  await openSearch();delay=true;await click('检索并审核候选图片',scope);await click('停止等待',scope);await new Promise(resolve=>setTimeout(resolve,1500));
  assert.equal(await page.$$eval(scope+' .commons-candidate',e=>e.length),0);delay=false;
  checks.push('Stopped search cannot apply late candidates');
  await click('检索并审核候选图片',scope);await page.waitForSelector(scope+' .commons-candidate');await click('使用此图',scope);await page.waitForSelector(scope+' .commons-attribution');
  const file=path.join(out,'upload-fixture.png');await fs.writeFile(file,png);
  const input=await page.$(scope+' .evidence-edit-section input[type=file]');await input.uploadFile(file);
  await page.waitForFunction(scope=>!document.querySelector(scope+' .commons-attribution'),{},scope);
  const source=await page.$$eval(scope+' label',els=>els.find(e=>e.textContent.includes('图片出处（必填）')).querySelector('input').value);assert.equal(source,'');
  await page.$$eval(scope+' label',els=>{const e=els.find(e=>e.textContent.includes('图片出处（必填）')).querySelector('input');e.value='教师上传测试文件';e.dispatchEvent(new Event('input',{bubbles:true}));});
  await click('保存本页及其他修改');await page.waitForFunction(()=>!document.querySelector('.unsaved-note'));
  assert.equal(task.result.slides[0].picture.asset_id,'b'.repeat(32));assert.equal(task.result.slides[0].picture.source,'教师上传测试文件');
  checks.push('Upload replacement clears inherited Commons attribution');
  assert.deepEqual(errors,[]);assert.equal(saved,3);assert.equal(searches,3);
  if(web)assert.equal(confirmations,1);
  await fs.writeFile(path.join(out,'browser.json'),JSON.stringify({scope:'Real Vue UI; mocked business APIs',checks,saved,searches,confirmations,errors},null,2));
  console.log(JSON.stringify({checks,saved,searches}));
 }catch(e){if(page){console.error((await page.evaluate(()=>document.body.innerText)).slice(-4500));await page.screenshot({path:path.join(out,'browser-failure.png'),fullPage:true});}throw e;}finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
