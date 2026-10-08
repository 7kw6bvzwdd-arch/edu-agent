// Fixture APIs only: no production login, paid model request, task or balance write.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
const {createHash}=require('node:crypto'),{spawn,execFile}=require('node:child_process');
const {promisify}=require('node:util');
const root=path.resolve(__dirname,'..'),sleep=ms=>new Promise(r=>setTimeout(r,ms)),clone=v=>JSON.parse(JSON.stringify(v));
let browser,socket;
(async()=>{
 const output=path.join(root,'work/courseware-studio');await fs.mkdir(output,{recursive:true});
 const deck=JSON.parse(await fs.readFile(path.join(root,'fixtures/courseware-quality-v2.json'),'utf8'));
 deck.theme='classroom';deck.slides.forEach((sl,i)=>sl.id='fixture-slide-'+i);
 const pptx=path.join(output,'fixture.pptx');
 const rendered=await fetch((process.env.TEST_RENDER_URL||'http://127.0.0.1:8000')+'/render/pptx',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(deck)});
 assert.ok(rendered.ok,'real fixture PPTX renders');
 await fs.writeFile(pptx,Buffer.from(await rendered.arrayBuffer()));
 await promisify(execFile)('powershell.exe',['-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',path.join(root,'scripts/render-courseware-preview.ps1'),'-Path',pptx,'-OutputDirectory',pptx+'.slides'],{windowsHide:true,timeout:60000});
 const pngs=await Promise.all(deck.slides.map((_,i)=>fs.readFile(path.join(pptx+'.slides','slide-'+(i+1)+'.png'))));
 const actor={tenant_id:1,member_id:1,name:'测试教师',role:'teacher',permissions:['courseware.generate','animation.render']};
 const errors=[],checks=[],messages=[],reviewRequests=[],feedback=new Map(),history=new Map();
 let task,creation,revisionRequest,failReviewOn='',changeAfterReview=false,expansions=0,confirmations=0,studioVersion='studio-v1';
 function metas(){return task.result.slides.map(sl=>{const hash=createHash('sha256').update(JSON.stringify([task.result.title,task.result.theme,sl])).digest('hex');return {id:sl.id,hash,...(feedback.get(hash)||{approved:false,vote:0,reason:''})}})}
 function view(){return {...task,slide_reviews:metas(),preview_images_available:true}}
 function archive(){history.set(task.revision,clone(task.result))}
 function mutate(d){task.result=clone(d);task.revision++;archive()}
 browser=spawn('C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',['--headless=new','--disable-gpu','--no-first-run','--disable-background-networking','--remote-debugging-port=9248','--remote-debugging-address=127.0.0.1','--user-data-dir='+path.join(output,'browser-profile'),'about:blank'],{windowsHide:true,stdio:'ignore'});
 let page;
 for(let i=0;i<40;i++){try{page=(await fetch('http://127.0.0.1:9248/json/list').then(r=>r.json())).find(p=>p.type==='page');if(page)break}catch{}await sleep(200)}
 assert.ok(page,'Edge starts');socket=new WebSocket(page.webSocketDebuggerUrl);
 await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
 let seq=0;const pending=new Map();
 const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq,timer=setTimeout(()=>reject(Error('CDP timeout: '+method)),15000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}))});
 socket.onmessage=e=>{
  const m=JSON.parse(e.data);
  if(m.id){const p=pending.get(m.id);if(p){clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}}
  if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);
  if(m.method==='Page.javascriptDialogOpening')call('Page.handleJavaScriptDialog',{accept:true}).catch(e=>errors.push(String(e)));
  if(m.method==='Fetch.requestPaused'){
   const {requestId,request}=m.params,url=new URL(request.url),p=url.pathname;
   const data=request.postData?JSON.parse(request.postData):{},method=request.method;
   let reply,code=200,binary,contentType='application/json';
   try{
    if(p==='/api/health')reply={status:'ok',mode:'sample',courseware_media_version:'media-v1',courseware_studio_version:studioVersion,works_view_version:'views-v1'};
    else if(p.endsWith('/events')){binary=Buffer.from(': fixture\n\n');contentType='text/event-stream'}
    else if(p==='/api/auth/me')reply=actor;
    else if(p==='/api/tenants')reply=[{id:1,name:'课件协作测试'}];
    else if(p==='/api/billing')reply={balance:100,reserved:task?.status==='succeeded'?0:20,available:80,plan:'basic',ledger:[]};
    else if(p==='/api/tasks'&&method==='POST'){
     creation=data;task={id:'fixture-studio',type:'courseware',status:'awaiting_preview',mode:'sample',revision:1,cost:20,input:data,result:{...clone(deck),slides:clone(deck.slides.slice(0,3))},sources:[],events:[]};archive();reply={id:task.id};code=202;
    }else if(p==='/api/tasks')reply=task?[view()]:[];
    else if(p==='/api/tasks/fixture-studio')reply=view();
    else if(p==='/api/tasks/fixture-studio/conversation'&&method==='GET')reply=messages;
    else if(p==='/api/tasks/fixture-studio/conversation'&&method==='POST'){
     revisionRequest=data;
     if(data.mode==='revise'){const d=clone(task.result),sl=d.slides.find(sl=>sl.id===data.slide_id);sl.notes+='\n'+data.content;mutate(d)}
     const msg={id:'message-'+messages.length,...data,status:'completed',reply:data.mode==='revise'?'已修改指定页；其他页保持不变。':'建议先区分浮力方向和浮沉条件。',vote:0};messages.push(msg);reply={id:msg.id,revision:task.revision,reply:msg.reply};
    }else if(/\/conversation\/[^/]+\/feedback$/.test(p)){
     Object.assign(messages.find(m=>m.id===p.split('/').at(-2)),{vote:data.vote,reason:data.reason});reply={ok:true};
    }else if(/\/slides\/[^/]+\/(review|feedback)$/.test(p)){
     const id=p.split('/').at(-2),meta=metas().find(m=>m.id===id),review=p.endsWith('/review');
     if(review)reviewRequests.push({id,...data});
     if(data.revision!==task.revision||data.hash!==meta.hash){code=409;reply={message:'当前版本已变化，请刷新后审核'}}
     else if(review&&id===failReviewOn){failReviewOn='';code=503;reply={message:'模拟审核服务暂时不可用'}}
     else if(review&&data.approved&&meta.vote<0){code=409;reply={message:'这页仍有点踩'}}
     else{
      if(review)meta.approved=data.approved;else{meta.vote=data.vote;meta.reason=data.reason;if(data.vote<0)meta.approved=false}
      feedback.set(meta.hash,{approved:meta.approved,vote:meta.vote,reason:meta.reason});reply={ok:true};
      if(review&&changeAfterReview){changeAfterReview=false;const d=clone(task.result);d.slides.at(-1).notes+='\n模拟另一窗口的内容更新';mutate(d)}
     }
    }else if(/\/slides\/[^/]+\/preview$/.test(p)){
     if(Number(url.searchParams.get('revision'))!==task.revision){code=409;reply={message:'当前预览版本已变化'}}
     else{const id=p.split('/').at(-2),i=deck.slides.findIndex(sl=>sl.id===id);binary=pngs[Math.max(0,i)];contentType='image/png'}
    }else if(p.endsWith('/draft')||p.endsWith('/edit')){mutate(data.result);reply={revision:task.revision}}
    else if(p.endsWith('/expand')){
     assert.ok(metas().every(m=>m.approved&&m.vote>=0),'only approved preview expands');assert.equal(task.result.slides.length,3);
     expansions++;mutate({...task.result,slides:task.result.slides.concat(clone(deck.slides.slice(3)))});task.status='awaiting_confirmation';reply={ok:true};code=202;
    }else if(p.endsWith('/confirm')){
     assert.ok(metas().every(m=>m.approved&&m.vote>=0));assert.equal(data.accept_review,true);assert.equal(data.revision,task.revision);
     confirmations++;task.status='succeeded';reply={ok:true};code=202;
     setTimeout(()=>{task.result.evaluation={status:'completed',assessment:{summary:'模拟独立评审，仍需教师核验'}}},100);
    }else if(p.endsWith('/revisions')){
     const rows=[...history].reverse().map(([revision,result])=>({revision,pages:result.slides.length,title:result.title}));reply=rows;
    }else if(/\/revisions\/\d+$/.test(p)){
     const revision=Number(p.split('/').at(-1));reply={revision,result:history.get(revision)};
    }else if(/\/revisions\/\d+\/restore$/.test(p)){
     mutate(history.get(Number(p.split('/').at(-2))));reply={ok:true};
    }else if(p.endsWith('/preview-pptx')||p.endsWith('/download/pptx')){binary=Buffer.from('PK fixture download');contentType='application/vnd.openxmlformats-officedocument.presentationml.presentation'}
    else throw Error('Unexpected fixture request: '+method+' '+p);
   }catch(e){code=400;reply={message:e.message};errors.push(e.stack)}
   call('Fetch.fulfillRequest',{requestId,responseCode:code,responseHeaders:[{name:'Content-Type',value:contentType}],body:(binary||Buffer.from(JSON.stringify(reply))).toString('base64')}).catch(e=>errors.push(String(e)));
  }
 };
 const evaluate=async expression=>{const r=await call('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result.value};
 const wait=async expression=>{for(let i=0;i<70;i++){if(await evaluate(expression))return;await sleep(100)}throw Error('UI timeout: '+expression+'\n'+await evaluate("document.body.innerText.slice(-2500)")+'\n'+JSON.stringify(errors))};
 const click=label=>evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.includes(${JSON.stringify(label)}));if(!b||b.disabled)throw Error('Missing/disabled button '+${JSON.stringify(label)});b.click()})()`);
 const byAria=label=>evaluate(`document.querySelector('[aria-label='+${JSON.stringify(JSON.stringify(label))}+']').click()`);
 const fill=(selector,value)=>evaluate(`(()=>{const n=document.querySelector(${JSON.stringify(selector)});n.value=${JSON.stringify(value)};n.dispatchEvent(new Event('input',{bubbles:true}))})()`);
 const idle=()=>wait("!document.querySelector('.chat-send-row button').textContent.includes('处理中')");
 const screenshot=async name=>fs.writeFile(path.join(output,name),Buffer.from((await call('Page.captureScreenshot',{format:'png'})).data,'base64'));
 const reload=async()=>{const stamp=await evaluate("window.__studioFixtureLoad");await call('Page.reload');await wait("window.__studioFixtureLoad!=="+JSON.stringify(stamp)+"&&[...document.querySelectorAll('nav button')].some(b=>b.textContent.includes('我的作品'))")};
 await call('Runtime.enable');await call('Page.enable');await call('Fetch.enable',{patterns:[{urlPattern:'*/api/*'}]});
 await call('Page.addScriptToEvaluateOnNewDocument',{source:"sessionStorage.setItem('edu_token','fixture-token');window.__studioFixtureLoad=crypto.randomUUID()"});
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await call('Page.navigate',{url:'http://127.0.0.1:8080'});await wait("!!document.querySelector('.workbench')&&!document.querySelector('.config-panel .primary').disabled");
 await evaluate("(()=>{const l=[...document.querySelectorAll('.config-panel label')].find(l=>l.textContent.startsWith('课件页数'));const s=l.querySelector('select');s.value='8';s.dispatchEvent(new Event('change',{bubbles:true}));document.querySelector('.config-panel form').requestSubmit()})()");
 await wait("document.querySelectorAll('.studio-slide').length===3");
 assert.equal(creation.preview_first,true);assert.equal(creation.pages,8);
 assert.equal(await evaluate("getComputedStyle(document.querySelector('.config-panel')).display"),'none');
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true);
 assert.equal(await evaluate("document.querySelector('.studio-approve-all')?.textContent"),'一键审核通过');
 assert.equal(await evaluate("document.querySelector('.studio-approve-all').disabled"),false);
 await wait("document.querySelectorAll('.studio-slide-picture img').length===3");
 checks.push('New creation requests staged generation; exactly three actual PPTX snapshots and no hidden full deck');
 await screenshot('studio-desktop.png');
 await byAria('点踩第1页');await fill('textarea[aria-label="点踩原因"]','缺少具体观察问题');await click('提交点踩原因');await idle();
 assert.equal(metas()[0].vote,-1);assert.equal(metas()[0].approved,false);
 assert.equal(await evaluate("document.querySelector('.studio-slide-bottom>div:last-child button:nth-child(2)').disabled"),true);
 await click('一键审核通过');await idle();
 assert.equal(await evaluate("document.querySelector('.alert.error')?.textContent.includes('第1页仍有点踩')"),true);
 assert.equal(reviewRequests.length,0,'Preflight blocks the whole bulk review before any page approval');
 assert.equal(expansions,0);assert.equal(confirmations,0);
 checks.push('Downvote requires a reason and blocks page approval / expansion without charging teachers');
 const unchanged=clone(task.result.slides.slice(1));
 await click('修改本页');await fill('[aria-label="继续课件对话"]','加入木块漂浮的观察提问');await click('发送 ↑');await idle();
 assert.equal(revisionRequest.slide_id,deck.slides[0].id);assert.equal(revisionRequest.mode,'revise');assert.deepEqual(task.result.slides.slice(1),unchanged);
 assert.equal(metas()[0].vote,0);assert.equal(metas()[0].approved,false);
 await byAria('点赞回复 message-0');await idle();assert.equal(messages[0].vote,1);
 checks.push('Conversation modifies only selected slide, keeps identity, invalidates old hash feedback; reply thumbs-up persists');
 await byAria('点踩第3页');await fill('textarea[aria-label="点踩原因"]','这一页需要补充条件');await click('提交点踩原因');await idle();
 await click('一键审核通过');await idle();
 assert.equal(await evaluate("document.querySelector('.alert.error')?.textContent.includes('第3页仍有点踩')"),true);
 assert.equal(reviewRequests.length,0,'A later downvoted page must also block approval of earlier pages');
 await byAria('点踩第3页');await idle();assert.equal(metas()[2].vote,0);
 await evaluate("document.querySelector('.studio-slide-bottom>div:last-child').querySelectorAll('button')[1].click()");
 await wait("document.querySelectorAll('.studio-slide-bottom .approved').length===1");
 await byAria('编辑第2页');await fill('.studio-slide-editor input','观察：测力计的示数为什么变小？');
 const previewReviewStart=reviewRequests.length;
 await evaluate("(()=>{const b=document.querySelector('.studio-approve-all');b.click();b.click()})()");await idle();
 assert.equal(task.result.slides[1].title,'观察：测力计的示数为什么变小？');
 assert.equal(reviewRequests.length-previewReviewStart,2,'Skip the approved page and suppress duplicate clicks');
 assert.ok(reviewRequests.slice(previewReviewStart).every(r=>r.revision===task.revision&&r.hash===metas().find(m=>m.id===r.id).hash),'Review latest hashes after saving edits');
 assert.equal(await evaluate("document.querySelectorAll('.studio-slide-bottom .approved').length"),3);
 assert.equal(await evaluate("document.querySelector('.studio-approve-all').disabled"),true);
 assert.equal(await evaluate("document.querySelector('.studio-approve-all').textContent"),'全部已审核');
 assert.equal(expansions,0);assert.equal(confirmations,0);assert.equal(task.status,'awaiting_preview');
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),false);
 checks.push('One-click teacher review saves pending edits, uses current hashes, skips approved pages, prevents duplicate clicks and does not expand / deliver');
 const approved=clone(task.result.slides);
 await click('确认前三页，生成剩余');await wait("document.querySelectorAll('.studio-slide').length===8");
 assert.equal(expansions,1);assert.deepEqual(task.result.slides.slice(0,3),approved);
 checks.push('Saved edits require fresh page review; only confirming all three generates remaining five pages');
 await byAria('编辑第2页'); // Close the prior editor; reproduce later-page PPT viewing itself.
 await evaluate("document.querySelector('.studio-canvas').scrollTop=0");
 const scrollBefore=await evaluate("(()=>{const c=document.querySelector('.studio-canvas'),chat=document.querySelector('.studio-conversation'),composer=document.querySelector('.chat-composer');return {height:c.clientHeight,contentHeight:c.scrollHeight,overflow:getComputedStyle(c).overflowY,chatTop:chat.getBoundingClientRect().top,composerTop:composer.getBoundingClientRect().top,windowY:scrollY}})()");
 assert.equal(scrollBefore.overflow,'auto','PPT canvas must scroll independently, not move the whole page');
 assert.ok(scrollBefore.contentHeight>scrollBefore.height+100,'Full deck overflows a bounded PPT canvas');
 const wheelPoint=await evaluate("(()=>{const r=document.querySelector('.studio-canvas').getBoundingClientRect();return {x:r.left+r.width/2,y:Math.min(innerHeight-30,r.top+300)}})()");
 await call('Input.dispatchMouseEvent',{type:'mouseWheel',...wheelPoint,deltaY:650,deltaX:0});
 await wait("document.querySelector('.studio-canvas').scrollTop>200");
 const scrollAfter=await evaluate("(()=>{const chat=document.querySelector('.studio-conversation'),composer=document.querySelector('.chat-composer');return {chatTop:chat.getBoundingClientRect().top,composerTop:composer.getBoundingClientRect().top,windowY:scrollY}})()");
 assert.ok(Math.abs(scrollAfter.chatTop-scrollBefore.chatTop)<1&&Math.abs(scrollAfter.composerTop-scrollBefore.composerTop)<1,'Scrolling later slides must not hide the conversation or composer');
 assert.equal(scrollAfter.windowY,scrollBefore.windowY,'Mouse wheel stays inside the PPT canvas');
 await screenshot('studio-independent-scroll.png');
 const keyboardBefore=await evaluate("document.querySelector('.studio-canvas').scrollTop");
 await evaluate("document.querySelector('[aria-label=\"课件页面滚动区域\"]').focus({preventScroll:true})");
 await call('Input.dispatchKeyEvent',{type:'keyDown',key:'PageDown',code:'PageDown',windowsVirtualKeyCode:34});
 await call('Input.dispatchKeyEvent',{type:'keyUp',key:'PageDown',code:'PageDown',windowsVirtualKeyCode:34});
 await wait("document.querySelector('.studio-canvas').scrollTop>"+keyboardBefore);
 await evaluate("document.querySelector('.studio-canvas').scrollTop=document.querySelector('.studio-canvas').scrollHeight");
 const footerVisible=await evaluate("(()=>{const r=document.querySelector('.studio-completion').getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight+1})()");
 assert.equal(footerVisible,true,'Final review controls remain reachable at canvas bottom');
 await call('Input.dispatchMouseEvent',{type:'mouseWheel',...wheelPoint,deltaY:650,deltaX:0});
 await sleep(150);
 assert.equal(await evaluate("scrollY"),scrollBefore.windowY,'Scrolling beyond final slide does not pull away the chat');
 await call('Emulation.setDeviceMetricsOverride',{width:1366,height:768,deviceScaleFactor:1,mobile:false});
 assert.equal(await evaluate("(()=>{const r=document.querySelector('.chat-composer').getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight+1})()"),true,'Composer fits a 768px-high desktop');
 await screenshot('studio-scroll-short-desktop.png');
 await call('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
 await evaluate("document.querySelector('.studio-canvas').scrollTop=document.querySelector('.studio-canvas').scrollHeight");
 await byAria('点踩第8页');
 await wait("(()=>{const f=document.querySelector('.feedback-form'),c=document.querySelector('.studio-canvas');if(!f)return false;const r=f.getBoundingClientRect(),p=c.getBoundingClientRect();return r.top>=p.top&&r.bottom<=p.bottom})()");
 await evaluate("document.querySelector('.feedback-form button:last-child').click()");
 await evaluate("document.querySelector('.studio-canvas').scrollTop=0");
 checks.push('Full-deck mouse-wheel scroll is confined to PPT canvas; chat and composer stay visible');
 await byAria('复制第4页');await wait("document.querySelectorAll('.studio-slide').length===9");await byAria('删除第5页');await wait("document.querySelectorAll('.studio-slide').length===8");
 await byAria('上移第8页');await byAria('下移第7页');
 await byAria('编辑第4页');await fill('.studio-slide-editor input','排液体积决定浮力大小');
 await click('保存修改');await idle();assert.deepEqual(task.result.slides.slice(0,3),approved);
 checks.push('Full-deck stage retains duplicate / delete / reorder and free manual save');
 failReviewOn=deck.slides[5].id;
 await click('一键审核通过');await idle();
 assert.equal(metas().filter(m=>m.approved).length,5);
 assert.equal(await evaluate("document.querySelectorAll('.studio-slide-bottom .approved').length"),5,'Partial failure refreshes actual approvals');
 assert.equal(await evaluate("document.querySelector('.alert.error')?.textContent.includes('未全部完成')"),true);
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true);assert.equal(confirmations,0);
 const retryReviewStart=reviewRequests.length;
 await click('一键审核通过');await idle();
 assert.equal(reviewRequests.length-retryReviewStart,3,'Retry only outstanding pages');
 assert.ok(metas().every(m=>m.approved));assert.equal(confirmations,0);
 assert.equal(await evaluate("document.querySelector('.studio-completion .consent input').checked"),false);
 assert.equal(await evaluate("document.querySelector('.studio-completion .primary').disabled"),true,'Bulk approval cannot grant fee consent');
 checks.push('Full-deck bulk review recovers partial service failures truthfully; retries only outstanding pages and retains explicit delivery consent');
 for(const i of [6,7]){await evaluate(`document.querySelectorAll('.studio-slide-bottom>div:last-child')[${i}].querySelectorAll('button')[1].click()`);await idle()}
 changeAfterReview=true;const conflictRevision=task.revision;
 await click('一键审核通过');await idle();
 assert.equal(task.revision,conflictRevision+1);assert.equal(metas().at(-1).approved,false);
 assert.equal(await evaluate("document.querySelector('.alert.error')?.textContent.includes('未全部完成')"),true);
 const conflictRetryStart=reviewRequests.length;
 await click('一键审核通过');await idle();
 assert.equal(reviewRequests.length-conflictRetryStart,1);assert.equal(reviewRequests.at(-1).revision,task.revision);
 assert.ok(metas().every(m=>m.approved));assert.equal(confirmations,0);
 checks.push('Concurrent revision changes halt bulk review; new content is not approved with stale hashes and can be reviewed on retry');
 await evaluate("document.querySelector('.studio-completion .consent input').click()");await click('全部审核通过，完整交付');await wait("document.querySelector('.studio-header p').textContent.includes('课件已完成')");
 assert.equal(confirmations,1);checks.push('Final delivery requires every page approved and explicit fee consent');
 await wait("document.querySelector('.studio-canvas').textContent.includes('独立模型辅助评审')");
 assert.equal(await evaluate("document.querySelector('.studio-toolbar button').disabled"),true,'Background evaluation is not an unsaved user edit');
 await click('版本历史');await wait("!!document.querySelector('.studio-history')");await click('查看内容');await wait("!!document.querySelector('.studio-history article')");
 assert.ok(await evaluate("document.querySelector('.studio-history details').textContent.includes('教师核对')||document.querySelector('.studio-history details').textContent.includes('讲解')"));
 await click('关闭');await click('讨论课件');await fill('[aria-label="继续课件对话"]','怎么讲解浮沉条件？');await click('发送 ↑');await idle();
 assert.equal(revisionRequest.mode,'discuss');checks.push('Finished deck retains history content / notes, downloads and ongoing discussion');
 await call('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
 assert.equal(await evaluate("(()=>{const c=document.querySelector('.studio-canvas');return c.clientHeight<=Math.ceil(innerHeight*.75)+1&&c.scrollHeight>c.clientHeight})()"),true,'Mobile PPT area remains bounded and scrollable');
 await evaluate("window.scrollTo(0,0)");assert.equal(await evaluate("document.documentElement.scrollWidth<=390"),true);await screenshot('studio-mobile.png');
 await evaluate("document.querySelector('.studio-slide-picture').scrollIntoView()");await screenshot('studio-mobile-slide.png');
 checks.push('390px mobile conversation and slide review have no horizontal overflow');
 await reload();await click('我的作品');await wait("!!document.querySelector('.task-open')");await evaluate("document.querySelector('.task-open').click()");
 await wait("document.querySelectorAll('.chat-bubble.user').length===3");assert.equal(await evaluate("document.querySelectorAll('.studio-slide').length"),8);
 checks.push('Reopening a work preserves conversation, selected workflow and approvals');
 await click('新建课件');studioVersion='';await reload();await wait("!!document.querySelector('.config-panel form')");const previous=creation;
 await evaluate("document.querySelector('.config-panel form').requestSubmit()");await wait("document.querySelector('.alert.error')?.textContent.includes('分阶段课件服务尚未更新')");assert.equal(creation,previous);
 checks.push('Outdated backend is blocked rather than silently reverting to full-deck generation');
 assert.deepEqual(errors,[]);
 await fs.writeFile(path.join(output,'browser.json'),JSON.stringify({checks,errors,scope:'Intercepted fixture APIs; actual fixture PPTX/Office PNGs; zero production writes or paid model calls'},null,2));
 console.log(JSON.stringify({checks}));await call('Browser.close');socket.close();
})().catch(e=>{console.error(e.stack);process.exitCode=1;socket?.close();browser?.kill()});
