import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
import {createCaptureSession,initializeSession,captureFrameToBuffer,closeCaptureSession,spawnStreamingEncoder,processCompositionAudio} from '@hyperframes/engine';
const require=createRequire(import.meta.url);
if(process.argv.length!==4)throw Error('Usage: node export.mjs <trusted-package.html> <out.mp4>');
const source=path.resolve(process.argv[2]),target=path.resolve(process.argv[3]);
await fs.mkdir(path.dirname(target),{recursive:true});
const out=await fs.mkdtemp(path.join(path.dirname(target),'.capture-'));
process.env.HYPERFRAMES_FFMPEG_PATH=require('ffmpeg-static');
process.env.HYPERFRAMES_FFPROBE_PATH=require('ffprobe-static').path;
process.env.DO_NOT_TRACK='1';

let html=(await fs.readFile(source,'utf8')).replace(/\r\n/g,'\n');
const data=JSON.parse(html.match(/<script type="application\/json" id="anim-data">([\s\S]*?)<\/script>/)[1]);
const duration=data.timeline.total_ms/1000,fps=24,width=1280,height=720;
assert.ok(duration>=30&&duration<=180,'export supports 30–180 seconds');
assert.ok(['func.params@1','geo.transform@1','lesson.explain@1'].includes(data.script.archetype));
const hash=s=>createHash('sha256').update(s).digest('base64');
// Adapt only a test copy of the trusted renderer; preserve its CSP hashes.
const bridge='\nwindow.__anim.player.playing=false;window.__anim.step(false);window.__hf={duration:'+duration+',seek:function(seconds){window.__anim.player.playing=false;window.__anim.seek(seconds*1000)}};\n';
const tag=[...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].at(-1);assert.ok(tag);
const previous=tag[1],next=previous+bridge;
html=html.replace(tag[0],'<script>'+next+'</script>').replace('sha256-'+hash(previous),'sha256-'+hash(next));
const css=html.match(/<style>([\s\S]*?)<\/style>/)[1];
const marking=data.labels.map(l=>l.text).join(' · ');
const exportCSS=css+'\nheader,nav{display:none}main{padding:0}.frame{width:100vw;height:100vh;aspect-ratio:auto}#stage{border-radius:0}.frame::after{content:"'+marking.replace(/["\\]/g,'')+'";position:absolute;top:8px;right:12px;color:#28445e;background:rgba(255,255,255,.92);padding:4px 8px;border-radius:5px;font-size:14px}';
html=html.replace('<style>'+css+'</style>','<style>'+exportCSS+'</style>').replace('sha256-'+hash(css),'sha256-'+hash(exportCSS));
await fs.writeFile(path.join(out,'capture.html'),html);await fs.mkdir(path.join(out,'audio'),{recursive:true});
const audio=[];
for(const scene of data.timeline.scenes)for(const sen of scene.sentences){
 const encoded=data.audio[sen.id];assert.ok(encoded,'every narration has real source audio');
 assert.match(sen.id,/^[a-z0-9][a-z0-9_-]{0,31}$/);
 assert.match(encoded,/^data:audio\/(mpeg|wav);base64,[A-Za-z0-9+/=]+$/);
 const filename='audio/'+sen.id+(encoded.startsWith('data:audio/wav')?'.wav':'.mp3');await fs.writeFile(path.join(out,filename),Buffer.from(encoded.split(',')[1],'base64'));
 audio.push({id:sen.id,src:filename,start:sen.start/1000,end:sen.end/1000,mediaStart:0,layer:audio.length,volume:1,type:'audio'});
}
assert.equal(audio.length,data.timeline.scenes.reduce((n,s)=>n+s.sentences.length,0));
const server=http.createServer((req,res)=>{if(new URL(req.url,'http://localhost').pathname!='/index.html'){res.writeHead(404);return res.end()}res.setHeader('Content-Type','text/html; charset=utf-8');res.end(html)});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
let session,encoder;const checks=[],started=performance.now();
try{
 const config={chromePath:process.env.ANIMATION_CHROME_PATH||(process.platform==='win32'?'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe':undefined),forceScreenshot:true,useDrawElement:false,staticFrameDedup:false,enableBrowserPool:false,browserGpuMode:'software',protocolTimeout:60000,playerReadyTimeout:10000};
 session=await createCaptureSession('http://127.0.0.1:'+server.address().port,path.join(out,'frames'),{width,height,fps:{num:fps,den:1},format:'png',compositionDurationSeconds:duration},null,config);
 await session.page.setRequestInterception(true);const external=[];session.page.on('request',req=>{if(req.url().startsWith('data:')||req.url().startsWith('http://127.0.0.1:'+server.address().port+'/'))req.continue();else{external.push(req.url());req.abort()}});
 const errors=[];session.page.on('pageerror',e=>{errors.push(e.message);console.log('PAGE ERROR '+e.message)});session.page.on('console',m=>{if(m.type()==='error')console.log('PAGE CONSOLE '+m.text())});await initializeSession(session);

 const state=()=>session.page.evaluate(()=>({svg:document.getElementById('stage').outerHTML,subtitle:document.getElementById('subtitle').textContent}));
 const saved=new Map();
 for(const t of [0,duration*.2,duration*.5,duration*.8,duration-0.05,duration*.5,duration*.2,0]){
  const frame=await captureFrameToBuffer(session,Math.floor(t*fps),t),value=JSON.stringify(await state());
  if(saved.has(t))assert.equal(value,saved.get(t),'backward seek repeats state');saved.set(t,value);
  await fs.writeFile(path.join(out,'checkpoint-'+String(t).replace('.','_')+'.png'),frame.buffer);
 }
 checks.push('首中末和随机倒退定位状态一致');
 for(const scene of data.timeline.scenes){await captureFrameToBuffer(session,0,scene.start/1000);const box=await session.page.evaluate(()=>{const r=document.querySelector('.frame').getBoundingClientRect();return {w:r.width,h:r.height,controls:getComputedStyle(document.querySelector('nav')).display}});assert.equal(box.w,width);assert.equal(box.h,height);assert.equal(box.controls,'none')}
 checks.push('全部分镜1280×720，播放控件不入画');
 const subtitle=data.timeline.scenes.flatMap(s=>s.subtitles||s.sentences).find(s=>s.end>s.start);assert.ok(subtitle);
 await captureFrameToBuffer(session,0,(subtitle.start+subtitle.end)/2000);assert.ok((await state()).subtitle);checks.push('源字幕随原时间轴烧录');
 encoder=await spawnStreamingEncoder(path.join(out,'video-only.mp4'),{fps:{num:fps,den:1},width,height,imageFormat:'png',codec:'h264',preset:'ultrafast',quality:23,useGpu:false});
 const count=Math.ceil(duration*fps);
 for(let i=0;i<count;i++){const frame=await captureFrameToBuffer(session,i,i/fps);assert.ok(await encoder.writeFrame(frame.buffer),encoder.getExitError());if(i%240===0)console.log('capture '+i+'/'+count)}
 const encoded=await encoder.close();encoder=null;assert.ok(encoded.success,encoded.error);
 const mixed=await processCompositionAudio(audio,out,path.join(out,'mix-work'),path.join(out,'audio.m4a'),duration);assert.ok(mixed.success,JSON.stringify(mixed));
 execFileSync(process.env.HYPERFRAMES_FFMPEG_PATH,['-y','-v','error','-i',path.join(out,'video-only.mp4'),'-i',path.join(out,'audio.m4a'),'-map','0:v:0','-map','1:a:0','-c','copy','-t',String(duration),'-metadata','comment=AI-assisted teaching content; AI synthesized or explicitly labelled test audio','-movflags','+faststart+use_metadata_tags',target],{windowsHide:true,timeout:120000});
 const probe=JSON.parse(execFileSync(process.env.HYPERFRAMES_FFPROBE_PATH,['-v','quiet','-show_streams','-show_format','-of','json',target],{encoding:'utf8',windowsHide:true}));
 const video=probe.streams.find(s=>s.codec_type==='video'),sound=probe.streams.find(s=>s.codec_type==='audio');
 assert.equal(video.width,width);assert.equal(video.height,height);assert.equal(video.nb_frames,String(count));assert.ok(sound);assert.ok(Math.abs(Number(probe.format.duration)-duration)<0.1);assert.equal(errors.length,0);// Injected browser extensions may attempt network; every such request is blocked.
 checks.push('完整时间轴、H264+AAC、全部原音轨，外部资源被阻止');
 await fs.writeFile(target+'.json',JSON.stringify({upstreamCommit:'8be76fc86cda5b2a77d0ee23b65f582e85376042',packageVersion:'0.8.163',source,sourceHash:hash(await fs.readFile(source)),duration,fps,width,height,frames:count,audioTracks:audio.length,captureMode:session.captureMode,elapsedSeconds:(performance.now()-started)/1000,checks,errors,external,video:{codec:video.codec_name,frames:video.nb_frames},audio:{codec:sound.codec_name},bytes:Number(probe.format.size)},null,2));
 console.log('probe passed: '+checks.join('; '));
}finally{if(encoder)await encoder.close();if(session)await closeCaptureSession(session);await new Promise(r=>server.close(r));await fs.rm(out,{recursive:true,force:true})}
