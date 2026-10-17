// Node 22+; run against the isolated sample server: node scripts/browser-workspace.cjs
// EDU_TEST_URL defaults to :8081; EDU_TEST_USERNAME / EDU_TEST_PASSWORD default to
// admin / test-pass-123. Never reads .env or starts an application server.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const {spawn, spawnSync} = require('node:child_process');
const {randomUUID} = require('node:crypto');

const root = path.resolve(__dirname, '..');
const output = path.join(root, 'work');
const base = new URL(process.env.EDU_TEST_URL || 'http://127.0.0.1:8081').origin;
const port = 9240;
const runID = 'workspace-' + randomUUID().slice(0, 12);
const topics = new Set();
const owned = new Set();
const deleted = new Set();
const checks = [];
const runtimeErrors = [];
const cleanupErrors = [];
const pending = new Map();
const downloads = new Map();
const paused = new Map();
const terminal = ['succeeded', 'degraded', 'failed', 'cancelled'];
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const quote = JSON.stringify;
let browser, socket, profile, downloadDir, token = '', sequence = 0, failure;

async function request(route, method = 'GET', body, allowMissing = false) {
  const response = await fetch(base + '/api' + route, {
    method, redirect: 'error', signal: AbortSignal.timeout(8000),
    headers: {...(token ? {Authorization: 'Bearer ' + token} : {}), 'Content-Type': 'application/json'},
    ...(body === undefined ? {} : {body: JSON.stringify(body)}),
  });
  if (allowMissing && response.status === 404) return null;
  assert.ok(response.ok, `${method} ${route}: HTTP ${response.status}`);
  return response.status === 204 ? null : response.json();
}

async function sampleHealth() {
  const health = await request('/health');
  assert.equal(health.mode, 'sample', 'Refusing a non-sample server: no generation calls were authorized');
  assert.equal(health.status, 'ok', 'Sample server health');
  return health;
}

async function waitFor(name, test, timeout = 30000) {
  const deadline = Date.now() + timeout;
  do {
    const result = await test();
    if (result) return result;
    await sleep(150);
  } while (Date.now() < deadline);
  throw Error('Timed out: ' + name);
}

function passed(name) { checks.push(name); console.log('PASS ' + name); }
function call(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(Error('CDP timeout: ' + method)); }, 10000);
    pending.set(id, {resolve, reject, timer});
    try { socket.send(JSON.stringify({id, method, params})); }
    catch (e) { clearTimeout(timer); pending.delete(id); reject(e); }
  });
}
async function evaluate(expression) {
  const result = await call('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true});
  if (result.exceptionDetails) throw Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
  return result.result.value;
}
async function check(name, expression) { assert.equal(await evaluate(expression), true, name); passed(name); }
async function uiWait(name, expression, timeout = 30000, page) {
  return waitFor(name, async () => {
    const state = await evaluate(`({ok: !!(${expression}), error: document.querySelector('.alert.error')?.textContent,
      page: document.querySelector('nav [aria-current="page"]')?.textContent})`);
    assert.ok(!state.error, `${name}: ${state.error}`);
    if (page) assert.ok(state.page?.includes(page), `${name}: left ${page}`);
    return state.ok;
  }, timeout);
}
async function click(text, scope = 'document') {
  await evaluate(`(() => {
    const button = [...${scope}.querySelectorAll('button')].find(b => b.textContent.includes(${quote(text)}));
    if (!button || button.disabled) throw Error('Missing or disabled button: ' + ${quote(text)});
    button.focus(); button.click();
  })()`);
}
async function setValue(selector, value) {
  await evaluate(`(() => {
    const field = document.querySelector(${quote(selector)});
    if (!field || field.disabled || field.readOnly) throw Error('Missing or disabled field: ' + ${quote(selector)});
    field.value = ${quote(value)};
    field.dispatchEvent(new Event(field.tagName === 'SELECT' ? 'change' : 'input', {bubbles:true}));
  })()`);
}
async function screenshot(filename) {
  const shot = await call('Page.captureScreenshot', {format: 'png', captureBeyondViewport: false});
  await fs.writeFile(path.join(output, filename), Buffer.from(shot.data, 'base64'));
}
async function key(key, modifiers = 0) {
  const code = key === 'Tab' ? 9 : 27;
  await call('Input.dispatchKeyEvent', {type: 'keyDown', key, code: key, windowsVirtualKeyCode: code, modifiers});
  await call('Input.dispatchKeyEvent', {type: 'keyUp', key, code: key, windowsVirtualKeyCode: code, modifiers});
}

async function create(kind) {
  await sampleHealth();
  const topic = runID + '-' + kind;
  topics.add(topic);
  await setValue('.config-panel input[maxlength="100"]', topic);
  await evaluate(`(() => {
    const inputs = document.querySelectorAll('.check-card input');
    if (!inputs[0].checked) inputs[0].click();
    if (inputs[1].checked) inputs[1].click();
  })()`);
  // Pause only this outbound POST: prove feedback is visible before the server
  // can reply, then resume the real request and let the app poll/SSE on its own.
  await call('Fetch.enable', {patterns: [{urlPattern: base + '/api/tasks', requestStage: 'Request'}]});
  try {
    await click('开始研究', "document.querySelector('.config-panel')");
    const post = await waitFor('create request', () => [...paused.values()].find(p => p.request.method === 'POST'), 10000);
    assert.equal(JSON.parse(post.request.postData).topic, topic);
    assert.equal(JSON.parse(post.request.postData).use_web, false);
    assert.equal(JSON.parse(post.request.postData).animation_engine, undefined,
      'Animation engine is explicit only on animation requests');
    passed(kind + ': correct animation engine request field');
    await uiWait('immediate progress before POST response', `(() => {
      const p = document.querySelector('.generation-progress.working');
      if (!p) return false;
      const r = p.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && r.top < innerHeight && r.bottom > 0 &&
        document.querySelector('.output-panel').getAttribute('aria-busy') === 'true';
    })()`, 5000, kind === 'animation' ? '教学动画' : '课件工作台');
    passed(kind + ': immediate visible progress before response');
    await call('Fetch.continueRequest', {requestId: post.requestId});
    paused.delete(post.requestId);
  } finally {
    for (const requestId of paused.keys()) await call('Fetch.failRequest', {requestId, errorReason: 'Aborted'});
    paused.clear();
    await call('Fetch.disable');
  }
  const page = kind === 'animation' ? '教学动画' : '课件工作台';
  await uiWait(kind + ' automatically awaits confirmation', "!!document.querySelector('.output-head .status.awaiting_confirmation')", 90000, page);
  const rows = await request('/tasks?q=' + encodeURIComponent(topic));
  const matches = rows.filter(t => t.input.topic === topic);
  assert.equal(matches.length, 1, 'Exactly one test-owned creation');
  owned.add(matches[0].id);
  const task = await request('/tasks/' + matches[0].id);
  assert.equal(task.mode, 'sample');
  assert.equal(task.type, kind);
  assert.equal(task.status, 'awaiting_confirmation');
  passed(kind + ': automatic awaiting_confirmation without navigation');
  return task;
}

async function downloadPPTX() {
  const before = new Set(downloads.keys());
  await click('下载 PPTX');
  const complete = await waitFor('real PPTX download', () => [...downloads.values()].find(d => !before.has(d.guid) && d.state === 'completed'));
  const file = await fs.readFile(path.join(downloadDir, complete.guid));
  assert.ok(file.length > 1000 && file.subarray(0, 2).toString() === 'PK', 'Downloaded a nonempty PPTX ZIP');
  passed('PPTX download completed (' + file.length + ' bytes)');
  return file;
}

async function searchWorks(query) {
  await setValue('.works-search input', query);
  await click('查询', "document.querySelector('.works-toolbar')");
  await uiWait('works query complete', "document.querySelector('.works-toolbar button.primary')?.textContent.trim() === '查询' && !document.querySelector('.works-toolbar button.primary')?.disabled");
}
const card = id => `document.querySelector('[data-task-id="${id}"]')`;
async function dialogFocus(name) {
  await uiWait(name + ' focus', "!!document.querySelector('.work-dialog')?.contains(document.activeElement)");
  await check(name + ' accessible dialog', "document.querySelector('.work-dialog').getAttribute('role') === 'dialog' && document.querySelector('.work-dialog').getAttribute('aria-modal') === 'true'");
  await evaluate("document.querySelector('.work-dialog input,.work-dialog button').focus()");
  await key('Tab', 8);
  await check(name + ' Shift+Tab stays in dialog', "document.activeElement === [...document.querySelectorAll('.work-dialog button')].at(-1)");
  await key('Tab');
  await check(name + ' Tab wraps to first control', "document.activeElement === document.querySelector('.work-dialog input,.work-dialog button')");
}

async function deleteThroughDialog(id) {
  assert.ok(owned.has(id), 'May only delete this run\'s created IDs');
  await click('删除', card(id));
  await dialogFocus('Delete');
  await click('保留作品', "document.querySelector('.work-dialog')");
  await uiWait('delete dismissed', "!document.querySelector('.work-dialog')");
  assert.ok(await request('/tasks/' + id), 'Dismiss keeps owned task');
  await click('删除', card(id));
  await uiWait('delete dialog reopened', "!!document.querySelector('.work-dialog')");
  await click('确认删除', "document.querySelector('.work-dialog')");
  await uiWait('owned item removed', `!document.querySelector('.work-dialog') && !${card(id)}`);
  assert.equal(await request('/tasks/' + id, 'GET', undefined, true), null);
  deleted.add(id);
  passed('Owner deletes only test-created item via dialog');
}

async function main() {
  await fs.mkdir(output, {recursive: true});
  const serverTimeout = Number(process.env.EDU_TEST_SERVER_TIMEOUT_MS || 180000);
  assert.ok(Number.isFinite(serverTimeout) && serverTimeout > 0 && serverTimeout <= 600000, 'Server wait must be 1–600000 ms');
  console.log('Waiting for isolated sample server ' + base + ' (max ' + serverTimeout + ' ms)');
  await waitFor('sample server readiness at ' + base, async () => {
    try {
      const health = await request('/health');
      assert.equal(health.mode, 'sample', 'Refusing non-sample health; no paid calls');
      return health.status === 'ok';
    } catch (e) {
      if (e instanceof assert.AssertionError) throw e;
      return false;
    }
  }, serverTimeout);
  passed('Health explicitly reports sample mode');
  // Do not connect to another developer's debugging session.
  await new Promise((resolve, reject) => {
    const probe = net.createServer();
    probe.once('error', reject);
    probe.listen(port, '127.0.0.1', () => probe.close(resolve));
  });
  profile = await fs.mkdtemp(path.join(os.tmpdir(), 'edu-workspace-9240-'));
  downloadDir = path.join(profile, 'downloads');
  await fs.mkdir(downloadDir);
  browser = spawn(process.env.EDU_TEST_EDGE || 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', [
    '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
    '--disable-extensions', '--disable-background-networking', '--disable-sync',
    '--remote-debugging-port=' + port, '--remote-debugging-address=127.0.0.1',
    '--user-data-dir=' + profile, 'about:blank',
  ], {windowsHide: true, stdio: 'ignore'});
  let launchError;
  browser.on('error', e => { launchError = e; });
  const target = await waitFor('isolated Edge debugging endpoint', async () => {
    if (launchError) throw launchError;
    assert.equal(browser.exitCode, null, 'Edge exited before debugger startup');
    try {
      const pages = await fetch(`http://127.0.0.1:${port}/json/list`, {signal: AbortSignal.timeout(1000)}).then(r => r.json());
      return pages.find(p => p.type === 'page');
    } catch { return null; }
  }, 15000);
  socket = new WebSocket(target.webSocketDebuggerUrl);
  socket.onmessage = event => {
    const m = JSON.parse(event.data);
    if (m.id && pending.has(m.id)) {
      const p = pending.get(m.id); pending.delete(m.id); clearTimeout(p.timer);
      m.error ? p.reject(Error(JSON.stringify(m.error))) : p.resolve(m.result);
    }
    if (m.method === 'Runtime.exceptionThrown') runtimeErrors.push(m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text);
    if (m.method === 'Fetch.requestPaused') paused.set(m.params.requestId, m.params);
    if (m.method === 'Browser.downloadWillBegin' || m.method === 'Browser.downloadProgress') {
      downloads.set(m.params.guid, {...downloads.get(m.params.guid), ...m.params});
    }
  };
  await waitFor('CDP websocket open', () => socket.readyState === WebSocket.OPEN, 10000);
  await call('Runtime.enable');
  await call('Page.enable');
  await call('Browser.setDownloadBehavior', {behavior: 'allowAndName', downloadPath: downloadDir, eventsEnabled: true});
  await call('Emulation.setDeviceMetricsOverride', {width: 1440, height: 1080, deviceScaleFactor: 1, mobile: false});
  await call('Page.navigate', {url: base});
  await uiWait('login form', "!!document.querySelector('.login-form')");
  await setValue('input[autocomplete="username"]', process.env.EDU_TEST_USERNAME || 'admin');
  await setValue('input[type="password"]', process.env.EDU_TEST_PASSWORD || 'test-pass-123');
  await click('登录工作台');
  await uiWait('authenticated workbench', "!!document.querySelector('.workbench')");
  token = await evaluate("sessionStorage.getItem('edu_token')");
  assert.ok(token, 'Authenticated owner session');
  await check('Grade explains retrieval, difficulty, prerequisites and textbook verification', `(() => {
    const select = document.querySelector('select[aria-describedby="grade-help"]');
    const text = document.getElementById('grade-help')?.textContent || '';
    return !!select && ['检索范围','难度','前置知识','教材','核验'].every(s => text.includes(s));
  })()`);
  await check('Courseware has its independent assistant explanation', "document.querySelector('.assistant-note')?.textContent.includes('课件助手') && document.querySelector('.assistant-note').textContent.includes('独立')");
  await check('Courseware does not show animation engine selector', "!document.querySelector('#animation-engine')");

  await check('Six distinct template cards', "document.querySelectorAll('.themes .theme').length === 6 && new Set([...document.querySelectorAll('.themes input')].map(x => x.value)).size === 6");
  // Select by its page-count options, independent of neighboring subject fields.
  await evaluate("[...document.querySelectorAll('.config-panel select')].find(s => [...s.options].some(o => o.value === '20')).setAttribute('data-workspace-pages', '')");
  await check('12 pages selected by default', "document.querySelector('[data-workspace-pages]').value === '12'");
  await setValue('[data-workspace-pages]', '20');
  await check('20-page option can be selected', "document.querySelector('[data-workspace-pages]').value === '20'");
  await setValue('[data-workspace-pages]', '12');
  await check('Desktop workspace has no horizontal overflow', 'innerWidth === 1440 && document.documentElement.scrollWidth <= innerWidth');

  const courseware = await create('courseware');
  assert.equal(courseware.result.slides.length, 12);
  await check('12 editable slides appear automatically', "document.querySelectorAll('.outline-slide').length === 12");
  await check('Unverified sample needs explicit agreement', "!!document.querySelector('.consent input') && [...document.querySelectorAll('button')].find(b => b.textContent.includes('确认方案，开始制作')).disabled");
  await evaluate("document.querySelector('.consent input').click()");
  await click('确认方案，开始制作');
  await uiWait('automatic completed courseware', "!!document.querySelector('.output-head .status.succeeded')", 120000, '课件工作台');
  passed('Agreement and confirmation automatically complete in workbench');
  const original = await downloadPPTX();
  const before = await request('/tasks/' + courseware.id);
  const billing = await request('/billing');
  const notes = '教师备注 ' + runID + '：先提问，再逐步展示答案。';
  await setValue('.outline-slide .slide-edit-options select', 'comparison');
  await evaluate("document.querySelector('.outline-slide details').open = true");
  await setValue('.outline-slide details textarea', notes);
  await click('免费保存并重导出');
  await uiWait('saved revision', "document.querySelector('.alert.success')?.textContent.includes('新修订已保存')", 90000);
  const edited = await request('/tasks/' + courseware.id);
  assert.ok(edited.revision > before.revision, 'Revision increased');
  assert.equal(edited.result.slides[0].layout, 'comparison');
  assert.equal(edited.result.slides[0].notes, notes);
  const afterBilling = await request('/billing');
  assert.equal(afterBilling.balance, billing.balance, 'Manual reexport is free');
  assert.equal(afterBilling.reserved, billing.reserved);
  assert.notDeepEqual(await downloadPPTX(), original, 'Reexport updates PPTX bytes');
  passed('Per-slide layout and notes persisted; free reexport changed downloaded PPTX');
  await click('课件预览');
  await check('Edited layout reaches slide preview', "!!document.querySelector('.slide-preview.layout-comparison')");
  await check('Desktop completed workspace has no horizontal overflow', 'document.documentElement.scrollWidth <= 1440');
  await evaluate('scrollTo(0, 0)');
  await screenshot('new-workspace-desktop.png');
  await call('Emulation.setDeviceMetricsOverride', {width: 390, height: 844, deviceScaleFactor: 1, mobile: true});
  await uiWait('mobile viewport', 'innerWidth === 390');
  await check('Mobile workspace has no horizontal overflow', 'document.documentElement.scrollWidth <= 390');
  await screenshot('new-workspace-mobile.png');

  await click('我的作品', 'document.querySelector("nav")');
  await uiWait('My Works', "!!document.querySelector('.works-toolbar')");
  await searchWorks(runID + '-no-match');
  await check('Search excludes nonmatching works', "document.querySelectorAll('.task-card').length === 0 && document.body.textContent.includes('没有找到匹配的作品')");
  await searchWorks(runID);
  await check('Search finds only this run\'s courseware', `document.querySelectorAll('.task-card').length === 1 && !!${card(courseware.id)}`);
  await check('Mobile My Works has no horizontal overflow', 'document.documentElement.scrollWidth <= 390');
  await click('重命名', card(courseware.id));
  await dialogFocus('Rename');
  await check('Mobile work dialog has no horizontal overflow', "document.documentElement.scrollWidth <= 390 && document.querySelector('.work-dialog').getBoundingClientRect().right <= 390");
  await key('Escape');
  await uiWait('rename Escape closes', "!document.querySelector('.work-dialog')");
  await check('Dialog restores trigger focus', `!!${card(courseware.id)}.contains(document.activeElement) && document.activeElement.textContent.includes('重命名')`);
  await click('重命名', card(courseware.id));
  await uiWait('rename input', "!!document.querySelector('.work-dialog input')");
  const renamed = runID + '-renamed';
  await setValue('.work-dialog input', renamed);
  await click('保存名称', 'document.querySelector(".work-dialog")');
  await uiWait('rename persisted in list', `!document.querySelector('.work-dialog') && ${card(courseware.id)}?.querySelector('h3')?.textContent === ${quote(renamed)}`);
  assert.equal((await request('/tasks/' + courseware.id)).display_title, renamed);
  await searchWorks(renamed);
  await check('Owner can search by renamed title', `document.querySelectorAll('.task-card').length === 1 && !!${card(courseware.id)}`);
  await call('Emulation.setDeviceMetricsOverride', {width: 1440, height: 1080, deviceScaleFactor: 1, mobile: false});
  await uiWait('desktop works viewport', 'innerWidth === 1440');
  await check('Desktop My Works has no horizontal overflow', 'document.documentElement.scrollWidth <= 1440');
  await click('查看 / 编辑', card(courseware.id));
  await uiWait('reopened edited work', "!!document.querySelector('.outline-slide')");
  await check('Reopened work retains layout and notes', `document.querySelector('.outline-slide select').value === 'comparison' && document.querySelector('.outline-slide details textarea').value === ${quote(notes)}`);
  await check('Reopened work restores topic and source choices', `document.querySelector('.config-panel input').value === ${quote(courseware.input.topic)} && document.querySelectorAll('.check-card input')[0].checked && !document.querySelectorAll('.check-card input')[1].checked`);
  await click('版本历史');
  await uiWait('history loaded', "document.querySelectorAll('.history-row').length === 2");
  const oldRow = `document.querySelector('.history-row[data-revision="${before.revision}"]')`;
  await click('查看内容', oldRow);
  await uiWait('historical content', "!!document.querySelector('.history-preview li')");
  await check('Viewing history does not overwrite unsaved editor', `document.querySelector('.outline-slide details textarea').value === ${quote(notes)}`);
  await call('Emulation.setDeviceMetricsOverride', {width:390,height:844,deviceScaleFactor:1,mobile:true});
  await check('History fits narrow screens', 'document.documentElement.scrollWidth <= 390');
  await screenshot('courseware-history-mobile.png');
  await call('Emulation.setDeviceMetricsOverride', {width:1440,height:1080,deviceScaleFactor:1,mobile:false});
  // Simulate the browser's affirmative confirmation; this sample-server test never calls a model.
  await evaluate('window.confirm = () => true');
  await click('恢复此版', oldRow);
  await uiWait('historical restore delivered', "document.querySelector('.alert.success')?.textContent.includes('历史版本已恢复')",90000);
  const restored = await request('/tasks/'+courseware.id);
  assert.equal(restored.result.slides[0].notes,before.result.slides[0].notes);
  assert.equal(restored.revision,edited.revision+1);
  assert.deepEqual(await request('/billing'),afterBilling);
  passed('Restoring a version creates a new revision and leaves billing unchanged');
  await check('Old and new versions both retained', "document.querySelectorAll('.history-row').length === 3");
  await click('查看制作过程');
  await uiWait('restoration audit shown', "document.querySelector('.process-dialog')?.textContent.includes('版本恢复') && document.querySelector('.process-dialog')?.textContent.includes('内容修改')");
  passed('Process tree includes edits and version restoration');
  await key('Escape');
  await uiWait('process closed', "!document.querySelector('.process-dialog')");
  await screenshot('courseware-history-desktop.png');

  await click('我的作品', 'document.querySelector("nav")');
  await uiWait('works ready for new animation', "!!document.querySelector('.collection-create')");
  await click('新建动画', 'document.querySelector(".collection-create")');
  await uiWait('new animation opens workbench', `!!document.querySelector('.animation-studio') && !document.querySelector('#animation-engine')`);
  passed('New animation from My Works opens animation workbench');
  await check('HTML animation requires real provider configuration', "document.querySelector('.anim-brief .primary').disabled");
  await click('我的作品', 'document.querySelector("nav")');
  await uiWait('My Works for deletion', "!!document.querySelector('.works-toolbar')");
  await searchWorks(runID);
  await deleteThroughDialog(courseware.id);
  await check('All test-created works removed from filtered list', "document.querySelectorAll('.task-card').length === 0");
  assert.deepEqual(runtimeErrors, [], 'No browser runtime exceptions');
  passed('No browser runtime exceptions');
}

async function cleanup() {
  // Recover even if the UI failed after creation but before we captured its ID.
  if (token && topics.size) {
    try {
      await sampleHealth();
      const rows = await request('/tasks?q=' + encodeURIComponent(runID));
      for (const row of rows) if (topics.has(row.input?.topic)) owned.add(row.id);
      for (const id of owned) {
        if (deleted.has(id)) continue;
        const task = await request('/tasks/' + id, 'GET', undefined, true);
        if (!task) continue;
        assert.ok(topics.has(task.input.topic) && task.mode === 'sample', 'Cleanup ownership guard');
        if (!terminal.includes(task.status)) await request('/tasks/' + id + '/cancel', 'POST', {});
        await request('/tasks/' + id, 'DELETE');
        deleted.add(id);
      }
    } catch (e) { cleanupErrors.push('Task cleanup: ' + e.message); }
  }
  try { if (socket?.readyState === WebSocket.OPEN) await call('Browser.close'); } catch { /* process fallback below */ }
  try { socket?.close(); } catch { /* already closed */ }
  for (const p of pending.values()) { clearTimeout(p.timer); p.reject(Error('Browser closed')); }
  pending.clear();
  if (browser?.pid && browser.exitCode === null) {
    await Promise.race([new Promise(resolve => browser.once('exit', resolve)), sleep(3000)]);
    if (browser.exitCode === null) {
      if (process.platform === 'win32') spawnSync('taskkill', ['/PID', String(browser.pid), '/T', '/F'], {windowsHide: true, timeout: 10000, stdio: 'ignore'});
      else browser.kill('SIGKILL');
    }
  }
  if (profile) {
    try {
      assert.equal(path.dirname(path.resolve(profile)), path.resolve(os.tmpdir()));
      assert.ok(path.basename(profile).startsWith('edu-workspace-9240-'));
      await fs.rm(profile, {recursive: true, force: true, maxRetries: 5, retryDelay: 300});
    } catch (e) { cleanupErrors.push('Profile cleanup: ' + e.message); }
  }
}

(async () => {
  try { await main(); }
  catch (e) { failure = e.stack || String(e); console.error(failure); }
  finally {
    await cleanup();
    const report = {url: base, runID, passed: checks.length, checks, runtimeErrors, cleanupErrors,
      created: [...owned], deleted: [...deleted], failure: failure || null,
      scope: 'Isolated sample app; courseware editing and owner dialogs; HTML animation readiness.'};
    await fs.mkdir(output, {recursive: true});
    await fs.writeFile(path.join(output, 'new-workspace-validation.json'), JSON.stringify(report, null, 2));
    console.log(JSON.stringify(report, null, 2));
    if (failure || cleanupErrors.length) process.exitCode = 1;
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
