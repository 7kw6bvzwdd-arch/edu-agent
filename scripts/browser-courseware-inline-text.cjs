// Intercept every API request: no production account, task or model writes.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/canvas-text-release/browser');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.slides=deck.slides.slice(0,3);deck.slides.forEach((s,i)=>s.id='slide-'+i);
 const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=','base64');
 const task={id:'fixture-progress',type:'courseware',status:'expanding',mode:'sample',revision:1,cost:20,input:{topic:'浮力',pages:12,preview_first:true,theme:'classroom'},result:deck,events:[],sources:[{id:'S1',hash:'fixture-hash',location:'文件第7页',file_page:7,content:'构造样例原文：浮力方向竖直向上。',textbook:{name:'原文证据样例.pdf',edition:'人教版 教师标注样例版次',chapter:'样例章节'}}],preview_images_available:true,slide_reviews:[]};
 deck.slides[0].evidence=[{claim:'浮力方向竖直向上',source_id:'S1',source_hash:'fixture-hash',quote:'浮力方向竖直向上。'}];
 const resume='已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。';deck.notice='请教师核验。 '+resume.repeat(3);
 let previews=0,polls=0,eventID=0,failBillingOnce=false,created=0,creation=null,saved=0,failSaveOnce=false,downloads=0; const reviews=new Map();const errors=[],checks=[];let reparsed=0;const documents=[{id:'document-fixture',name:'教材样例.pdf',original_available:true,status:'ready',chunks:3,created_at:'2026-10-13T12:00:00Z'}];
 function event(stage){task.events.push({id:++eventID,stage,message:stage})}
 event('batch_start_4_6');
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9285','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9285/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout: '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Page.javascriptDialogOpening')call('Page.handleJavaScriptDialog',{accept:true}).catch(e=>errors.push(String(e)));
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method!=='Fetch.requestPaused')return;
  const {requestId,request}=m.params,p=new URL(request.url).pathname;
  let reply,body,contentType='application/json',code=200;
  if(p.endsWith('/objects'))reply={version:'regions-v2',objects:[{id:'title',x:.06+(task.result.slides[0].canvas_positions?.title?.dx||0),y:.2+(task.result.slides[0].canvas_positions?.title?.dy||0),width:.8*(task.result.slides[0].canvas_positions?.title?.scale??1),height:.1*(task.result.slides[0].canvas_positions?.title?.scale??1),min_scale:16/(30*(task.result.slides[0].canvas_positions?.title?.scale??1))},{id:'body',x:.06,y:.35,width:.5,height:.35},{id:'focus',x:.06,y:.76,width:.85,height:.1}]};
  else if(p==='/api/documents/document-fixture/reparse'&&request.method==='POST'){reparsed++;documents[0].status='pending';reply={ok:true}}
  else if(p==='/api/documents'&&request.method==='GET')reply=documents;
  else if(p==='/api/tasks'&&request.method==='POST'){created++;creation=JSON.parse(request.postData);task.status='awaiting_preview';task.input=creation;task.result=deck;deck.slides[0].focus='课堂问题';task.revision=3;task.slide_reviews=deck.slides.map(s=>({id:s.id,hash:'hash-'+s.id,approved:false,vote:s.id==='slide-0'?-1:0,reason:'历史改进意见'}));reply={id:task.id}}
  else if(p==='/api/courseware/brief'&&request.method==='POST')reply={reply:'请补充课堂探究活动。',mode:'sample'};
  else if(p==='/api/tasks/'+task.id+'/draft'&&request.method==='POST'){const b=JSON.parse(request.postData);if(failSaveOnce){failSaveOnce=false;code=422;reply={message:'文字过密，请缩短要点；原版本保留'}}else if(b.revision!==task.revision){code=409;reply={message:'当前版本已变化'}}else{const old=task.result;task.result=b.result;task.revision++;saved++;task.slide_reviews=task.result.slides.map(s=>JSON.stringify(s)===JSON.stringify(old.slides.find(o=>o.id===s.id))?task.slide_reviews.find(m=>m.id===s.id):({id:s.id,hash:'saved-'+s.id,approved:false,vote:0}));reply={ok:true}}}
  else if(p.endsWith('/feedback')&&request.method==='POST'){const b=JSON.parse(request.postData),id=p.split('/').at(-2);assert.equal(b.vote,0);task.slide_reviews.find(s=>s.id===id).vote=0;reply={ok:true}}
  else if(p.endsWith('/review')&&request.method==='POST'){const b=JSON.parse(request.postData),id=p.split('/').at(-2);assert.equal(b.revision,task.revision);const m=task.slide_reviews.find(s=>s.id===id);assert.equal(b.hash,m.hash);assert.equal(m.vote,0);m.approved=b.approved;reviews.set(id,b);reply={ok:true}}
  else if(p==='/api/courseware/materials/preview'){reply={pages:[{page:1,text:'第十九章 一次函数'},{page:2,text:'y=kx+b，k不为零'}],material_snapshot:{name:'教材样例.pdf',edition:'教师标注测试版次',chapter:'原文章节'},page_warnings:[{file_page:1,kind:'catalogue'}]}}
  else if(p==='/api/courseware/materials')reply=[{id:'textbook1',name:'人教版数学八年级.pdf',pages:2,curriculum:{subject:'物理',grade:'八年级',edition:'人教版'},chapters:[{title:'第十九章 一次函数',start:1,end:2}]}];
  else if(/\/materials\/[^/]+\/pages\/\d+\/image$/.test(p))reply={file_page:2,data_url:'data:image/png;base64,'+png.toString('base64'),name:'原始教材.pdf',original_hash:'fixture-original-hash'};
  else if(p==='/api/courseware/references')reply=[];
  else if(request.method!=='GET'){errors.push('Unexpected write: '+p);code=400;reply={message:'No writes allowed'}}
  else if(p==='/api/health')reply={status:'ok',mode:'sample',courseware_canvas_version:'regions-v2',courseware_studio_version:'studio-v1',courseware_media_version:'media-v1',courseware_material_images_version:'page-images-v1',document_reparse_version:'reparse-v1',teaching_features_version:'teaching-v1',works_view_version:'views-v1'};
  else if(p==='/api/account')reply={username:'teacher',name:'测试教师'};
  else if(p==='/api/dashboard')reply={totals:{},tasks:[],usage:[]};
  else if(p==='/api/usage')reply={items:[],total:0};
  else if(p==='/api/auth/me')reply={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate','kb.manage']};
  else if(p==='/api/tenants')reply=[{id:1,name:'测试'}];
  else if(p==='/api/billing'){if(failBillingOnce){failBillingOnce=false;code=503;reply={message:'测试瞬时网络错误'}}else reply={balance:100,reserved:task.status==='failed'?0:20,available:task.status==='failed'?100:80,plan:'basic',ledger:[]}}
  else if(p==='/api/tasks')reply=[task];
  else if(p==='/api/tasks/'+task.id){polls++;reply=task}
  else if(p.endsWith('/conversation'))reply=[];
  else if(p.endsWith('/events')){contentType='text/event-stream';body=Buffer.from('id: '+eventID+'\ndata: '+JSON.stringify(task.events.at(-1))+'\n\n')}
  else if(p.endsWith('/preview-pptx')||p.endsWith('/download/pptx')){downloads++;contentType='application/vnd.openxmlformats-officedocument.presentationml.presentation';body=Buffer.from('fixture-pptx')}
  else if(p.endsWith('/preview')){previews++;contentType='image/png';body=png}
  else{errors.push('Unexpected API: '+p);code=404;reply={}}
  call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:(body||Buffer.from(JSON.stringify(reply))).toString('base64')}).catch(e=>errors.push(String(e)));
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<100;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate('document.body.innerText.slice(-2000)')+'\n'+JSON.stringify(errors))};
 const click=label=>evaluate(`(()=>{const b=[...(document.querySelector('.classroom-presentation')||document).querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing button');b.click()})()`);
 const srcs=()=>evaluate("[...document.querySelectorAll('.studio-slide-picture img')].map(i=>i.src)");
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token')"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:process.env.TEST_APP_URL||'http://127.0.0.1:8080'});
 await wait("[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品'))");
 await wait("!!document.querySelector('.courseware-setup')");
 assert.equal(await evaluate("document.querySelectorAll('.courseware-setup').length"),1);
 assert.equal(await evaluate("!!document.querySelector('.config-panel')"),false);
 await evaluate("document.querySelector('.sidebar-toggle').click()");await wait("document.querySelector('.app-shell').classList.contains('sidebar-collapsed')");
 assert.ok(await evaluate("document.querySelector('nav button[aria-label=\"所有工具\"]').title==='所有工具'"));
 await evaluate("document.querySelector('.sidebar-toggle').click()");
 checks.push('Home requirements integrated, sidebar collapse and accessible tools navigation');
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'home-desktop.png'),Buffer.from(r.data,'base64')));
 await click('所有工具');assert.equal(await evaluate("!!document.querySelector('.courseware-setup')"),false);
 assert.ok(await evaluate("document.querySelectorAll('[data-tool]').length===2"));
 await evaluate("document.querySelector('.profile-open').click()");await wait("!!document.querySelector('.account-navigation')");
 assert.ok(await evaluate("[...document.querySelectorAll('.account-navigation button')].map(b=>b.textContent).join('|').includes('套餐与账单')"));
 await click('套餐与账单');await wait("document.querySelector('.content').innerText.includes('业务账单')");
 checks.push('All tools grouped; account settings, billing and usage share account navigation');
 await click('知识与来源');await wait("document.querySelector('.document-row')?.innerText.includes('已就绪')");await click('重新解析');await wait("document.querySelector('.document-row')?.innerText.includes('排队中')");assert.equal(reparsed,1);assert.equal(await evaluate("[...document.querySelectorAll('.document-row button')].some(b=>b.textContent==='重新解析')"),false);checks.push('Reparse confirmation queues existing document once, pending state prevents repeated request');
 await click('我的作品');await wait("!!document.querySelector('.task-open')");await evaluate("document.querySelector('.task-open').click()");
 await wait("document.querySelectorAll('.studio-slide-picture img').length===3");
 await evaluate("document.querySelector('.claim-sources summary').click()");await wait("document.querySelector('.claim-sources').innerText.includes('文件第7页')");assert.ok(await evaluate("document.querySelector('.claim-sources').innerText.includes('人教版 教师标注样例版次')"));assert.ok(await evaluate("document.querySelector('.claim-sources blockquote').innerText.includes('浮力方向竖直向上')"));checks.push('Per-page actual excerpt, teacher edition and file-page distinction shown');
 task.status='succeeded';task.revision++;
 deck.slides[0]={...deck.slides[0],layout:'exercise',title:'计算与解释',bullets:['计算2+3','解释你的计算方法'],answer:'答案是5',notes:'TEACHER_ONLY_HINT'};
 deck.slides[1]={...deck.slides[1],title:'三角形',scientific:{kind:'triangle',title:'三边关系',sides:[3,4,5]},graph:null,diagram:'',chart:null,table:null,figure:null};
 deck.slides[2]={...deck.slides[2],title:'断路电流',scientific:{kind:'circuit',title:'断开的串联电路',voltage:6,resistances:[10,20],connection:'series',closed:false},graph:null,diagram:'',chart:null,table:null,figure:null};
 await evaluate("document.querySelector('.studio-header-actions button').click()");
 await click('我的作品');await evaluate("document.querySelector('.task-open').click()");
 await wait("[...document.querySelectorAll('button')].some(b=>b.textContent==='课堂演示')");
 await click('课堂演示');await wait("!!document.querySelector('.classroom-presentation')");
 assert.equal(await evaluate("document.querySelector('.classroom-presentation').innerText.includes('TEACHER_ONLY_HINT')"),false);
 assert.equal(await evaluate("document.querySelector('.classroom-presentation').innerText.includes('答案是5')"),false);
 assert.equal(await evaluate("getComputedStyle(document.querySelector('.presentation-body li')).visibility"),'hidden');
 await click('下一步');assert.equal(await evaluate("getComputedStyle(document.querySelector('.presentation-body li')).visibility"),'visible');
 await click('揭示答案');await wait("document.querySelector('.presentation-answer')?.textContent==='答案是5'");
 await click('下一步');await click('下一步');await wait("!!document.querySelector('.presentation-page .scientific-preview svg')");
 assert.equal(await evaluate("!!document.querySelector('.presentation-answer')"),false);
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'classroom.png'),Buffer.from(r.data,'base64')));
 for(let i=0;i<10&&!await evaluate("document.querySelector('.presentation-page h1')?.textContent==='断路电流'");i++)await click('下一步');
 await wait("document.querySelector('.scientific-preview').innerText.includes('总电流 0 A')");
 assert.equal(await evaluate("document.querySelector('.scientific-switch')?.getAttribute('d')"),'M35 75 V98 M35 128 V180 M35 98 L53 117');
 await click('退出');await wait("!document.querySelector('.classroom-presentation')");
 assert.equal(await evaluate("document.querySelectorAll('[aria-label^=\"点赞\"],[aria-label^=\"点踩\"]').length"),0);
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'studio-desktop.png'),Buffer.from(r.data,'base64')));
 await click('新建课件');await wait("!!document.querySelector('.teaching-materials')");await evaluate("document.querySelector('.brief-parameters').open=true");
 await wait("[...document.querySelectorAll('.teaching-materials select option')].some(o=>o.value==='textbook1')");
 const material=await evaluate("(()=>{const s=[...document.querySelectorAll('.teaching-materials label')].find(l=>l.textContent.startsWith('教材原文')).querySelector('select');s.value='textbook1';s.dispatchEvent(new Event('change',{bubbles:true}));return s.value})()");assert.equal(material,'textbook1');
 await click('查看匹配原文');await wait("document.querySelector('.teaching-materials').innerText.includes('y=kx+b，k不为零')");
 await wait("[...document.querySelectorAll('button')].some(b=>b.textContent.includes('查看原始页面'))");await click('查看原始页面');await wait("!!document.querySelector('.original-page img')");checks.push('Original PDF image in material preview');
 assert.ok(await evaluate("document.querySelector('.teaching-materials').innerText.includes('教师标注测试版次')"));assert.ok(await evaluate("document.querySelector('.teaching-materials').innerText.includes('目录只能定位章节')"));checks.push('Material preview preserves edition/chapter and directory warning');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 await evaluate("document.querySelector('.brief-parameters').open=false;document.querySelector('.brief-summary details').open=true");
 await evaluate("(()=>{const x=document.querySelector('[aria-label=\"课件需求摘要\"]');x.value+='\\n教师自定义：先观察，再推导。';x.dispatchEvent(new Event('input',{bubbles:true}))})()");
 await click('确认需求，生成三页质量预览');await wait("!!document.querySelector('.courseware-studio')");await wait("!!document.querySelector('[aria-label=\"移动第1页对象\"]')");
 assert.equal(created,1);assert.ok(creation.requirements.includes('教师自定义'));
 assert.equal(await evaluate("!!document.querySelector('.config-panel')||!!document.querySelector('.courseware-setup')"),false);
 assert.equal(await evaluate("document.querySelectorAll('[aria-label^=\"点赞\"],[aria-label^=\"点踩\"]').length"),0);
 await click('一键审核通过');await wait("document.querySelector('.studio-completion').innerText.includes('全部已审核')");
 assert.equal(reviews.size,3);assert.ok(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent.includes('确认预览，制作其余'))?.disabled===false"));
 await evaluate("document.querySelector('[aria-label=\"编辑第1页\"]').click()");
 await evaluate("(()=>{const x=document.querySelector('.studio-slide-editor label input');x.value='教师直接编辑标题';x.dispatchEvent(new Event('input',{bubbles:true}))})()");
 await click('保存本页及其他修改');await wait("document.querySelector('.studio-slide-editor input').value==='教师直接编辑标题'&&!document.querySelector('.unsaved-note')");
 assert.equal(saved,1);assert.equal(task.result.slides[0].title,'教师直接编辑标题');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");

 await sleep(650);
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').scrollIntoView({block:'center',behavior:'instant'})");await sleep(300);
 const objectRect=await evaluate("(()=>{const e=document.querySelector('[aria-label=\"移动标题\"]'),a=e.getBoundingClientRect();if(!e.contains(document.elementFromPoint(a.x+a.width/2,a.y+a.height/2)))throw Error('Drag target obscured');return {x:a.x+a.width/2,y:a.y+a.height/2}})()");
 await call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...objectRect});
 await call('Input.dispatchMouseEvent',{type:'mouseMoved',button:'left',buttons:1,x:objectRect.x+10,y:objectRect.y+10});
 await call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,x:objectRect.x+10,y:objectRect.y+10});
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').focus()");
 await call('Input.dispatchKeyEvent',{type:'keyDown',key:'ArrowRight',code:'ArrowRight'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'ArrowRight',code:'ArrowRight'});
 await evaluate("(()=>{const x=document.querySelector('[aria-label=\"对象缩放\"]');x.value='90';x.dispatchEvent(new Event('input',{bubbles:true}))})()");
 assert.equal(await evaluate("document.querySelector('.object-actions output').textContent"),'90%');
 assert.ok(Math.abs(await evaluate("parseFloat(document.querySelector('[aria-label=\"移动标题\"]').style.width)")-72)<.001);
 assert.ok(await evaluate("!!document.querySelector('.unsaved-note')"));
 await click('保存画布及其他修改');await wait("!document.querySelector('.unsaved-note')");
 assert.equal(saved,2);assert.equal(task.result.slides[0].canvas_positions.title.scale,.9);assert.ok(task.result.slides[0].canvas_positions.title.dx>0);assert.ok(task.result.slides[0].canvas_positions.title.dy>0);
 const positionsSaved=JSON.stringify(task.result.slides[0].canvas_positions);
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");await sleep(650);
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'object-canvas.png'),Buffer.from(r.data,'base64')));
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').focus()");
 assert.equal(await evaluate("document.querySelector('.object-actions output').textContent"),'90%');
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').focus()");
 assert.ok(Math.abs(await evaluate("parseFloat(document.querySelector('[aria-label=\"移动标题\"]').style.width)")-72)<.001);
 await evaluate("(()=>{const x=document.querySelector('[aria-label=\"对象缩放\"]');x.value='100';x.dispatchEvent(new Event('input',{bubbles:true}))})()");
 assert.ok(Math.abs(await evaluate("parseFloat(document.querySelector('[aria-label=\"移动标题\"]').style.width)")-80)<.001);
 checks.push('Proportional scale saves at 90%; reopen does not multiply twice; returning to 100% restores base width');
 await click('恢复默认位置与大小');await click('保存画布及其他修改');await wait("!document.querySelector('.unsaved-note')");assert.equal(task.result.slides[0].canvas_positions,undefined);
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').focus()");await call('Input.dispatchKeyEvent',{type:'keyDown',key:'ArrowRight',code:'ArrowRight'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'ArrowRight',code:'ArrowRight'});
 await click('保存画布及其他修改');await wait("!document.querySelector('.unsaved-note')");
 const reopenedPositions=JSON.stringify(task.result.slides[0].canvas_positions);
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});

 checks.push('Pointer drag and keyboard adjustment save normalized native-object offsets');

 const pictureBefore=await srcs();await click('我的作品');await evaluate("document.querySelector('.task-open').click()");
 await wait("document.querySelector('.studio-slide-picture img')?.alt.includes('教师直接编辑标题')");
 await evaluate("document.querySelector('[aria-label=\"编辑第1页\"]').click()");await wait("document.querySelector('.studio-slide-editor input')?.value==='教师直接编辑标题'");await evaluate("document.querySelector('[aria-label=\"编辑第1页\"]').click()");
 await evaluate("document.querySelector('.modal-backdrop button')?.click()");
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'studio-desktop.png'),Buffer.from(r.data,'base64')));
 assert.equal(JSON.stringify(task.result.slides[0].canvas_positions),reopenedPositions);
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 await wait("!!document.querySelector('[aria-label=\"画布编辑标题\"]')");
 assert.ok(await evaluate("document.querySelector('.canvas-text-input').getBoundingClientRect().height>=44"));
 await evaluate("document.querySelector('.canvas-text-input').dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',isComposing:true,bubbles:true}))");
 assert.ok(await evaluate("!!document.querySelector('.canvas-text-input')"));

 const typeText=async (label,value)=>evaluate(`(()=>{const x=document.querySelector('[aria-label="'+${JSON.stringify(label)}+'"]');x.value=${JSON.stringify(value)};x.dispatchEvent(new Event('input',{bubbles:true}))})()`);
 await typeText('画布编辑标题','画布直接编辑标题');
 assert.ok(await evaluate("!!document.querySelector('.unsaved-note')"));
 await click('撤销文字修改');
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 assert.equal(await evaluate("document.querySelector('[aria-label=\"画布编辑标题\"]').value"),'教师直接编辑标题');
 await typeText('画布编辑标题','画布直接编辑标题');await click('完成输入');
 await evaluate("document.querySelector('[aria-label=\"移动正文\"]').click()");
 await typeText('画布编辑正文','正文第一条\n\n正文第二条');await click('完成输入');
 await evaluate("document.querySelector('[aria-label=\"移动课堂提问\"]').click()");
 await typeText('画布编辑课堂提问','如何解释这个现象？');await click('完成输入');
 await evaluate("document.querySelector('[aria-label=\"移动课堂提问\"]').focus()");
 const resizeControl=await evaluate("(()=>{const x=document.querySelector('[aria-label=\"对象缩放\"]');return {min:Number(x.min),max:Number(x.max)}})()");
 await evaluate(`(()=>{const x=document.querySelector('[aria-label="对象缩放"]');x.value=${Math.max(90,resizeControl.min)};x.dispatchEvent(new Event('input',{bubbles:true}))})()`);
 await evaluate("document.querySelector('[aria-label=\"移动课堂提问\"]').click()");
 await typeText('画布编辑课堂提问','');
 await click('撤销文字修改');
 await evaluate("document.querySelector('[aria-label=\"移动课堂提问\"]').click()");
 assert.equal(await evaluate("document.querySelector('[aria-label=\"画布编辑课堂提问\"]').value"),'如何解释这个现象？');
 await click('完成输入');

 const titleBefore=task.result.slides[0].title,savesBefore=saved;
 failSaveOnce=true;await click('保存画布及其他修改');
 await wait("document.querySelector('.alert.error')?.textContent.includes('文字过密')");
 assert.equal(task.result.slides[0].title,titleBefore);assert.equal(saved,savesBefore);
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 assert.equal(await evaluate("document.querySelector('[aria-label=\"画布编辑标题\"]').value"),'画布直接编辑标题');
 await typeText('画布编辑标题',' ');
 assert.equal(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent==='保存画布及其他修改').disabled"),true);
 await typeText('画布编辑标题','画布直接编辑标题');await click('完成输入');
 await evaluate("document.querySelector('[aria-label=\"移动正文\"]').click()");
 await typeText('画布编辑正文',Array.from({length:7},(_,i)=>'第'+i+'条').join('\n'));
 assert.ok(await evaluate("document.querySelector('.canvas-text-warning').textContent.includes('6')"));
 assert.equal(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent==='保存画布及其他修改').disabled"),true);
 await click('撤销文字修改');
 await click('保存画布及其他修改');await wait("!document.querySelector('.unsaved-note')");
 assert.equal(task.result.slides[0].title,'画布直接编辑标题');
 assert.deepEqual(task.result.slides[0].bullets,['正文第一条','正文第二条']);
 assert.equal(task.result.slides[0].focus,'如何解释这个现象？');
 assert.equal(saved,savesBefore+1);
 // Export without a separate save must persist valid inline edits first.
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 await typeText('画布编辑标题','导出前保存的标题');await click('完成输入');
 await click('下载预览 PPTX');await wait("!document.querySelector('.unsaved-note')");
 assert.equal(downloads,1);assert.equal(task.result.slides[0].title,'导出前保存的标题');
 checks.push('Export persists inline changes first');

 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 assert.equal(await evaluate("document.querySelector('[aria-label=\"画布编辑标题\"]').value"),'导出前保存的标题');
 await typeText('画布编辑标题','取消这个修改');
 await call('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape'});await call('Input.dispatchKeyEvent',{type:'keyUp',key:'Escape',code:'Escape'});
 assert.equal(await evaluate("!!document.querySelector('.canvas-text-input')"),false);
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 assert.equal(await evaluate("document.querySelector('[aria-label=\"画布编辑标题\"]').value"),'导出前保存的标题');
 await click('完成输入');
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').scrollIntoView({block:'center',behavior:'instant'})");await sleep(250);
 const titlePoint=await evaluate("(()=>{const r=document.querySelector('[aria-label=\"移动标题\"]').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()");
 await call('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...titlePoint});
 await call('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...titlePoint});
 await wait("!!document.querySelector('.canvas-text-input')");
 await typeText('画布编辑标题','导出前保存的标题');
 assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'));

 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'inline-text-mobile.png'),Buffer.from(r.data,'base64')));
 checks.push('Inline title/body/focus on mobile: undo, validation, failed-save retention, saved reopen and Escape');
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");
 await click('我的作品');await evaluate("document.querySelector('.task-open').click()");await wait("!!document.querySelector('.courseware-studio')");await wait("!!document.querySelector('[aria-label=\"移动第1页对象\"]')");
 await evaluate("document.querySelector('[aria-label=\"移动第1页对象\"]').click()");await wait("!!document.querySelector('.object-stage')");
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");await typeText('画布编辑标题','保留本地修改');await click('完成输入');
 task.revision++;const saveCount=saved;
 await click('保存画布及其他修改');await wait("document.querySelector('.alert.error')?.textContent.includes('版本')");assert.equal(saved,saveCount);
 await evaluate("document.querySelector('[aria-label=\"移动标题\"]').click()");
 assert.equal(await evaluate("document.querySelector('[aria-label=\"画布编辑标题\"]').value"),'保留本地修改');
 checks.push('Version conflict retains inline text without a successful write');

 checks.push('Home confirmation sends custom summary once; no repeated setup; old feedback cleared only on explicit review; three-page gate retained; direct edit saved and reopened');
 assert.ok(await evaluate('document.documentElement.scrollWidth<=innerWidth'));
 await call('Page.captureScreenshot',{format:'png'}).then(r=>fs.writeFile(path.join(output,'home-mobile.png'),Buffer.from(r.data,'base64')));
 checks.push('Step reveal and hidden teacher notes; explicit answer reveal; scientific classroom preview; textbook original page selection; mobile width');
 assert.deepEqual(errors,[]);await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({checks,errors},null,2));
 console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
