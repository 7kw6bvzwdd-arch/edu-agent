import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const html = `<div id="question" style="position:absolute;font-size:32px;color:#b71c1c;left:50%;transform:translateX(-50%);top:500px;font-weight:bold;opacity:0;">静态标签</div>
<div id="sample" style="position:absolute;left:100px;top:100px;width:200px;height:100px;color:#b71c1c;background:rgba(255,255,255,.8);font-size:32px;opacity:0">位置示意</div>
<script>window.renderFrame=function(frame){const question=document.getElementById('question');const el=document.getElementById('sample');question.style.opacity=frame.progress>.8?1:0;el.style.left=(100+frame.progress*400)+'px';el.style.opacity=frame.progress>.5?1:0};</script>`;
const input = {
  representative: 's1',
  scenes: [{
    id: 's1',
    html,
    frames: [
      {time: 10, duration: 100, progress: 0.1, cues: {}},
      {time: 50, duration: 100, progress: 0.5, cues: {}},
      {time: 100, duration: 100, progress: 1, cues: {}},
    ],
  }],
};
const verify = value => spawnSync(process.execPath, [fileURLToPath(new URL('./verify-authored.mjs', import.meta.url))], {
  input: JSON.stringify(value),
  encoding: 'utf8',
  timeout: 45000,
});
const result = verify(input);

assert.equal(result.status, 0, result.stderr || result.stdout);
assert.deepEqual(JSON.parse(result.stdout), {
  version: 'authored-animation-v1',
  scenes: [{id: 's1', changed: true, samples: 3, reverse_seek: true}],
});

const driftingHTML = html.replace(
  'window.renderFrame=function(frame){',
  'let calls=0;window.renderFrame=function(frame){document.getElementById("question").setAttribute("data-render",String(++calls));',
);
const drift = verify({...input, scenes: [{...input.scenes[0], html: driftingHTML}]});
assert.equal(drift.status, 0, drift.stderr);
assert.deepEqual(JSON.parse(drift.stdout), {error: 'nondeterministic_reverse_seek'});
console.log('authored animation canonicalizes CSSOM and rejects non-style DOM drift');

const geometry = verify({...input, scenes: [{...input.scenes[0], html: `<svg><polygon id="p"/></svg><script>window.renderFrame=f=>document.getElementById('p').setAttribute('points','0,0 NaN,5 50,50')</script>`}]});
assert.equal(geometry.status, 0, geometry.stderr);
assert.deepEqual(JSON.parse(geometry.stdout), {error: 'invalid_geometry'});
console.log('authored animation returns a safe geometry diagnosis');
