// Fixture APIs only. This check never sends model calls or writes to the backend.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/courseware-review-20261016');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.plan={pages:Array.from({length:12},()=>({}))};deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const pngs=await Promise.all(deck.slides.map((_,i)=>fs.readFile(path.join(root,'work/courseware-studio/fixture.pptx.slides/slide-'+(i+1)+'.png'))));
 const task={id:'fixture-review',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,created_at:'2026-10-15T02:00:00Z',input:{topic:'浮力与阿基米德原理',grade:'八年级',subject:'物理',pages:12,preview_first:true},result:deck,events:[],sources:[],preview_images_available:true,slide_reviews:deck.slides.map(s=>({id:s.id,hash:'hash-'+s.id,approved:false,vote:0}))};
 const errors=[],writes=[],checks=[];let discussion=[],failChat=false;let permissions=['courseware.generate'],empty=false,failBrief=false,failReview=false,liveReview=false;
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9303','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9303/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
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
  else if(p.endsWith('/conversation')){if(request.method==='POST'){if(failChat){failChat=false;code=503;reply={message:'调整服务暂不可用，请重试'}}else{discussion.push({id:'m'+discussion.length,...data,status:'completed',reply:'已收到当前页调整要求。'});reply={id:'m'+discussion.length}}}else reply=discussion;}
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

 await wait("!!document.querySelector('.home-recent-work')");await click('.home-recent-work');await wait("document.querySelectorAll('.studio-slide-picture img').length===3");
 assert.equal(await evaluate("document.querySelector('.studio-adjust-heading h2').textContent"),'审核与调整');
 assert.equal(await evaluate("document.querySelectorAll('.inspector-tabs').length"),0,'Review and adjustment share one surface');
 assert.equal(await evaluate("document.querySelectorAll('.studio-phase-list li').length"),4);
 assert.ok(await evaluate("!!document.querySelector('.studio-conversation .studio-completion')"),'Delivery actions live in review card');
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true);
 await shot('review-desktop.png');
 checks.push('Four phases, one review-and-adjust card and separate gated delivery action');
 await click('.studio-page-thumbnail:nth-child(3)');
 assert.equal(await evaluate("document.querySelector('.studio-inspector-title').textContent"),deck.slides[2].title);
 assert.equal(await evaluate("document.querySelector('.studio-adjust-heading').dataset.currentSlideId"),'slide-2');
 assert.equal(await evaluate("document.querySelector('.studio-canvas').dataset.currentSlideId"),'slide-2');
 assert.equal(await evaluate("document.querySelector('.studio-slide.selected').dataset.slideId"),'slide-2');
 assert.equal(await evaluate("document.querySelector('.studio-page-thumbnail[aria-current=page]').getAttribute('aria-label')"),'跳转第3页：'+deck.slides[2].title);
 await fill('.chat-composer textarea','把本页改成实验探究，保留完整答案');failChat=true;await click('.chat-send-row button');
 await wait("document.querySelector('.studio-canvas>.alert.error')?.textContent.includes('暂不可用')");
 assert.equal(await evaluate("document.querySelector('.chat-composer textarea').value"),'把本页改成实验探究，保留完整答案');
 await click('.chat-send-row button');await wait("document.querySelector('.chat-composer textarea').value===''");
 const revision=writes.filter(w=>w.path.endsWith('/conversation')).at(-1).data;
 assert.equal(revision.mode,'revise');assert.equal(revision.slide_id,'slide-2');assert.equal(revision.revision,task.revision);
 assert.ok(await evaluate("document.querySelector('.studio-discussion-history').open"),'Reply becomes visible after submitting');
 await click('.chat-mode button:first-child');await fill('.chat-composer textarea','这页适合八年级学生吗');await click('.chat-send-row button');await wait("document.querySelectorAll('.conversation-log .chat-bubble.user').length===2");
 assert.equal(writes.filter(w=>w.path.endsWith('/conversation')).at(-1).data.mode,'discuss');
 await click('.studio-discussion-history summary');
 checks.push('Selected-page AI revision and discussion retain failed input and show replies without a tab switch');
 await click('.inspector-edit-actions button:last-child');await wait("document.querySelector('.studio-page-thumbnail:nth-child(3)').textContent.includes('已确认')");
 assert.equal(task.slide_reviews[0].approved,false);await click('.studio-page-thumbnail:first-child');
 await click('.inspector-edit-actions button:first-child');await wait("!!document.querySelector('.studio-slide.selected .studio-slide-editor input')");
 await fill('.studio-slide.selected .studio-slide-editor input','课堂实验：观察浮力');await click('.studio-slide.selected .studio-slide-editor>button.primary');await wait("!document.querySelector('.unsaved-note')");
 assert.equal(task.result.slides[0].title,'课堂实验：观察浮力');assert.ok(task.slide_reviews.every(s=>!s.approved));
 await click('.studio-slide.selected .slide-tools button[aria-label="编辑第1页"]');
 checks.push('Direct editing remains available and saving invalidates page approvals');
 for(const width of [1440,1024,820,390]){
  await call('Emulation.setDeviceMetricsOverride',{width,height:width===390?844:1000,deviceScaleFactor:1,mobile:width===390});await evaluate('window.scrollTo(0,0)');
  assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth+1'),'No horizontal overflow at '+width);
  assert.ok(await evaluate("getComputedStyle(document.querySelector('.studio-canvas')).overflowY==='visible'"),'Preview has no nested scrollbar at '+width);
  if(width>=1024)assert.ok(await evaluate("(()=>{const p=document.querySelector('.studio-canvas').getBoundingClientRect(),a=document.querySelector('.studio-conversation').getBoundingClientRect();return a.left>=p.right+12&&p.width>a.width})()"),'Preview leads beside the action card at '+width);
  else assert.ok(await evaluate("document.querySelector('.studio-conversation').getBoundingClientRect().top>=document.querySelector('.studio-canvas').getBoundingClientRect().bottom"),'Narrow layout shows preview before review');
  if(width===390)await shot('review-mobile.png');
 }
 checks.push('Desktop, tablet and mobile preserve preview priority and document scrolling');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
 failReview=true;await click('.studio-approve-all');await wait("document.querySelector('.studio-canvas>.alert.error')?.textContent.includes('未全部完成')");
 assert.equal(task.status,'awaiting_preview');await click('.studio-approve-all');await wait("document.querySelector('.studio-approve-all').textContent==='全部已审核'");
 assert.equal(writes.filter(w=>w.path.endsWith('/expand')).length,0);await click('.studio-completion .primary');await wait("!!document.querySelector('.studio-completion .consent')");
 assert.equal(await evaluate("document.querySelector('.studio-phase-list .current').textContent.trim()"),'3逐页审核');
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true);await click('.studio-completion .consent input');await click('.studio-completion .primary');await wait("document.querySelector('.studio-adjust-heading h2').textContent==='交付与调整'");
 assert.equal(task.status,'succeeded');await click('.chat-mode button:last-child');assert.ok(await evaluate("document.querySelector('.chat-composer').textContent.includes('5积分')"));
 checks.push('Bulk review errors recover; expansion and explicit delivery consent retain their separate gates');
 task.result.slides=Array.from({length:14},(_,i)=>({...structuredClone(deck.slides[i%3]),id:'slide-'+i}));task.slide_reviews=task.result.slides.map(s=>({id:s.id,hash:'h'+s.id,approved:false,vote:0}));task.status='awaiting_confirmation';
 await call('Page.reload');await wait("!!document.querySelector('.home-recent-work')");await click('.home-recent-work');await wait("document.querySelectorAll('.studio-page-thumbnail').length===14");
 assert.ok(await evaluate("(()=>{const t=document.querySelector('.studio-page-thumbnails');return t.scrollWidth>t.clientWidth&&t.getBoundingClientRect().height<130})()"),'Long decks use a compact horizontal page rail');
 await click('.studio-page-thumbnail:last-child');assert.equal(await evaluate("document.querySelector('.studio-slide.selected').dataset.slideId"),'slide-13');await shot('review-long-desktop.png');
 checks.push('Long deck page rail scrolls horizontally and selects the final page');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({checks,writes:writes.map(w=>w.path),errors},null,2));console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
