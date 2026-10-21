// Real Vue UI with explicit fixed API fixtures; never writes to production.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const puppeteer = require('../animation-video/node_modules/puppeteer-core');

(async () => {
  const out = path.resolve(__dirname, '../work/knowledge-click-20261020');
  await fs.mkdir(out, { recursive: true });
  const browser = await puppeteer.launch({ executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe', headless: true, args: ['--disable-background-networking'], defaultViewport: { width: 1440, height: 1080 } });
  const errors = [], checks = [];
  let page, searches = 0, saves = 0, reparses = 0;
  const asset = 'a'.repeat(32);
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=', 'base64');
  const provenance = { provider: 'knowledge_document', document_id: 'document-fixture', page: 7, title: '水循环教材.pdf', description: '固定资料图示', author: '原文未声明', license: 'not_declared', retrieved_at: '2026-10-20T07:00:00Z', sha256: 'a'.repeat(64) };
  const graph = { title: '比较开口方向', x_min: -3, x_max: 3, y_min: -4, y_max: 4, renderer: 'matplotlib', series: [{ kind: 'quadratic', a: 1, b: 0, c: 0, mark_x: [1] }, { kind: 'quadratic', a: -1, b: 0, c: 0, mark_x: [-1] }] };
  const task = { id: 'knowledge-click-fixture', type: 'courseware', status: 'awaiting_preview', mode: 'sample', revision: 1, cost: 20, input: { topic: '资料复用与函数比较', pages: 3, theme: 'ocean', preview_first: true, use_kb: true, use_web: false }, events: [], sources: [], preview_images_available: false, slide_reviews: [], result: { title: '固定交互验收', theme: 'ocean', lesson: '固定样例，不代表真实AI结果', slides: [
    { id: 'slide-1', title: '观察水循环', layout: 'concept', bullets: ['观察图示并解释。'], notes: '固定工程样例', image_need: { purpose: '观察水循环', queries: ['水循环图示'] } },
    { id: 'slide-2', title: '比较二次函数', layout: 'concept', bullets: ['比较两条曲线。'], notes: '固定工程样例', graph },
    { id: 'slide-3', title: '解释变化', layout: 'concept', bullets: ['用图像说明理由。'], notes: '固定工程样例' }
  ] } };
  try {
    page = await browser.newPage();
    page.on('pageerror', e => errors.push(e.message));
    await page.setRequestInterception(true);
    page.on('request', async request => {
      if (!request.url().includes('/api/')) return request.continue();
      const p = new URL(request.url()).pathname;
      let data = [], body, contentType = 'application/json';
      try {
        if (p.endsWith('/images/search')) {
          const input = JSON.parse(request.postData());
          assert.equal(input.allow_web, false); searches++;
          data = { status: 'found', message: '固定验收：知识库图片已匹配', candidates: [{ picture: { asset_id: asset, caption: input.need.purpose, source: '企业知识库 · 第7页' }, provenance, reason: '固定视觉审核结果' }] };
        } else if (p.endsWith('/edit') || p.endsWith('/draft')) { task.result = JSON.parse(request.postData()).result; task.revision++; saves++; data = { ok: true, revision: task.revision }; }
        else if (p === '/api/documents/document-fixture/images') data = { images: [{ asset_id: asset, page: 7, caption: '水循环固定图示', provenance }], warnings: [] };
        else if (p === '/api/documents/document-fixture/reparse') { reparses++; data = { ok: true }; }
        else if (p === '/api/documents') data = [{ id: 'document-fixture', name: '水循环教材.pdf', status: 'ready', chunks: 3, image_count: 1, image_warnings: [], original_available: true, created_at: '2026-10-20T07:00:00Z' }];
        else if (p.startsWith('/api/media/') && p.endsWith('/metadata')) data = { asset_id: asset, provenance };
        else if (p.startsWith('/api/media/') || p.endsWith('/preview')) { body = png; contentType = 'image/png'; }
        else if (p === '/api/health') data = { status: 'ok', mode: 'sample', knowledge_image_version: 'knowledge-images-v1', document_reparse_version: 'reparse-v1', courseware_native_click_version: 'native-click-v1', courseware_canvas_version: 'canvas-v3', courseware_studio_version: 'studio-v1', teaching_features_version: 'teaching-v1', works_view_version: 'views-v1' };
        else if (p === '/api/account') data = { username: 'teacher', name: '测试教师' };
        else if (p === '/api/auth/me') data = { tenant_id: 1, member_id: 1, name: '测试教师', role: 'teacher', permissions: ['courseware.generate', 'kb.manage'] };
        else if (p === '/api/tenants') data = [{ id: 1, name: '固定验收' }];
        else if (p === '/api/billing') data = { balance: 100, available: 100, reserved: 0, plan: 'basic', ledger: [] };
        else if (p === '/api/tasks') data = [task];
        else if (p === '/api/tasks/' + task.id) data = task;
        else if (p === '/api/dashboard') data = { totals: {}, tasks: [], usage: [] };
        else if (p === '/api/usage') data = { items: [], total: 0 };
        else if (request.method() !== 'GET') throw Error('Unexpected write ' + p);
        await request.respond({ status: 200, contentType, body: body || JSON.stringify(data) });
      } catch (e) { errors.push(e.message); if (!request.isInterceptResolutionHandled()) await request.respond({ status: 500, body: JSON.stringify({ message: e.message }) }); }
    });
    await page.evaluateOnNewDocument(() => sessionStorage.setItem('edu_token', 'fixture-token'));
    await page.goto(process.env.TEST_APP_URL || 'http://127.0.0.1:15174');
    const click = async (text, scope = 'body') => page.evaluate(({ text, scope }) => { const element = [...document.querySelector(scope).querySelectorAll('button')].find(e => e.textContent.trim().includes(text)); if (!element || element.disabled) throw Error('Missing enabled button ' + text); element.click(); }, { text, scope });
    await page.waitForFunction(() => [...document.querySelectorAll('nav button')].some(b => b.textContent.includes('知识与来源')));
    await click('知识与来源');
    await page.waitForSelector('.knowledge-images summary');
    await page.click('.knowledge-images summary');
    await page.waitForSelector('.knowledge-images img');
    assert.ok((await page.$eval('.knowledge-images figcaption', e => e.innerText)).includes('第7页'));
    await page.screenshot({ path: path.join(out, 'knowledge-ui.png'), fullPage: true });
    checks.push('资料图片鉴权预览与页码显示');
    page.once('dialog', dialog => dialog.accept());
    await click('重新解析');
    await page.waitForFunction(() => !document.querySelector('.spinner'));
    assert.equal(reparses, 1); checks.push('旧资料重新解析入口');
    await click('我的作品'); await page.waitForSelector('.task-open'); await page.click('.task-open');
    await page.waitForFunction(() => document.body.innerText.includes('手动编辑内容')); await click('手动编辑内容');
    const scope = '.studio-slide.selected';
    await page.waitForSelector(scope + ' .commons-image-search details');
    await page.$eval(scope + ' .commons-image-search details', e => { for (let parent = e; parent; parent = parent.parentElement) if (parent.tagName === 'DETAILS') parent.open = true; });
    const enabled = await page.$eval(scope + ' .commons-image-search', e => [...e.querySelectorAll('button')].find(b => /自动配图/.test(b.textContent)).disabled);
    assert.equal(enabled, false); await click('自动配图', scope);
    await page.waitForSelector(scope + ' .commons-attribution');
    await page.$eval(scope + ' .commons-attribution', e => e.open = true);
    assert.ok((await page.$eval(scope + ' .commons-attribution', e => e.innerText)).includes('第7页'));
    assert.equal(await page.$$eval(scope + ' .commons-attribution a', es => es.filter(e => e.href.startsWith('knowledge:')).length), 0);
    await click('保存本页及其他修改'); await page.waitForFunction(() => !document.querySelector('.unsaved-note'));
    assert.equal(searches, 1); assert.equal(task.result.slides[0].picture.asset_id, asset);
    checks.push('关闭联网仍可匹配知识库，allow_web保持false，自动保存私有素材引用');
    await page.$$eval('button', es => { const button = es.find(b => b.textContent.includes('比较二次函数')); if (!button) throw Error('Missing curve slide'); button.click(); });
    await page.waitForFunction(() => document.querySelector('.studio-slide.selected')?.innerText.includes('比较二次函数'));
    await click('手动编辑内容');
    await page.waitForSelector(scope + ' select[aria-label="函数绘图方式"]');
    await page.$eval(scope + ' .courseware-evidence-editor', e => e.open = true);
    const toggle = async text => page.$$eval(scope + ' label', (es, text) => { const label = es.find(e => e.textContent.includes(text)); const input = label?.querySelector('input[type=checkbox]'); if (!input) throw Error('Missing checkbox ' + text); input.click(); }, text);
    await toggle('PPT 放映时点击');
    assert.equal(await page.$eval(scope + ' select[aria-label="函数绘图方式"]', e => e.value), 'native');
    await toggle('导出参数变化GIF');
    assert.equal(await page.$$eval(scope + ' label', es => es.find(e => e.textContent.includes('PPT 放映时点击')).querySelector('input').checked), false);
    await toggle('PPT 放映时点击');
    assert.equal(await page.$$eval(scope + ' label', es => es.find(e => e.textContent.includes('导出参数变化GIF')).querySelector('input').checked), false);
    await page.select(scope + ' select[aria-label="函数绘图方式"]', 'matplotlib');
    assert.equal(await page.$$eval(scope + ' label', es => es.find(e => e.textContent.includes('PPT 放映时点击')).querySelector('input').checked), false);
    await toggle('PPT 放映时点击');
    await page.screenshot({ path: path.join(out, 'native-click-ui.png'), fullPage: true });
    await click('保存本页及其他修改'); await page.waitForFunction(() => !document.querySelector('.unsaved-note'));
    assert.equal(task.result.slides[1].graph.reveal_on_click, true); assert.equal(task.result.slides[1].graph.renderer, 'native'); assert.equal(task.result.slides[1].graph.animation, null);
    assert.equal(saves, 2); assert.deepEqual(errors, []);
    checks.push('点击动画与GIF/高清图互斥，选择后保存且保留原生曲线参数');
    await fs.writeFile(path.join(out, 'browser.json'), JSON.stringify({ scope: 'Real Vue; fixed intercepted APIs, no production mutations', checks, searches, saves, reparses, errors }, null, 2));
    console.log(JSON.stringify({ checks, searches, saves, reparses }));
  } catch (e) { if (page) { console.error((await page.evaluate(() => document.body.innerText)).slice(-5500)); await page.screenshot({ path: path.join(out, 'browser-failure.png'), fullPage: true }); } throw e; }
  finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
