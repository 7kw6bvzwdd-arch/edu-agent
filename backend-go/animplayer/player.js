(() => {
'use strict';
// Offline teaching-animation player. Scene state is a pure function of global
// teaching time t (ms); audio only drives t while a sentence with audio plays.
const D = JSON.parse(document.getElementById('anim-data').textContent);
const S = D.script, T = D.timeline, svg = document.getElementById('stage'), NS = svg.namespaceURI;
const W = 1280, H = 720, HEAD = 96, SUB = 112, COLORS = ['#7fb2ff', '#ffb35c', '#7fe0b0', '#f08bd0'];
const $ = id => document.getElementById(id), clamp = (v, a, b) => Math.min(b, Math.max(a, v));
const ease = u => u < .5 ? 2 * u * u : 1 - 2 * (1 - u) * (1 - u);
const num = v => { const r = Math.round(v * 10) / 10; return String(Object.is(r, -0) ? 0 : r).replace('-', '−'); };
const sentences = [], display = {};
T.scenes.forEach(s => s.sentences.forEach(x => sentences.push(x)));
S.scenes.forEach(s => s.narration.forEach(n => { display[n.id] = n.display; }));

function sceneAt(t) { const i = T.scenes.findIndex(s => t < s.end); return i < 0 ? T.scenes.length - 1 : i; }
function sentenceAt(t) { return sentences.find(s => t >= s.start && t < s.end) || null; }
function stateAt(t) {
  const i = sceneAt(t), sc = S.scenes[i], sp = T.scenes[i], st = { scene: i, objects: {}, sentence: sentenceAt(t) };
  for (const o of sc.objects) st.objects[o.id] = { params: { ...(o.params || {}) }, opacity: 1, emph: 0 };
  const order = sc.actions.map((a, j) => [a, sp.actions[j]]).sort((x, y) => x[1].start - y[1].start);
  const seen = {};
  for (const [a] of order) if ((a.op === 'show' || a.op === 'hide') && !seen[a.object]) {
    seen[a.object] = 1; if (st.objects[a.object]) st.objects[a.object].opacity = a.op === 'show' ? 0 : 1;
  }
  for (const [a, s] of order) {
    const o = st.objects[a.object]; if (!o || t < s.start) continue;
    const u = clamp((t - s.start) / Math.max(1, s.end - s.start), 0, 1), e = ease(u);
    if (a.op === 'show' || a.op === 'hide') o.opacity += ((a.op === 'show' ? 1 : 0) - o.opacity) * e;
    else if (a.op === 'tween') o.params[a.param] += (a.to - o.params[a.param]) * e;
    else if (a.op === 'emphasize' && t < s.end) o.emph = Math.max(o.emph, Math.sin(Math.PI * u));
  }
  return st;
}

function el(tag, attrs, parent, text) {
  const e = document.createElementNS(NS, tag);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  if (text != null) e.textContent = text;
  (parent || svg).appendChild(e); return e;
}
function formula(k, b) {
  const kr = Math.round(k * 10) / 10, br = Math.round(b * 10) / 10;
  let s = 'y = ' + (kr === 1 ? '' : kr === -1 ? '−' : num(kr)) + 'x';
  return br ? s + (br > 0 ? ' + ' + num(br) : ' − ' + num(-br)) : s;
}
function wrap(text, max) {
  const out = []; let line = '', w = 0;
  for (const ch of text) { const cw = /[\x00-\xff]/.test(ch) ? .56 : 1; if (w + cw > max && line) { out.push(line); line = ''; w = 0; } line += ch; w += cw; }
  if (line) out.push(line); return out;
}
function drawAxes(o, box, s) {
  const p = s.params, u = Math.min(box.w / (p.xmax - p.xmin), box.h / (p.ymax - p.ymin));
  const w = (p.xmax - p.xmin) * u, h = (p.ymax - p.ymin) * u, x0 = box.x + (box.w - w) / 2, y0 = box.y + (box.h - h) / 2;
  const X = x => x0 + (x - p.xmin) * u, Y = y => y0 + (p.ymax - y) * u, g = el('g', { opacity: s.opacity });
  // Tick labels go in their own group, re-appended above the lines with a halo so lines never hide them.
  const lab = el('g', { opacity: s.opacity, stroke: '#111a2e', 'stroke-width': 5, 'paint-order': 'stroke', 'stroke-linejoin': 'round' });
  const step = Math.max(p.xmax - p.xmin, p.ymax - p.ymin) > 12 ? 2 : 1;
  for (let x = Math.ceil(p.xmin); x <= p.xmax; x++) el('line', { x1: X(x), y1: y0, x2: X(x), y2: y0 + h, stroke: '#21304f' }, g);
  for (let y = Math.ceil(p.ymin); y <= p.ymax; y++) el('line', { x1: x0, y1: Y(y), x2: x0 + w, y2: Y(y), stroke: '#21304f' }, g);
  const ox = clamp(X(0), x0, x0 + w), oy = clamp(Y(0), y0, y0 + h);
  el('line', { x1: x0, y1: oy, x2: x0 + w, y2: oy, stroke: '#8d9aba', 'stroke-width': 2 }, g);
  el('line', { x1: ox, y1: y0, x2: ox, y2: y0 + h, stroke: '#8d9aba', 'stroke-width': 2 }, g);
  for (let x = Math.ceil(p.xmin / step) * step; x <= p.xmax; x += step) if (x) el('text', { x: X(x), y: Math.min(oy + 22, y0 + h - 4), 'font-size': 16, 'text-anchor': 'middle', fill: '#8d9aba' }, lab, num(x));
  for (let y = Math.ceil(p.ymin / step) * step; y <= p.ymax; y += step) if (y) el('text', { x: Math.max(ox - 10, x0 + 18), y: Y(y) + 6, 'font-size': 16, 'text-anchor': 'end', fill: '#8d9aba' }, lab, num(y));
  el('text', { x: x0 + w - 6, y: oy - 10, 'font-size': 20, 'text-anchor': 'end', fill: '#c7d0e6', 'font-style': 'italic' }, g, 'x');
  el('text', { x: ox + 12, y: y0 + 22, 'font-size': 20, fill: '#c7d0e6', 'font-style': 'italic' }, g, 'y');
  el('rect', { x: x0, y: y0, width: w, height: h }, el('clipPath', { id: 'plot' }));
  return { X, Y, p, lab };
}
function drawSide(objs, box, st, colorOf) {
  let y = box.y + 8;
  for (const o of objs) {
    const s = st.objects[o.id], g = el('g', { opacity: s.opacity }); let lines, size;
    if (o.type === 'equation') { const l = st.objects[o.line].params; lines = [formula(l.k, l.b)]; size = 40; }
    else if (o.type === 'text') { size = 28; lines = wrap(o.text, Math.floor((box.w - 28) / size)); }
    else { size = 26; lines = []; o.items.forEach(it => wrap('• ' + it, Math.floor((box.w - 28) / size)).forEach(x => lines.push(x))); }
    const height = size * 1.3 * lines.length + 22;
    if (s.emph > 0) el('rect', { x: box.x, y, width: box.w, height, rx: 10, fill: '#2a2a14', stroke: '#ffd36b', 'stroke-width': 3, opacity: s.emph }, g);
    lines.forEach((tx, i) => el('text', { x: box.x + 14, y: y + 11 + size * (1.3 * i + 1), 'font-size': size, fill: o.type === 'equation' ? colorOf[o.line] : '#e8ecf6', 'data-box': 'side' }, g, tx));
    y += height + 16;
  }
}
function render(t) {
  const st = stateAt(t), sc = S.scenes[st.scene]; svg.textContent = '';
  el('title', {}, svg, sc.title);
  el('rect', { x: 0, y: 0, width: W, height: H, fill: '#111a2e' });
  el('text', { x: 48, y: 62, 'font-size': 34, 'font-weight': 600, fill: '#f2f5fc', 'data-box': 'head' }, svg, sc.title);
  const main = sc.objects.filter(o => o.slot === 'main'), side = sc.objects.filter(o => o.slot === 'side');
  const area = { x: 40, y: HEAD, w: W - 80, h: H - HEAD - SUB }; let mainBox = area, sideBox = null;
  if (main.length && side.length) { mainBox = { ...area, w: area.w * .64 }; sideBox = { x: area.x + area.w * .67, y: area.y, w: area.w * .33, h: area.h }; }
  else if (!main.length) sideBox = { x: area.x + area.w * .2, y: area.y + 40, w: area.w * .6, h: area.h - 40 };
  svg.setAttribute('data-side', sideBox ? [sideBox.x, sideBox.y, sideBox.w, sideBox.h].join(',') : '');
  const colorOf = {}; main.filter(o => o.type === 'line').forEach((o, i) => { colorOf[o.id] = COLORS[i % COLORS.length]; });
  const ax = main.find(o => o.type === 'axes'), m = ax && drawAxes(ax, mainBox, st.objects[ax.id]);
  if (m) for (const o of main) {
    const s = st.objects[o.id];
    if (o.type === 'line') {
      const { k, b } = s.params;
      el('line', { x1: m.X(m.p.xmin), y1: m.Y(k * m.p.xmin + b), x2: m.X(m.p.xmax), y2: m.Y(k * m.p.xmax + b), stroke: colorOf[o.id], 'stroke-width': 4 + 4 * s.emph, 'stroke-linecap': 'round', 'clip-path': 'url(#plot)', opacity: s.opacity, 'data-line': o.id, 'data-k': k.toFixed(3) });
    } else if (o.type === 'point') {
      const l = st.objects[o.line], x = s.params.x, y = l.params.k * x + l.params.b, op = Math.min(s.opacity, l.opacity);
      if (y < m.p.ymin || y > m.p.ymax) continue;
      el('circle', { cx: m.X(x), cy: m.Y(y), r: 7 + 5 * s.emph, fill: '#ffffff', stroke: '#111a2e', 'stroke-width': 2, opacity: op });
      if (o.text) el('text', { x: m.X(x) + 14, y: m.Y(y) - 12, 'font-size': 20, fill: '#f2f5fc', opacity: op }, svg, o.text);
    }
  }
  if (m) svg.appendChild(m.lab);
  if (sideBox) drawSide(side, sideBox, st, colorOf);
  return st;
}
// Controls. Auto play: await holds last their fixed ms. Step mode: time freezes at each await until 继续.
const P = { t: 0, playing: false, step: false, waiting: null, subs: true, last: 0, audio: null, audioFor: null, stall: 0 };
const awaits = []; T.scenes.forEach(s => s.holds.forEach(h => { if (h.type === 'await') awaits.push(h); }));
const fmt = ms => { const s = Math.floor(ms / 1000); return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0'); };
function stopAudio() { if (P.audio) P.audio.pause(); P.audio = P.audioFor = null; }
function syncAudio(sen) {
  // Inside a sentence with audio, its playback position defines t; elsewhere the monotonic clock does.
  if (!P.playing || !sen || !D.audio[sen.id]) { stopAudio(); return false; }
  if (P.audioFor !== sen.id) {
    stopAudio(); const a = new Audio(D.audio[sen.id]); P.audio = a; P.audioFor = sen.id; P.stall = 0;
    a.currentTime = Math.max(0, (P.t - sen.start) / 1000);
    a.addEventListener('ended', () => { if (P.audioFor === sen.id) { P.t = Math.max(P.t, sen.end); stopAudio(); } });
    a.play().catch(() => { if (P.audioFor === sen.id) { D.audio[sen.id] = null; stopAudio(); } });
  }
  return true;
}
function frame(now) {
  const dt = Math.min(250, now - P.last); P.last = now;
  if (P.playing && !P.waiting) {
    const sen = sentenceAt(P.t);
    if (syncAudio(sen) && P.audio) {
      const at = sen.start + P.audio.currentTime * 1000;
      // Audio drives time; if it stalls (buffering/blocked) the clock takes over after 400 ms.
      if (at > P.t) { P.t = at; P.stall = 0; } else if ((P.stall += dt) > 400) P.t += dt;
    } else P.t += dt;
    if (P.step) {
      const h = awaits.find(h => P.t >= h.start && P.t - dt < h.start + 1);
      if (h) { P.t = h.start; P.waiting = h; stopAudio(); }
    }
    if (P.t >= T.total_ms) { P.t = T.total_ms; P.playing = false; stopAudio(); }
  }
  draw(); requestAnimationFrame(frame);
}
function draw() {
  const st = render(Math.min(P.t, T.total_ms - 1)), sub = $('subtitle');
  sub.textContent = ''; const sen = P.t < T.total_ms ? sentenceAt(P.t) : null;
  if (sen) { const span = document.createElement('span'); span.textContent = display[sen.id]; sub.appendChild(span); }
  sub.classList.toggle('off', !P.subs);
  $('seek').value = String(Math.round(P.t)); $('seek').setAttribute('aria-valuetext', fmt(P.t) + '，第' + (st.scene + 1) + '幕');
  $('time').textContent = fmt(P.t) + ' / ' + fmt(T.total_ms);
  $('play').textContent = P.playing ? '暂停' : '播放'; $('play').setAttribute('aria-pressed', String(P.playing));
  $('continue').hidden = !P.waiting; $('scene').textContent = '第 ' + (st.scene + 1) + ' / ' + T.scenes.length + ' 幕';
}
function seek(t) { stopAudio(); P.waiting = null; P.t = clamp(t, 0, T.total_ms); draw(); }
function toggle() {
  if (P.waiting) return resume();
  if (!P.playing && P.t >= T.total_ms) P.t = 0;
  P.playing = !P.playing; if (!P.playing) stopAudio(); draw();
}
function resume() { if (!P.waiting) return; P.t = P.waiting.end; P.waiting = null; P.playing = true; draw(); }
function goScene(d) { const i = clamp(sceneAt(P.t) + d, 0, T.scenes.length - 1); seek(T.scenes[i].start); }
$('seek').max = String(T.total_ms);
$('seek').addEventListener('input', e => seek(Number(e.target.value)));
$('play').addEventListener('click', toggle);
$('continue').addEventListener('click', resume);
$('prev').addEventListener('click', () => goScene(-1));
$('next').addEventListener('click', () => goScene(1));
$('replay').addEventListener('click', () => { seek(0); P.playing = true; draw(); });
$('mode').addEventListener('click', () => { P.step = !P.step; if (!P.step && P.waiting) resume(); $('mode').setAttribute('aria-pressed', String(P.step)); });
$('subs').addEventListener('click', () => { P.subs = !P.subs; $('subs').setAttribute('aria-pressed', String(P.subs)); draw(); });
document.addEventListener('keydown', e => {
  if (e.target.closest && e.target.closest('button,input')) return;
  if (e.key === ' ') { e.preventDefault(); toggle(); }
  else if (e.key === 'ArrowLeft') seek(P.t - 5000); else if (e.key === 'ArrowRight') seek(P.t + 5000);
});
window.__anim = { stateAt, render, sentenceAt, seek, toggle, resume, timeline: T, player: P, step: v => { P.step = v; $('mode').setAttribute('aria-pressed', String(v)); } };
draw(); P.last = performance.now(); requestAnimationFrame(frame);
})();
