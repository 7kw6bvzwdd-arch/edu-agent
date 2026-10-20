// Production read-only check: login creates a session; all business requests are GET.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict'),crypto=require('node:crypto');
const puppeteer=require('../animation-video/node_modules/puppeteer-core');
(async()=>{
 const root=path.resolve(__dirname,'..'),out=path.join(root,'work/workspace-history-20261020');await fs.mkdir(out,{recursive:true});
 const settings=Object.fromEntries((await fs.readFile(path.join(root,'.env'),'utf8')).replace(/^\uFEFF/,'').split(/\r?\n/).filter(l=>l.includes('=')&&!l.trimStart().startsWith('#')).map(l=>[l.slice(0,l.indexOf('=')),l.slice(l.indexOf('=')+1)]));
 const base=process.env.TEST_APP_URL||'http://127.0.0.1:8080',active=JSON.parse(await fs.readFile(path.join(root,'work/active-release.json'),'utf8'));
 const health=await fetch(base+'/api/health').then(r=>r.json());assert.equal(health.release_commit,active.commit);assert.equal(health.status,'ok');assert.equal(health.workspace_history_version,'history-v1');
 const manifest=JSON.parse(await fs.readFile(path.join(active.release,'manifest.json'),'utf8'));let bundles=0;
 for(const[file,sha]of Object.entries(manifest.files)){if(!file.startsWith('frontend-vue/dist/'))continue;const r=await fetch(base+'/'+file.slice('frontend-vue/dist/'.length));assert.equal(r.status,200);assert.equal(crypto.createHash('sha256').update(Buffer.from(await r.arrayBuffer())).digest('hex'),sha);bundles++;}
 const login=await fetch(base+'/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'teacher',password:settings.SEED_PASSWORD})});assert.equal(login.status,200);const{token}=await login.json();
 const headers={Authorization:'Bearer '+token},get=async p=>{const r=await fetch(base+'/api'+p,{headers});assert.equal(r.status,200);return r.json();};
 const beforeBilling=await get('/billing'),beforeTasks=await get('/tasks?view=history'),beforeHistory=await get('/workspace/conversations');
 const browser=await puppeteer.launch({executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',headless:true,args:['--disable-background-networking'],defaultViewport:{width:1440,height:1000}});
 const errors=[],writes=[];
 try{
  const page=await browser.newPage();page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(r.url().startsWith(base+'/api/')&&r.method()!=='GET')writes.push(r.method()+' '+new URL(r.url()).pathname);});
  await page.evaluateOnNewDocument(token=>sessionStorage.setItem('edu_token',token),token);
  await page.goto(base+'/#home');await page.waitForSelector('[data-history-action=panel]');
  const checks={release_identity:true,history_capability:true,frontend_hashes:bundles>=3};
  checks.new_conversation=await page.$eval('[data-history-action=new]',e=>!e.disabled);
  checks.clear_control=await page.$eval('[data-history-action=clear]',e=>e.textContent.includes('清空当前对话'));
  await page.click('[data-history-action=panel]');await page.waitForSelector('.workspace-history-panel');
  await page.waitForFunction(()=>!document.querySelector('[data-history-action=panel]').disabled);
  checks.server_list=await page.$$eval('.workspace-history-item',es=>es.length)===beforeHistory.length;
  checks.no_error=await page.$$eval('.workspace-history-error',es=>es.length===0);
  await page.screenshot({path:path.join(out,'production-home-desktop.png'),fullPage:true});
  await page.setViewport({width:390,height:844});checks.mobile_no_overflow=await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth);
  await page.screenshot({path:path.join(out,'production-home-mobile.png'),fullPage:true});
  checks.no_business_writes=writes.length===0;checks.no_runtime_errors=errors.length===0;
  checks.history_preserved=JSON.stringify(beforeHistory)===JSON.stringify(await get('/workspace/conversations'));
  checks.billing_preserved=JSON.stringify(beforeBilling)===JSON.stringify(await get('/billing'));
  checks.tasks_preserved=JSON.stringify(beforeTasks)===JSON.stringify(await get('/tasks?view=history'));
  const result={release:active.commit,checks,errors,writes};await fs.writeFile(path.join(out,'production-readonly.json'),JSON.stringify(result,null,2));console.log(JSON.stringify(result));assert.ok(Object.values(checks).every(Boolean));
 }finally{await browser.close();}
})().catch(e=>{console.error(e.name+': '+e.message);process.exitCode=1;});
