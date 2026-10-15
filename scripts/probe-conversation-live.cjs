// Read-only production conversation probe; never creates a courseware task.
const fs=require('node:fs/promises'),path=require('node:path'),assert=require('node:assert/strict');
(async()=>{
 const root=path.resolve(__dirname,'..'),base='http://127.0.0.1:8080/api';
 const env=Object.fromEntries((await fs.readFile(path.join(root,'.env'),'utf8')).split(/\r?\n/).filter(s=>/^[^#=]+=/.test(s)).map(s=>{const i=s.indexOf('=');return[s.slice(0,i).trim(),s.slice(i+1).trim()]}));
 let token;
 async function api(url,method='GET',body){const r=await fetch(base+url,{method,headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:body===undefined?undefined:JSON.stringify(body)});if(!r.ok)throw Error('Probe HTTP '+r.status+' '+url);return r.json()}
 try{
  token=(await api('/auth/login','POST',{username:'teacher',password:env.SEED_PASSWORD})).token;assert.ok(token);
  const before=await api('/billing'),tasks=await api('/tasks');
  const started=performance.now(),events=[];let first,done,result;
  const response=await fetch(base+'/courseware/brief',{method:'POST',signal:AbortSignal.timeout(190000),headers:{'Content-Type':'application/json',Authorization:'Bearer '+token},body:JSON.stringify({stream:true,intent:'discuss',input:{type:'courseware',topic:'',subject:'物理',grade:'八年级',textbook:'人教版',lesson_minutes:45,detail:'default',use_kb:true,use_web:true},messages:[{role:'user',content:'制作九年级二次函数课件，40分钟，学生已学一次函数，先讲概念，再通过例题和练习巩固。'}]})});
  assert.equal(response.status,200);assert.ok(response.headers.get('content-type').startsWith('text/event-stream'));
  const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='';
  while(true){const chunk=await reader.read();buffer+=decoder.decode(chunk.value,{stream:!chunk.done});let end;while((end=buffer.indexOf('\n\n'))>=0){const lines=buffer.slice(0,end).split('\n');buffer=buffer.slice(end+2);const event=lines.find(s=>s.startsWith('event:'))?.slice(6).trim();const data=lines.filter(s=>s.startsWith('data:')).map(s=>s.slice(5).trimStart()).join('\n');if(!data)continue;const value=JSON.parse(data);events.push(event);if(event==='error')throw Error('Live brief returned incomplete response');if(event==='delta'&&first===undefined)first=performance.now()-started;if(event==='done'){done=performance.now()-started;result=value}}if(chunk.done)break}
  assert.ok(first!==undefined&&done>first);assert.equal(result.mode,'live');assert.equal(result.settings.subject,'数学');assert.equal(result.settings.grade,'九年级');assert.equal(result.settings.lesson_minutes,40);assert.match(result.settings.topic,/二次函数/);assert.ok(result.blocks.length>=2);
  const after=await api('/billing'),afterTasks=await api('/tasks');
  for(const key of ['balance','reserved','available'])assert.equal(after[key],before[key]);assert.deepEqual(afterTasks,tasks);
  const report={mode:result.mode,events,first_delta_ms:Math.round(first),done_ms:Math.round(done),settings:result.settings,blocks:result.blocks,task_list_unchanged:true,billing_unchanged:true,courseware_brief_version:(await api('/health')).courseware_brief_version};
  await fs.writeFile(path.join(root,'work/conversation-first/live-brief.json'),JSON.stringify(report,null,2));
  process.stdout.write(JSON.stringify({mode:report.mode,delta_count:events.filter(e=>e==='delta').length,first_delta_ms:report.first_delta_ms,done_ms:report.done_ms,task_list_unchanged:true,billing_unchanged:true})+'\n');
 }finally{if(token)await api('/auth/logout','POST',{}).catch(()=>{})}
})().catch(()=>{process.stderr.write('Live conversation probe failed; credentials and response suppressed.\n');process.exitCode=1});
