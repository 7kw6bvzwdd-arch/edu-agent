// Fixture business responses: no production task, review, model or billing writes.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const puppeteer = require('../animation-video/node_modules/puppeteer-core');

(async () => {
  const issue = '第1页未完成：图解仅支持flow/timeline/route/interval，2–4个节点，标题最多60字';
  const slide = {id:'paused-slide', title:'一条时间轴看近代史主线', bullets:['五个主干事件'], notes:'测试草稿'};
  const task = {id:'paused-review-fixture', type:'courseware', status:'failed', mode:'live', revision:43,
    input:{topic:'近代史',preview_first:true}, error:'制作暂未完成，已有页面已保存，预留已释放。',
    result:{title:'近代史',theme:'ocean',slides:[slide],generation:{issue,repair_page:1,phase:'preview'}},
    slide_reviews:[{id:slide.id,hash:'fixture-hash',approved:false,vote:0}],events:[],sources:[]};
  const writes = [], errors = [];
  const browser = await puppeteer.launch({executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
    headless:true,args:['--disable-background-networking'],defaultViewport:{width:1600,height:1100}});
  try {
    const page = await browser.newPage();
    page.on('pageerror',e=>errors.push(e.message));
    await page.setRequestInterception(true);
    page.on('request',r=>{
      const p=new URL(r.url()).pathname;
      if(!p.startsWith('/api/'))return r.continue();
      if(r.method()!=='GET')writes.push(r.method()+' '+p);
      let body;
      if(p==='/api/health')body={status:'ok',mode:'live',courseware_studio_version:'studio-v1',courseware_media_version:'media-v1'};
      else if(p==='/api/auth/me')body={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate']};
      else if(p==='/api/tenants')body=[{id:1,name:'测试空间'}];
      else if(p==='/api/billing')body={balance:100,reserved:0,available:100,plan:'basic',ledger:[]};
      else if(p==='/api/tasks')body=[task];
      else if(p==='/api/tasks/'+task.id)body=task;
      else body=[];
      r.respond({status:200,contentType:'application/json',body:JSON.stringify(body)}).catch(()=>{});
    });
    await page.evaluateOnNewDocument(()=>sessionStorage.setItem('edu_token','fixture-token'));
    await page.goto((process.env.TEST_FRONTEND_URL||'http://127.0.0.1:8080')+'/#courseware?task='+task.id);
    await page.waitForSelector('.studio-slide.selected');
    const controls=await page.$$eval('button',els=>els.filter(e=>e.textContent.includes('本页审核通过')).map(e=>({disabled:e.disabled,title:e.title,background:getComputedStyle(e).backgroundColor})));
    assert.equal(controls.length,2);
    assert.ok(controls.every(c=>c.disabled),'both review entries must remain blocked for failed draft');
    assert.ok(controls.every(c=>c.title.includes('继续制作')),'both entries explain how to resume before approval');
    const reason=await page.$eval('.inspector-review-foot',e=>e.textContent);
    assert.ok(reason.includes(issue),'actual checkpoint failure must appear beside disabled approval');
    assert.equal(controls[0].background,controls[1].background,'disabled controls have consistent styling');
    assert.equal(writes.length,0);assert.deepEqual(errors,[]);
    const out=path.join(__dirname,'../work/paused-review-20261020');await fs.mkdir(out,{recursive:true});
    await page.screenshot({path:path.join(out,'paused-review.png'),fullPage:true});
    console.log(JSON.stringify({checks:6,passed:true,writes,errors}));
  } finally {await browser.close()}
})().catch(e=>{console.error(e.message);process.exitCode=1});
