import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import assert from 'node:assert/strict';
import puppeteer from 'puppeteer-core';
const target=path.resolve(process.argv[2]||'work/lesson-video/fixture.mp4'),dir=path.dirname(target),video=await fs.readFile(target);
const report=JSON.parse(await fs.readFile(target+'.json','utf8'));
const server=http.createServer((req,res)=>{if(req.url==='/video.mp4'){res.setHeader('Content-Type','video/mp4');res.setHeader('Accept-Ranges','bytes');const m=req.headers.range?.match(/^bytes=(\d+)-(\d*)$/);if(m){const start=Number(m[1]),end=m[2]?Math.min(Number(m[2]),video.length-1):video.length-1;res.writeHead(206,{'Content-Range':'bytes '+start+'-'+end+'/'+video.length,'Content-Length':end-start+1});res.end(video.subarray(start,end+1))}else{res.setHeader('Content-Length',video.length);res.end(video)}}else{res.setHeader('Content-Type','text/html; charset=utf-8');res.end('<video controls width="1280" height="720" src="/video.mp4"></video>')}});
await new Promise(r=>server.listen(0,'127.0.0.1',r));let browser;
try{
 browser=await puppeteer.launch({executablePath:process.env.TEST_BROWSER_PATH||(process.platform==='win32'?'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe':'/usr/bin/google-chrome'),headless:true,args:['--autoplay-policy=no-user-gesture-required'],defaultViewport:{width:1280,height:720}});
 const page=await browser.newPage();await page.goto('http://127.0.0.1:'+server.address().port);await page.waitForFunction(()=>document.querySelector('video').readyState>=2);
 const samples=[];
 for(const seconds of [0.25,report.duration*.3,report.duration*.7,report.duration-.5]){
  await page.evaluate(async time=>{const v=document.querySelector('video');const done=new Promise(r=>v.addEventListener('seeked',r,{once:true}));v.currentTime=time;await done;},seconds);
  const sample=await page.evaluate(()=>{const v=document.querySelector('video');return {time:v.currentTime,width:v.videoWidth,height:v.videoHeight,error:v.error?.code||null,decoded:v.getVideoPlaybackQuality().totalVideoFrames}});
  assert.equal(sample.width,1280);assert.equal(sample.height,720);assert.equal(sample.error,null);assert.ok(sample.decoded>0);assert.ok(Math.abs(sample.time-seconds)<0.1,'seek reached requested time');samples.push(sample);
  await page.screenshot({path:path.join(dir,'mp4-playback-'+seconds+'.png')});
 }
 await page.evaluate(async()=>{const v=document.querySelector('video');v.currentTime=0;await v.play()});await new Promise(r=>setTimeout(r,1500));
 assert.ok(await page.evaluate(()=>document.querySelector('video').currentTime>0.5));
 await fs.writeFile(path.join(dir,'playback-report.json'),JSON.stringify({browser:await browser.version(),samples,plays:true,scope:'real MP4 decode and seek; audio stream present via ffprobe; no subjective listening claim'},null,2));
 console.log('Real MP4 playback and four seek samples passed');
}finally{if(browser)await browser.close();await new Promise(r=>server.close(r))}
