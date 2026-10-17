// Fixture APIs only. This check never sends model calls or writes to the backend.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/home-review-redesign');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.plan={pages:Array.from({length:12},()=>({}))};deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const pngs=await Promise.all(deck.slides.map((_,i)=>fs.readFile(path.join(root,'work/courseware-studio/fixture.pptx.slides/slide-'+(i+1)+'.png'))));
 const task={id:'fixture-review',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,created_at:'2026-10-15T02:00:00Z',input:{topic:'浮力与阿基米德原理',grade:'八年级',subject:'物理',pages:12,preview_first:true},result:deck,events:[],sources:[],preview_images_available:true,slide_reviews:deck.slides.map(s=>({id:s.id,hash:'hash-'+s.id,approved:false,vote:0}))};
 const errors=[],writes=[],checks=[];let permissions=['courseware.generate'],empty=false,failBrief=false,failReview=false,liveReview=false;
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9297','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9297/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page);socket=new WebSocket(page.webSocketDebuggerUrl);await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Page.javascriptDialogOpening')call('Page.handleJavaScriptDialog',{accept:true}).catch(e=>errors.push(String(e)));
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname,data=request.postData?JSON.parse(request.postData):{};
  let reply,binary,code=200,contentType='application/json';if(request.method!=='GET')writes.push({path:p,data});
  if(p==='/api/health')reply={status:'ok',mode:liveReview?'live':'sample',courseware_content_review_enabled:!liveReview,courseware_studio_version:'studio-v1',courseware_media_version:'media-v1',teaching_features_version:'teaching-v1'};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'林老师',role:'teacher',permissions};
  else if(p==='/api/tenants')reply=[{id:1,name:'启明教研空间'}];
  else if(p==='/api/billing')reply={balance:100,reserved:20,available:80,plan:'basic',products:[],orders:[],ledger:[]};
  else if(p==='/api/tasks')reply=request.method==='POST'?{id:task.id}:empty?[]:[task];
  else if(['/api/courseware/materials','/api/courseware/references'].includes(p))reply=[];
  else if(p==='/api/tasks/fixture-review')reply=task;
  else if(p==='/api/courseware/brief'){if(failBrief){code=503;reply={message:'需求服务暂不可用，请重试。'}}else reply={reply:'可以用生活情境导入，再通过实验讨论浮力。',mode:'sample',...(data.intent==='generate'?{settings:{topic:'二次函数',grade:data.input.grade,subject:data.input.subject}}:{})}}
  else if(p.endsWith('/conversation'))reply=[];
  else if(p.endsWith('/events')){binary=Buffer.from(': fixture\n\n');contentType='text/event-stream'}
  else if(p.endsWith('/preview')){const i=Number(p.split('/').at(-2).split('-').at(-1));binary=pngs[i%pngs.length];contentType='image/png'}
  else if(p.endsWith('/review')){const id=p.split('/').at(-2);if(failReview){failReview=false;code=503;reply={message:'审核服务暂不可用'}}else{Object.assign(task.slide_reviews.find(s=>s.id===id),{approved:data.approved});reply={ok:true}}}
  else if(p.endsWith('/draft')||p.endsWith('/edit')){task.result=data.result;task.revision++;task.slide_reviews.forEach(s=>s.approved=false);reply={revision:task.revision}}
  else if(p.endsWith('/confirm')){assert.equal(data.accept_review,true);assert.ok(task.slide_reviews.every(s=>s.approved));task.status='succeeded';reply={ok:true}}
  else if(p.endsWith('/expand')){assert.ok(task.slide_reviews.every(s=>s.approved));task.status='awaiting_confirmation';reply={ok:true}}
  else{errors.push('Unexpected request '+request.method+' '+p);code=404;reply={message:'Missing fixture'}}
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:(binary||Buffer.from(JSON.stringify(reply))).toString('base64')}).catch(e=>errors.push(String(e)));
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<80;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate('document.body.innerText.slice(-1000)'))};
 const fill=(sel,value)=>evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(sel)});e.value=${JSON.stringify(value)};e.dispatchEvent(new Event('input',{bubbles:true}))})()`);
 const click=sel=>evaluate(`document.querySelector(${JSON.stringify(sel)}).click()`);
 const shot=async name=>{await evaluate('window.scrollTo(0,0)');await sleep(100);const metrics=await call('Page.getLayoutMetrics');const height=Math.ceil(metrics.cssContentSize.height);await fs.writeFile(path.join(output,name),Buffer.from((await call('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,clip:{x:0,y:0,width:await evaluate('innerWidth'),height,scale:1}})).data,'base64'))};
 await call('Runtime.enable');await call('Page.enable');await call('Emulation.setEmulatedMedia',{features:[{name:'prefers-reduced-motion',value:'reduce'}]});await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token');localStorage.setItem('edu_sidebar_collapsed','0')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:5173'});
 await wait("!!document.querySelector('.conversation-home .brief-composer')&&!!document.querySelector('.home-recent-work')");
 assert.equal(await evaluate("document.querySelector('.brief-composer label').firstChild.textContent.trim()"),'提出你的需求','Demand input uses the requested label');
 assert.ok(await evaluate("document.querySelector('.brief-composer textarea').getBoundingClientRect().bottom<innerHeight"),'Homepage input fits desktop viewport');
 const contrast=await evaluate(`(()=>{const lum=c=>{const v=c.match(/[\\d.]+/g).slice(0,3).map(Number).map(n=>{n/=255;return n<=.04045?n/12.92:((n+.055)/1.055)**2.4});return v[0]*.2126+v[1]*.7152+v[2]*.0722};return [...document.querySelectorAll('.home-welcome p,.home-prompts p,.home-inspiration button,.brief-actions small,.home-recent-work small,.brief-bubble p')].map(e=>{let a=e,b;do{b=getComputedStyle(a).backgroundColor;a=a.parentElement}while(b==='rgba(0, 0, 0, 0)'&&a);const x=lum(getComputedStyle(e).color),y=lum(b);return {text:e.textContent.slice(0,30),ratio:(Math.max(x,y)+.05)/(Math.min(x,y)+.05)}})})()`);
 assert.deepEqual(contrast.filter(r=>r.ratio<4.5),[],'Home body copy passes AA contrast');
 await shot('home-desktop.png');
 await click('.brief-parameters summary');await fill('.brief-topic input','勾股定理的生活应用');await click('.brief-parameters summary');
 assert.equal(writes.length,0,'Editing a topic cannot create a task or reserve credits');
 await fill('.brief-composer textarea','基础偏弱，45分钟，增加生活中的应用');await click('.brief-composer button[type=submit]');await wait("document.querySelectorAll('.brief-bubble.user').length===1&&!document.querySelector('.brief-composer textarea').disabled");
 assert.deepEqual(writes.map(w=>w.path),['/api/courseware/brief']);
 failBrief=true;await fill('.brief-composer textarea','再增加一道练习');await click('.brief-composer button[type=submit]');await wait("document.querySelector('.brief-composer .alert.error')?.textContent.includes('暂不可用')");
 assert.equal(await evaluate("document.querySelector('.brief-composer textarea').value"),'再增加一道练习');failBrief=false;
 checks.push('Homepage conversation, optional settings, inline errors and retained input work without task creation');
 await click('.sidebar nav button[aria-label="所有工具"]');await click('[data-tool="courseware"]');
 await wait("!!document.querySelector('.courseware-generator')");
 assert.equal(await evaluate("!!document.querySelector('.conversation-home')"),false,'Tools opens a dedicated courseware form');
 assert.equal(writes.filter(w=>w.path==='/api/tasks').length,0,'Opening the tool cannot reserve credits');
 assert.equal(await evaluate("document.querySelector('.generator-submit').disabled"),true,'A teaching topic is required');
 await evaluate("(()=>{const e=document.querySelector('[aria-label=\"课件年级\"]');e.value='九年级';e.dispatchEvent(new Event('change',{bubbles:true}));const s=document.querySelector('[aria-label=\"课件学科\"]');s.value='数学';s.dispatchEvent(new Event('change',{bubbles:true}))})()");
 await fill('[aria-label="课件教学主题"]','二次函数的图像与性质，从生活情境开始，安排课堂练习。');
 await fill('[aria-label="课件其他要求"]','至少安排三个课堂问题，答案放在讲解备注中。');
 await shot('generator-desktop.png');
 for(const width of [1024,820,390]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:width===390?844:1000,deviceScaleFactor:1,mobile:width===390});
  assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'Generator has no overflow at '+width);
  if(width===390)await shot('generator-mobile.png');
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 await click('.generator-materials summary');await wait("document.querySelector('.generator-materials').open");
 assert.ok(await evaluate("!!document.querySelector('.generator-materials input[type=file]')"),'Reference upload is reachable');
 await click('.generator-materials summary');
 failBrief=true;await click('.generator-submit');await wait("document.querySelector('.generator-form .alert.error')?.textContent.includes('暂不可用')");
 assert.ok(await evaluate("document.querySelector('[aria-label=\"课件教学主题\"]').value.includes('二次函数')"),'Failed submission retains the topic');
 assert.equal(writes.filter(w=>w.path==='/api/tasks').length,0,'Brief errors do not create tasks');
 failBrief=false;await click('.generator-submit');await wait("document.querySelectorAll('.studio-slide-picture img').length===3");
 const generated=writes.find(w=>w.path==='/api/tasks').data;
 assert.equal(generated.type,'courseware');assert.equal(generated.preview_first,true);assert.equal(generated.grade,'九年级');assert.equal(generated.subject,'数学');
 assert.ok(generated.requirements.includes('至少安排三个课堂问题'),'Other requirements reach the generation request');
 assert.equal(writes.filter(w=>w.path.endsWith('/expand')).length,0,'Creation still requires preview review before expansion');
 checks.push('Tools opens a responsive form; reference upload, retained error input and existing three-page preview flow work');
 await click('.sidebar nav button[aria-label="首页"]');await wait("!!document.querySelector('.home-recent-work')");
 await click('.home-recent-work');await wait("document.querySelectorAll('.studio-slide-picture img').length===3");
 assert.equal(await evaluate("document.querySelectorAll('.studio-page-thumbnail').length"),3,'Review exposes a thumbnail navigator');
 assert.equal(await evaluate("[...document.querySelectorAll('.studio-slide')].filter(e=>e.getBoundingClientRect().height>0).length"),1,'Review shows one selected slide');
 await click('.studio-page-thumbnail:nth-child(3)');
 assert.equal(await evaluate("document.querySelector('.studio-slide.selected').dataset.slideId"),'slide-2');
 assert.equal(await evaluate("document.querySelector('.studio-inspector-title').textContent"),deck.slides[2].title);
 assert.ok(await evaluate("(()=>{const p=document.querySelector('.studio-slide.selected .studio-slide-picture').getBoundingClientRect(),a=document.querySelector('.studio-conversation').getBoundingClientRect();return p.width>a.width&&p.width>480})()"),'PPT preview is the largest work area');
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true,'Expansion requires review');
 await click('.studio-slide.selected .studio-slide-bottom button.approve-page');await wait("document.querySelector('.studio-page-thumbnail:nth-child(3)').textContent.includes('已确认')");
 assert.equal(task.slide_reviews[2].approved,true);assert.equal(task.slide_reviews[0].approved,false);
 await click('.studio-page-thumbnail:first-child');await shot('review-desktop.png');
 await click('.inspector-tabs button:last-child');await wait("document.querySelector('.chat-composer').getBoundingClientRect().height>0");
 await call('Emulation.setDeviceMetricsOverride',{width:1366,height:768,deviceScaleFactor:1,mobile:false});
 await wait("document.querySelector('.chat-composer').getBoundingClientRect().bottom<=innerHeight+1");
 assert.ok(await evaluate("document.querySelector('.chat-composer').getBoundingClientRect().bottom<=innerHeight+1"),'Dialogue composer remains reachable on short desktop '+await evaluate("JSON.stringify({y:scrollY,h:innerHeight,composer:document.querySelector('.chat-composer').getBoundingClientRect().toJSON(),studio:document.querySelector('.courseware-studio').getBoundingClientRect().toJSON(),body:document.querySelector('.studio-body').getBoundingClientRect().toJSON()})"));
 await click('.inspector-tabs button:first-child');
 checks.push('Thumbnail selection synchronizes the large preview and inspector; page confirmation affects only selected page; final actions remain gated');
 for(const width of [1440,1024,820,390]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:width===390?844:1000,deviceScaleFactor:1,mobile:width===390});
  assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'Review has no overflow at '+width);
  if(width===390){await shot('review-mobile.png');
   await evaluate("document.querySelector('.inspector-edit-actions').scrollIntoView({block:'center'})");
   await click('.inspector-edit-actions button:first-child');
   await wait("(()=>{const e=document.querySelector('.studio-slide.selected .studio-slide-editor input');if(!e)return false;const r=e.getBoundingClientRect();return document.activeElement===e&&r.top>=0&&r.bottom<=innerHeight})()");
   checks.push('Mobile inspector edit scrolls back to the opened editor and focuses the title field');
   await click('.studio-slide.selected .slide-tools button[aria-label="编辑第1页"]');
  }
 }
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 failReview=true;await click('.studio-approve-all');await wait("document.querySelector('.studio-canvas>.alert.error')?.textContent.includes('未全部完成')");
 assert.equal(task.slide_reviews.filter(s=>s.approved).length,1,'Failed bulk review preserves previous confirmations');
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true);
 await click('.studio-approve-all');await wait("document.querySelector('.studio-approve-all').textContent==='全部已审核'");
 assert.ok(task.slide_reviews.every(s=>s.approved));assert.equal(task.status,'awaiting_preview','Bulk review cannot trigger expansion');
 await click('.studio-completion .primary');await wait("!!document.querySelector('.studio-completion .consent')");
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true,'Delivery requires explicit fee consent');
 await click('.studio-completion .consent input');await click('.studio-completion .primary');await wait("document.querySelector('.studio-header p').textContent.includes('课件已完成')");
 assert.equal(task.status,'succeeded');
 await click('.studio-page-thumbnail:first-child');await click('.inspector-edit-actions button:first-child');
 await fill('.studio-slide.selected .studio-slide-editor input','更新后的课堂导入');
 await evaluate("document.querySelector('.studio-slide.selected .studio-slide-editor>button.primary').click()");await wait("!document.querySelector('.unsaved-note')");
 assert.equal(task.result.slides[0].title,'更新后的课堂导入');assert.ok(task.slide_reviews.every(s=>!s.approved),'Saved changes need fresh confirmation');
 await click('.studio-deck-settings summary');await wait("document.querySelector('.studio-deck-settings').open");
 assert.ok(await evaluate("document.querySelectorAll('.studio-deck-settings .theme').length>=6"));
 checks.push('Bulk review recovers failures; expansion and fee consent remain separate; content save invalidates approvals; templates remain accessible');
 await click('.studio-deck-settings summary');
 await evaluate("document.querySelector('.studio-slide.selected [aria-label=\"在第1页后添加\"]').click()");
 await wait("document.querySelectorAll('.studio-page-thumbnail').length===4");
 const removed=await evaluate("document.querySelector('.studio-slide.selected').dataset.slideId");
 await evaluate("document.querySelector('.studio-slide.selected [aria-label=\"删除第2页\"]').click()");
 await wait("document.querySelectorAll('.studio-page-thumbnail').length===3");
 assert.notEqual(await evaluate("document.querySelector('.studio-slide.selected')?.dataset.slideId"),removed);
 assert.equal(await evaluate("[...document.querySelectorAll('.studio-slide')].filter(e=>e.getBoundingClientRect().height>0).length"),1);
 assert.equal(await evaluate("document.querySelectorAll('.studio-slide-editor').length"),0);
 checks.push('Deleting the current page selects its surviving neighbor and clears removed editor state');

 // Reproduce the reported long deck with a live-mode warning and a collapsed sidebar.
 liveReview=true;task.mode='live';task.status='awaiting_confirmation';
 task.result.slides=Array.from({length:14},(_,i)=>({...structuredClone(deck.slides[i%3]),id:'slide-'+i}));
 task.result.notice='教学与视觉规范已应用；请逐页核对事实、例题答案和引用。已完成质控检查点保留，自动修复仅替换问题页。内容模型审核已停用；保留程序校验与截图审核，请教师核对知识、题设、计算和答案。';
 task.result.page_checks=Object.fromEntries(task.result.slides.map(s=>[s.id,{content:'disabled',visual:'checked'}]));
 task.slide_reviews=task.result.slides.map(s=>({id:s.id,hash:'hash-'+s.id,approved:false,vote:0}));
 await evaluate("localStorage.setItem('edu_sidebar_collapsed','1')");await call('Page.reload');
 await wait("!!document.querySelector('.home-recent-work')");
 assert.ok(await evaluate("document.querySelector('.home-dialogue').getBoundingClientRect().top<=200"),'Live warning and homepage heading leave a compact top area');
 await shot('home-live-compact.png');
 await click('.sidebar-toggle');await wait("!!document.querySelector('.sidebar-collapsed')");
 await call('Emulation.setDeviceMetricsOverride',{width:1366,height:768,deviceScaleFactor:1,mobile:false});await sleep(150);
 assert.ok(await evaluate("document.querySelector('.brief-actions button').getBoundingClientRect().bottom<=innerHeight"),'Live homepage keeps the demand input and send action in the short desktop viewport');
 await shot('home-live-short.png');
 await click('.home-recent-work');
 await wait("document.querySelectorAll('.studio-page-thumbnail').length===14");
 for(const [width,height] of [[1920,1080],[1366,768]]){
  await call('Emulation.setDeviceMetricsOverride',{width,height,deviceScaleFactor:1,mobile:false});
  await wait(`innerWidth===${width}&&innerHeight===${height}`);await sleep(150);
  await shot('review-compact-'+width+'.png');
  assert.ok(await evaluate("getComputedStyle(document.querySelector('.studio-canvas')).overflowY==='visible'"),'Middle pane uses document scrolling rather than a nested scrollbar');
  assert.ok(await evaluate("(()=>{const p=document.querySelector('.studio-slide.selected .studio-slide-picture').getBoundingClientRect(),f=document.querySelector('.studio-completion').getBoundingClientRect();return p.top>=0&&p.bottom<=innerHeight&&p.bottom<=f.top})()"),'Complete PPT fits the viewport without the delivery bar covering it');
  assert.ok(await evaluate("document.querySelector('.studio-header').getBoundingClientRect().bottom<=160"),'Top chrome and warning are compact');
  assert.ok(await evaluate("(()=>{const a=[...document.querySelectorAll('.studio-page-thumbnail')].map(e=>e.getBoundingClientRect());return a.every((r,i)=>r.height>50&&(!i||r.top>=a[i-1].bottom+8))})()"),'Fourteen thumbnails keep their natural height without overlapping');
  assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'No horizontal page overflow');
  assert.ok(await evaluate("document.querySelector('.studio-completion').getBoundingClientRect().bottom<=innerHeight+1"),'Review and fee consent fit the desktop viewport');
 }
 await click('.studio-review-panel .studio-lesson:last-of-type summary');
 assert.ok(await evaluate("document.querySelector('.studio-review-panel .studio-lesson:last-of-type').open"),'Lesson and sources remain available in inspector');
 await click('.studio-review-panel .studio-lesson:last-of-type summary');
 await click('.inspector-edit-actions button:first-child');
 await wait("(()=>{const e=document.querySelector('.studio-slide.selected .studio-slide-editor input'),r=e?.getBoundingClientRect();return e&&document.activeElement===e&&r.top>=44&&r.bottom<=innerHeight})()");
 await click('.studio-slide.selected .slide-tools button[aria-label="编辑第1页"]');
 checks.push('Long live deck: compact header, full PPT, no central scrollbar, non-overlapping thumbnails and reachable desktop editor');

 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await click('button[aria-label="首页"]');await wait("!!document.querySelector('.conversation-home')");
 await shot('home-mobile.png');
 assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'));
 permissions=[];empty=true;await call('Page.reload');await wait("!!document.querySelector('.home-recent-empty')");
 assert.ok(await evaluate("document.querySelector('.brief-direct').disabled"));
 checks.push('Responsive layouts at four widths, empty works and permission-disabled creation remain usable');
 empty=false;task.status='running';task.result=null;task.slide_reviews=[];await call('Page.reload');
 await wait("!!document.querySelector('.home-recent-work')");await click('.home-recent-work');await wait("!!document.querySelector('.studio-progress')");
 assert.equal(await evaluate("document.querySelectorAll('.studio-lesson').length"),0,'Generating deck has no editor before slide data exists');
 checks.push('Generation progress remains available before slide data exists');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({checks,writes:writes.map(w=>w.path),errors},null,2));
 console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
