// Trusted host. Each authored scene has an opaque origin; only frame data crosses.
const TeachingDirect = (() => {
 const frames=new Map();let sequence=0,lastKey='',pending=Promise.resolve();
 const data=JSON.parse(document.getElementById('anim-data').textContent),boot=data.direct_boot;
 function error(message){let el=document.getElementById('author-error');if(!el){el=document.createElement('div');el.id='author-error';el.setAttribute('role','alert');document.querySelector('.frame').appendChild(el)}el.textContent='画面代码未完成：'+message;dispatchEvent(new Event('teaching-direct-error'));}
 function entry(sc){
  if(frames.has(sc.id))return frames.get(sc.id);
  const iframe=document.createElement('iframe');iframe.className='authored-scene';iframe.title=sc.title;iframe.setAttribute('sandbox','allow-scripts');iframe.setAttribute('allow','');iframe.hidden=true;
  let readyResolve;const ready=new Promise(r=>readyResolve=r),waiting=new Map();
  const e={iframe,ready,waiting,state:'',error:null};frames.set(sc.id,e);const readyTimer=setTimeout(()=>{e.error='页面加载超时';readyResolve()},5000);ready.then(()=>clearTimeout(readyTimer));
  addEventListener('message',event=>{
   if(event.source!==iframe.contentWindow)return;const d=event.data;
   if(d?.type==='author-ready')readyResolve();
   if(d?.type==='author-error'){e.error=d.error||'执行失败';error(e.error);for(const w of waiting.values()){clearTimeout(w.timer);w.reject(Error(e.error))}waiting.clear();readyResolve()}
   if(d?.type==='author-rendered'&&waiting.has(d.id)){const w=waiting.get(d.id);clearTimeout(w.timer);waiting.delete(d.id);e.state=d.state;w.resolve()}
  });
  const csp="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; media-src 'none'; base-uri 'none'; form-action 'none';";
  const doc='<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="'+csp+'"><style>'+data.direct_base_css+'</style><script>'+boot+'<'+ '/script>'+sc.html;
  iframe.src='data:text/html;base64,'+btoa(unescape(encodeURIComponent(doc)));document.querySelector('.frame').insertBefore(iframe,document.getElementById('subtitle'));
  return e;
 }
 function render(sc,span,t){
  const key=sc.id+'/'+t;
  const e=entry(sc);for(const [id,x] of frames)x.iframe.hidden=id!==sc.id;
  const scale=document.querySelector('.frame').clientWidth/1280;e.iframe.style.transform='scale('+scale+')';if(key===lastKey)return pending;lastKey=key;
  const time=Math.max(0,t-span.start),duration=span.end-span.start,cues={};
  for(let n of span.sentences){n=(span.subtitles||[]).find(s=>s.id===n.id)||n;cues[n.id]={start:n.start-span.start,end:n.end-span.start,progress:Math.max(0,Math.min(1,(t-n.start)/(n.end-n.start)))};}
  pending=e.ready.then(()=>{
   if(e.error)throw Error(e.error);const id=++sequence;
   return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{e.waiting.delete(id);reject(Error('画面更新超时'))},3000);e.waiting.set(id,{resolve,reject,timer});e.iframe.contentWindow.postMessage({type:'author-frame',id,picture:data.picture?.data_url,frame:{time,duration,progress:Math.max(0,Math.min(1,time/duration)),cues}},'*')});
  });pending.catch(e=>error(e.message));return pending;
 }
 return {render,wait:()=>pending,state:()=>[...frames].map(([id,e])=>({id,state:e.state,error:e.error}))};
})();
