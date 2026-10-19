const createCDP = require('./browser-cdp.cjs');
// Intercept every API request: no production account, task or model writes.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/sidebar-layout/browser');await fs.mkdir(output,{recursive:true});
 let actorRole='teacher';const errors=[];
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9283','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9283/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});

 const { call, evaluate } = createCDP(socket, m => {
  if(m.method==='Page.javascriptDialogOpening')call('Page.handleJavaScriptDialog',{accept:true}).catch(e=>errors.push(String(e)));
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname;
  let reply,code=200;
  if(request.method!=='GET'){errors.push('Unexpected write: '+p);code=400;reply={message:'No writes allowed'}}
  else if(p==='/api/health')reply={status:'ok',mode:'sample',courseware_studio_version:'studio-v1',courseware_media_version:'media-v1',teaching_features_version:'teaching-v1'};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'测试教师',role:actorRole,permissions:['courseware.generate']};
  else if(p==='/api/tenants')reply=[{id:1,name:'浏览器样例空间'}];
  else if(p==='/api/billing')reply={balance:100,reserved:0,available:100,plan:'basic',ledger:[],products:[],orders:[]};
  else if(p==='/api/account')reply={username:'fixture',name:'测试教师'};
  else if(p==='/api/dashboard')reply={totals:{calls:0,input_tokens:0,output_tokens:0,failures:0},p95_ms:0};
  else if(p==='/api/usage')reply={items:[],total:0};
  else if(['/api/documents','/api/admin/members'].includes(p))reply=[];
  else if(['/api/tasks','/api/courseware/materials','/api/courseware/references'].includes(p))reply=[];
  else{errors.push('Unexpected API: '+p);code=404;reply={}}
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:'application/json'}],body:Buffer.from(JSON.stringify(reply)).toString('base64')}).catch(e=>errors.push(String(e)));
 });
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate('document.body.innerText.slice(-2000)')+'\n'+JSON.stringify(errors))};
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});
 await wait("[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品'))");

 await evaluate("(()=>{if(document.querySelector('.sidebar-collapsed'))document.querySelector('.sidebar-toggle').click()})()");await sleep(350);
 const expanded=await evaluate("document.querySelector('.sidebar').getBoundingClientRect().width");
 await evaluate("document.querySelector('.sidebar-toggle').click()");await sleep(80);
 const moving=await evaluate("(()=>{const s=document.querySelector('.sidebar').getBoundingClientRect(),m=document.querySelector('main').getBoundingClientRect();return {width:s.width,right:s.right,main:m.left,overflow:document.documentElement.scrollWidth>innerWidth}})()");
 assert.ok(moving.width>77&&moving.width<expanded-1,'Sidebar has an intermediate width during collapse: '+JSON.stringify(moving));
 assert.ok(Math.abs(moving.right-moving.main)<=1&&!moving.overflow,'Content follows the animated sidebar without overlap');
 await evaluate("document.querySelector('.sidebar-toggle').click()");await sleep(350);
 assert.ok(Math.abs(await evaluate("document.querySelector('.sidebar').getBoundingClientRect().width")-expanded)<1,'Rapid reversal returns to expanded geometry');
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});
 await evaluate("document.querySelector('.sidebar-toggle').click()");await sleep(50);
 assert.equal(await evaluate("Math.round(document.querySelector('.sidebar').getBoundingClientRect().width)"),76,'Reduced motion applies the collapsed state immediately');
 await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'no-preference'}]});

 const cases=[];
 for(const [width,height,zoom] of [[1440,1080,1],[1440,500,1],[1100,650,1.25],[1100,450,1.5],[900,500,1],[820,700,1],[390,844,1]]){
  await call('Emulation.setDeviceMetricsOverride',{width:Math.round(width/zoom),height:Math.round(height/zoom),deviceScaleFactor:zoom,mobile:false});
  for(const collapsed of [false,true]){
   await evaluate(`(()=>{if(document.querySelector('.app-shell').classList.contains('sidebar-collapsed')!==${collapsed})document.querySelector('.sidebar-toggle').click();window.scrollTo(0,0)})()`);
   await sleep(350);
   const geometry=await evaluate(`(()=>{const box=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,right:r.right,bottom:r.bottom,width:r.width,height:r.height}};const visible=e=>e&&getComputedStyle(e).display!=='none'&&e.getBoundingClientRect().height>0;const nodes=[...document.querySelectorAll('.sidebar-toggle,.sidebar .brand-mark,.sidebar .brand>div,.sidebar nav button,.sidebar .profile-open,.sidebar .profile>button:last-child,.mobile-account')].filter(visible);return {boxes:nodes.map(e=>({name:e.getAttribute('aria-label')||e.textContent.trim(),...box(e),clickable:e.tagName==='BUTTON',hit:e.tagName==='BUTTON'?e.contains(document.elementFromPoint(e.getBoundingClientRect().x+e.getBoundingClientRect().width/2,e.getBoundingClientRect().y+e.getBoundingClientRect().height/2)):true})),brand:visible(document.querySelector('.sidebar .brand-mark'))?box(document.querySelector('.sidebar .brand-mark')):null,nav:box(document.querySelector('.sidebar nav button')),innerWidth,innerHeight,scrollWidth:document.documentElement.scrollWidth}})()`);
   const context=JSON.stringify({width,height,zoom,collapsed,geometry});
   if(geometry.brand)assert.ok(geometry.nav.y-geometry.brand.bottom>=8,context);
   for(let i=0;i<geometry.boxes.length;i++){
    const a=geometry.boxes[i];
    assert.ok(a.hit,`Click area is obscured: ${a.name}: ${context}`);
    for(const b of geometry.boxes.slice(i+1))assert.ok(Math.min(a.right,b.right)-Math.max(a.x,b.x)<=1||Math.min(a.bottom,b.bottom)-Math.max(a.y,b.y)<=1,`Overlap: ${a.name} / ${b.name}: ${context}`);
   }
   assert.ok(geometry.scrollWidth<=geometry.innerWidth+1,`Horizontal overflow: ${context}`);
   cases.push({width,height,zoom,collapsed,geometry});
   if(width===1440&&height===1080&&collapsed)await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'sidebar-collapsed.png'),Buffer.from(r.data,'base64')));
  }
 }

 actorRole='admin';await call('Page.reload');await wait("!!document.querySelector('.sidebar nav button[aria-label=\"企业管理\"]')");
 for(const collapsed of [false,true]){
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:400,deviceScaleFactor:1,mobile:false});
  await evaluate(`(()=>{if(document.querySelector('.app-shell').classList.contains('sidebar-collapsed')!==${collapsed})document.querySelector('.sidebar-toggle').click();window.scrollTo(0,0)})()`);
  await sleep(350);
  for(const label of ['首页','所有工具','我的作品','知识与来源','企业管理']){
   await evaluate(`document.querySelector('.sidebar nav button[aria-label="${label}"]').scrollIntoView({block:'nearest'})`);
   assert.ok(await evaluate(`(()=>{const e=document.querySelector('.sidebar nav button[aria-label="${label}"]'),r=e.getBoundingClientRect();return e.contains(document.elementFromPoint(r.x+r.width/2,r.y+r.height/2))})()`),`Short admin sidebar cannot reach ${label}`);
  }
  await evaluate("window.scrollTo(0,document.body.scrollHeight)");
  assert.ok(await evaluate("(()=>{const e=document.querySelector('.sidebar .profile-open'),r=e.getBoundingClientRect();return r.y>=0&&r.bottom<=innerHeight&&e.contains(document.elementFromPoint(r.x+r.width/2,r.y+r.height/2))})()"),'Account clipped after page scroll');
  const account=await evaluate("(()=>{const e=document.querySelector('.sidebar .profile-open'),r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
  await call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...account});
  await call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...account});
  await wait("!!document.querySelector('.account-navigation')");
  await evaluate("document.querySelector('.sidebar nav button[aria-label=\"首页\"]').click()");
  cases.push({width:1440,height:400,collapsed,role:'admin',checks:'all five navigation entries reachable via native scrolling, account clickable after page scroll'});
 }
 const patterns=[];
 for(const [page,selector] of [
  ['tools','.sidebar nav [aria-label="所有工具"]'],
  ['tasks','.sidebar nav [aria-label="我的作品"]'],
  ['knowledge','.sidebar nav [aria-label="知识与来源"]'],
  ['account','.profile-open'],
  ['billing','.account-navigation button:nth-child(2)'],
  ['dashboard','.account-navigation button:nth-child(3)'],
  ['admin','.sidebar nav [aria-label="企业管理"]']
 ]){
  await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  await evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
  await wait(`document.querySelector('.app-shell')?.dataset.page===${JSON.stringify(page)}`);await sleep(150);await evaluate('window.scrollTo(0,0)');
  patterns.push(await evaluate("(()=>{const s=getComputedStyle(document.querySelector('.app-shell>main'));return {page:document.querySelector('.app-shell').dataset.page,position:s.backgroundPosition,size:s.backgroundSize}})()"));
  await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'pattern-'+page+'.png'),Buffer.from(r.data,'base64')));
 }
 assert.equal(new Set(patterns.map(p=>p.position)).size,patterns.length,'Each workspace uses a different pattern placement');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});await sleep(150);
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'pattern-mobile.png'),Buffer.from(r.data,'base64')));
 await fs.writeFile(path.join(output,'pattern-checks.json'),JSON.stringify(patterns,null,2));
 assert.deepEqual(errors,[]);
 await fs.writeFile(path.join(output,'sidebar-layout-checks.json'),JSON.stringify({cases,errors},null,2));
 console.log(JSON.stringify({passed:cases.length,checks:'brand/nav gap, disjoint click areas, visible account, no horizontal clipping'}));
 await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
