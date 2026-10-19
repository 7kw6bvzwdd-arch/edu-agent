// Fixed author data and mocked task APIs; scientific previews use the real renderer.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const {pathToFileURL} = require('node:url');
const puppeteer = require('../animation-video/node_modules/puppeteer-core');
(async () => {
  const root = path.resolve(__dirname, '..'), out = path.join(root, 'work/science-visuals');
  const browser = await puppeteer.launch({executablePath:process.env.ANIMATION_CHROME_PATH || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--disable-background-networking'],defaultViewport:{width:1440,height:1080}});
  const errors=[],checks=[];let page;
  try {
    page=await browser.newPage();page.on('pageerror',e=>errors.push(e.message));
    await page.goto(pathToFileURL(path.join(out,'science.html')).href);
    await page.waitForFunction(()=>!!window.__anim);
    const spans=await page.evaluate(()=>JSON.parse(document.getElementById('anim-data').textContent).timeline.scenes);
    for(const [i,kind] of ['thin_lens','molecule','cell','scene'].entries()) {
      const a=spans[i].actions.find(a=>a.type==='tween');assert.ok(a);
      const snapshot=t=>page.evaluate(t=>{__anim.seek(t);const o=document.querySelector('[data-scientific]');return {kind:o.dataset.scientific,index:o.dataset.frame,image:o.getAttribute('href')};},t);
      const start=await snapshot(a.start),end=await snapshot(a.end);
      assert.equal(start.kind,kind);assert.equal(start.index,'0');assert.equal(end.index,'25');assert.notEqual(start.image,end.image);
      assert.deepEqual(await snapshot(a.start),start,'backward seek restores exact scientific image');
      await page.screenshot({path:path.join(out,kind+'-start.png')});
      await snapshot(a.end);await page.screenshot({path:path.join(out,kind+'-end.png')});
    }
    checks.push('Four scientific components change frames and restore exact state on backward seek');
    const script=JSON.parse(await fs.readFile(path.join(root,'fixtures/animation-v2-science.json'),'utf8'));
    const diagram=script.scenes[3].objects[0].scientific;
    const task={id:'science-fixture',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,input:{topic:'图形动画样例',pages:8,theme:'ocean',preview_first:true},events:[],sources:[],preview_images_available:false,slide_reviews:[],result:{title:'图示编辑样例',theme:'ocean',lesson:'固定样例',slides:[{id:'slide-1',title:'图形组合与动作',layout:'concept',bullets:['观察并解释位置变化。'],scientific:diagram}]}};
    let saved=0,previews=0;
    const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64');
    await page.setRequestInterception(true);
    page.on('request',async r=>{
      if(!r.url().includes('/api/'))return r.continue();
      const p=new URL(r.url()).pathname;let data=[],contentType='application/json',body,status=200;
      try {
        if(p==='/api/courseware/scientific') {
          const res=await fetch((process.env.SCIENCE_RENDER_URL||'http://127.0.0.1:18001')+'/render/scientific',{method:'POST',headers:{'Content-Type':'application/json'},body:r.postData()});
          status=res.status;data=await res.json();previews++;
        } else if(p.endsWith('/edit')||p.endsWith('/draft')) { task.result=JSON.parse(r.postData()).result;task.revision++;saved++;data={ok:true}; }
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
      } catch(e) { errors.push(e.message);await r.respond({status:500,contentType:'application/json',body:'{"message":"Test request failed"}'}); }
    });
    await page.evaluateOnNewDocument(()=>sessionStorage.setItem('edu_token','fixture-token'));
    await page.goto(process.env.TEST_APP_URL||'http://127.0.0.1:15173');
    const click=async(text,scope='body')=>page.evaluate(({text,scope})=>{const b=[...document.querySelector(scope).querySelectorAll('button')].find(b=>b.textContent.trim().includes(text));if(!b||b.disabled)throw Error('Missing button: '+text);b.click();},{text,scope});
    await page.waitForFunction(()=>[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品')));
    await click('我的作品');await page.waitForSelector('.task-open');await page.click('.task-open');
    await page.waitForFunction(()=>document.body.innerText.includes('手动编辑内容'));await click('手动编辑内容');
    await page.waitForSelector('.scene-editor');
    await page.waitForFunction(()=>document.querySelector('.scientific-editor img[data-science-kind="scene"]')?.src.startsWith('data:image/gif'));
    await page.evaluate(()=>{document.querySelector('.scene-object').open=true;});
    const before=await page.$eval('.scene-editor',e=>e.querySelectorAll('.scene-object').length);
    await click('＋ 矩形','.scene-editor');
    assert.equal(await page.$$eval('.scene-object',e=>e.length),before+1);
    await page.evaluate(()=>{const d=[...document.querySelectorAll('.scene-object')].at(-1);d.open=true;const x=[...d.querySelectorAll('label')].find(l=>l.textContent.includes('中心 x')).querySelector('input');x.value='.6';x.dispatchEvent(new Event('input',{bubbles:true}));});
    const count=await page.$$eval('.scene-action',e=>e.length);await click('＋ 动画动作','.scene-editor');assert.equal(await page.$$eval('.scene-action',e=>e.length),count+1);
    await page.evaluate(()=>{document.querySelectorAll('.scene-action').forEach(e=>e.open=true)});
    // New emphasize targets the first route and can safely coexist with following it.
    await page.waitForFunction(()=>document.querySelector('.scientific-editor img[data-science-kind="scene"]')?.src.startsWith('data:image/gif'));
    await click('保存本页及其他修改');await page.waitForFunction(()=>!document.querySelector('.unsaved-note'));
    assert.equal(saved,1);assert.equal(task.result.slides[0].scientific.scene.objects.length,before+1);
    assert.equal(task.result.slides[0].scientific.scene.objects.at(-1).x,.6);
    checks.push('Actual Vue scene editor adds shapes/actions, edits coordinates, previews real GIF and saves author data');
    await page.screenshot({path:path.join(out,'editor-scene.png'),fullPage:true});
    await click('移除专业图示','.scientific-editor');
    for(const [label,kind] of [['＋ 凸透镜成像','thin_lens'],['＋ 分子结构','molecule'],['＋ 细胞结构','cell']]) {
      await click(label,'.scientific-editor');
      await page.waitForFunction(kind=>!!document.querySelector('.scientific-editor img[data-science-kind="'+kind+'"]'),{},kind);
      await page.evaluate(()=>{const l=[...document.querySelectorAll('.scientific-editor label')].find(l=>l.textContent.includes('导出动态演示'));l.querySelector('input').click()});
      await page.waitForFunction(kind=>document.querySelector('.scientific-editor img[data-science-kind="'+kind+'"]')?.src.startsWith('data:image/gif'),{},kind);
      await click('移除专业图示','.scientific-editor');
    }
    checks.push('Lens, molecule and cell share editor entry and real animated preview');
    assert.deepEqual(errors,[]);assert.ok(previews>=8);
    await fs.writeFile(path.join(out,'browser.json'),JSON.stringify({checks,previews,errors},null,2));
    console.log('PASS: '+checks.join('; '));
  } catch(e) {
    if(page) { console.error((await page.evaluate(()=>document.body.innerText)).slice(-5000),errors);await page.screenshot({path:path.join(out,'browser-failure.png'),fullPage:true}); }
    throw e;
  } finally { await browser.close(); }
})().catch(e=>{console.error(e);process.exitCode=1;});
