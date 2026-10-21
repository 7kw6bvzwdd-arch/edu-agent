// Explicit paid live run against an isolated backend supplied by the Go test.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const puppeteer = require('../animation-video/node_modules/puppeteer-core');

(async () => {
 const [base, out] = process.argv.slice(2);
 assert.ok(/^http:\/\/127\.0\.0\.1:\d+$/.test(base), 'isolated loopback backend required');
 await fs.mkdir(out, { recursive: true });
 const browser = await puppeteer.launch({ executablePath:'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', headless:true, args:['--disable-background-networking'], defaultViewport:{width:1440,height:1050} });
 let page, token, stage='login', passed=false;
 const briefs=[], created=[], errors=[], checks=[];
 const report=async()=>fs.writeFile(path.join(out,'long-conversation.json'),JSON.stringify({at:new Date().toISOString(),mode:'live',stage,passed,checks,briefs,created,errors,test_teacher_approval:true,human_quality_accepted:false},null,2));
 async function api(route, method='GET', body) {
  const response=await fetch(base+'/api'+route,{method,headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:body===undefined?undefined:JSON.stringify(body)});
  assert.ok(response.ok,route+' HTTP '+response.status);
  return response.json();
 }
 const waitTask=async(want)=>{
  const deadline=Date.now()+20*60*1000;
  while(Date.now()<deadline){
   const task=await api('/tasks/'+created[0].id);
   await fs.writeFile(path.join(out,'task.json'),JSON.stringify(task,null,2));
   if(task.status===want)return task;
   assert.ok(!['failed','cancelled'].includes(task.status),'task failed: '+(task.error||task.stage));
   await new Promise(r=>setTimeout(r,1500));
  }
  throw Error('task deadline exceeded');
 };
 try {
  token=(await api('/auth/login','POST',{username:'teacher',password:'test-pass-123'})).token;
  const before=await api('/billing');assert.equal((await api('/tasks')).length,0);
  page=await browser.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.evaluateOnNewDocument(()=>{
   window.__liveBriefs=[]; const original=window.fetch.bind(window);
   window.fetch=async(...args)=>{
    const response=await original(...args);
    if(String(args[0]).includes('/workspace/brief')){
     const item={input:JSON.parse(args[1].body),status:response.status};window.__liveBriefs.push(item);
     response.clone().text().then(raw=>{item.events=raw;for(const frame of raw.split('\n\n'))if(frame.startsWith('event: done\ndata: '))item.result=JSON.parse(frame.slice('event: done\ndata: '.length))}).catch(()=>{item.error='stream capture failed'});
    }
    return response;
   };
  });
  page.on('response',async response=>{
   const req=response.request(),route=new URL(req.url()).pathname;
   if(req.method()!=='POST')return;
   if(route==='/api/tasks'){created.push({...await response.json(),input:JSON.parse(req.postData())});await report()}
  });
  await page.goto(base);await page.evaluate(value=>sessionStorage.setItem('edu_token',value),token);await page.reload();
  await page.waitForFunction(()=>!!document.querySelector('.brief-composer textarea')&&!document.querySelector('.brief-composer textarea').disabled,{timeout:30000});
  const send=async(content)=>{
   const count=briefs.length;
   await page.waitForFunction(()=>!!document.querySelector('.brief-composer textarea')&&!document.querySelector('.brief-composer textarea').disabled,{timeout:30000});
   await page.$eval('.brief-composer textarea',(el,value)=>{el.value=value;el.dispatchEvent(new Event('input',{bubbles:true}))},content);
   await page.click('.brief-composer button[type=submit]');
   await page.waitForFunction(()=>document.querySelector('.brief-composer textarea').disabled,{timeout:10000});
   await page.waitForFunction(()=>!document.querySelector('.brief-composer textarea').disabled,{timeout:190000});
   for(let i=0;i<100&&(!briefs[count]?.result);i++){const latest=await page.evaluate(()=>window.__liveBriefs.at(-1));if(latest)briefs[count]=latest;if(!briefs[count]?.result)await new Promise(r=>setTimeout(r,100))}
   await report();
   assert.ok(briefs[count]?.result,'complete SSE response required');
   assert.equal(briefs[count].result.mode,'live');
   assert.equal(briefs[count].result.action,'discuss','discussion cannot start generation');
   assert.equal(created.length,0);await report();
  };
  const turns=[
   '先只沟通，不开始制作。准备八年级历史《辛亥革命的背景与影响》完整45分钟课，学生基础薄弱，先帮我整理需求。',
   '教学重点是区分武昌起义和中华民国临时政府成立，不要把两件事写成同一年同一天。继续讨论，不制作。',
   '保留史料讨论，让学生依据实际资料说明观点；没有原文不要虚构引文。暂不开始。',
   '不安排分组表演，这项活动取消；采用个人观察和同桌讨论。仅整理需求。',
   '独立练习的答案放在教师备注，投影页先不显示答案。继续沟通。',
   '课件使用清晰大字，每页突出一个教学问题，不为了装饰而配图。先不要制作。',
   '需要真实图片的页面按教学用途自动找图并保留出处；不把现代建筑照片称为1911年现场照片。先讨论。',
   '安排背景、过程、影响、史料观察、独立练习和回顾，关注学生已有中国近代史基础。暂不制作。',
   '区分历史事实、材料观点与我们的课堂解释，不把讨论题设置成唯一措辞答案。继续整理。',
   '保持45分钟安排，至少一项练习检查学生能否区分事件发生与制度变化。不要开始制作。',
   '取消表演的要求仍有效，保留个人练习和史料讨论，请继续记住前面的要求。先只聊天。',
   '不要默认已经核对人教版教材，我还没有上传教材原文；允许使用能追溯的公开资料。暂不制作。',
   '请综合前面全部约定，只简述有没有遗漏，仍然不要开始制作。',
   '补充学生回顾时需要解释革命影响与局限的区别。仅讨论，继续保留此前约定。',
   '教学目标应落实在具体学生任务中，答案仍放备注，不安排表演。先只整理需求。'
  ];
  for(let i=0;i<turns.length;i++){stage='discussion-'+(i+1);await send(turns[i]);process.stdout.write('live conversation turn '+(i+1)+' complete\n');if(i>=12&&briefs.some(item=>item.input.compact_context&&item.result?.context_summary))break}
  const compacted=briefs.find(item=>item.input.compact_context&&item.result?.context_summary);
  assert.ok(compacted,'real UI summary threshold must trigger');
  assert.match(compacted.result.context_summary,/史料/);assert.match(compacted.result.context_summary,/备注/);assert.match(compacted.result.context_summary,/表演/);
  checks.push('continuous real discussion reached UI summary threshold without creating a task');
  const bill=await api('/billing');for(const key of ['balance','reserved','available'])assert.equal(bill[key],before[key]);
  stage='manual-edit';
  await page.$eval('.brief-current-requirements',el=>el.open=true);
  await page.$eval('input[aria-label="修改课时（分钟）"]',el=>{el.value='40';el.dispatchEvent(new Event('input',{bubbles:true}))});
  await page.$eval('textarea[aria-label="修改已理解的教学要求"]',el=>{el.value+='\n教师最新修改：40分钟；新增目标：依据材料区分革命影响与局限；取消分组表演，答案放备注。';el.dispatchEvent(new Event('input',{bubbles:true}))});
  await page.$eval('textarea[aria-label="课件需求摘要"]',el=>{el.value+='\n教师手工摘要：只做40分钟，保留史料讨论和独立练习，取消分组表演，答案放教师备注。';el.dispatchEvent(new Event('input',{bubbles:true}))});
  await send('采用我刚刚手改的40分钟和新增目标，保留手工摘要中的要求，先确认，不要制作。');
  assert.equal(briefs.at(-1).result.settings.lesson_minutes,40);
  await page.waitForFunction(()=>!document.querySelector('.workspace-history-notice')?.textContent.includes('正在保存'),{timeout:10000});
  await new Promise(r=>setTimeout(r,1000));
  await page.reload();await page.waitForSelector('.brief-composer textarea');
  await page.waitForFunction(()=>!!document.querySelector('.brief-current-requirements'),{timeout:15000});
  assert.equal((await api('/tasks')).length,0);
  await page.screenshot({path:path.join(out,'long-after-restore.png'),fullPage:true});
  checks.push('manual duration, goals and summary applied; history restored without generation');
  stage='explicit-start';
  await page.$eval('.brief-composer textarea',el=>{el.value='已确认全部需求，现在开始制作这份40分钟历史课件。';el.dispatchEvent(new Event('input',{bubbles:true}))});
  await page.click('.brief-direct');
  await page.waitForFunction(()=>!!document.querySelector('.home-tool-result'),{timeout:190000});
  for(let i=0;i<100&&!created.length;i++)await new Promise(r=>setTimeout(r,100));
  assert.equal(created.length,1);assert.equal(created[0].input.lesson_minutes,40);
  assert.match(created[0].input.requirements,/史料/);assert.match(created[0].input.requirements,/备注/);assert.match(created[0].input.requirements,/取消.*表演|不.*表演/);
  checks.push('explicit start created one task using latest manual requirements');
  stage='preview';let task=await waitTask('awaiting_preview');
  const approve=async(task)=>{for(const row of task.slide_reviews)await api('/tasks/'+created[0].id+'/slides/'+row.id+'/review','POST',{revision:task.revision,hash:row.hash,approved:true})};
  await approve(task);await api('/tasks/'+created[0].id+'/expand','POST',{revision:task.revision});
  stage='full-deck';task=await waitTask('awaiting_confirmation');
  assert.equal(task.result.deck_check?.status,'checked');assert.ok(task.result.plan?.learning_goals?.length);
  await approve(task);await api('/tasks/'+created[0].id+'/confirm','POST',{result:task.result,revision:task.revision,accept_review:true});
  stage='delivery';task=await waitTask('succeeded');
  const download=await fetch(base+'/api/tasks/'+created[0].id+'/download/pptx',{headers:{Authorization:'Bearer '+token}});assert.equal(download.status,200);
  const bytes=Buffer.from(await download.arrayBuffer());assert.equal(bytes.subarray(0,2).toString(),'PK');await fs.writeFile(path.join(out,'long-conversation.pptx'),bytes);
  const after=await api('/billing');assert.equal(after.available,before.available-20);assert.equal(after.reserved,0);assert.equal((await api('/tasks')).length,1);
  assert.deepEqual(errors,[]);checks.push('real full-deck review, download and exactly-once settlement passed');passed=true;stage='complete';await report();
  process.stdout.write(JSON.stringify({passed,checks})+'\n');
 } finally {await report();if(token)await api('/auth/logout','POST',{}).catch(()=>{});await browser.close()}
})().catch(error=>{process.stderr.write('Long live acceptance failed: '+error.message+'\n');process.exitCode=1});
