// Windows browser acceptance tests. All data is isolated from the user's blog.
const { chromium } = require('../../web/node_modules/playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const { spawn, execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '../..');
const artifacts = path.join(root, '.e2e-artifacts');
const results = [];
let server, browser, page, base, scratch;
const errors = [];
const contexts = [];

async function freePort() {
  const socket = net.createServer();
  await new Promise((resolve, reject) => socket.listen(0, '127.0.0.1', resolve).once('error', reject));
  const port = socket.address().port;
  await new Promise(resolve => socket.close(resolve));
  return port;
}
async function start(data, media = path.join(data, 'uploads')) {
  const port = await freePort();
  base = `http://127.0.0.1:${port}`;
  server = spawn(path.join(scratch, 'server.exe'), [], {cwd: root, windowsHide: true,
    env: {...process.env, APP_PORT: String(port), APP_DATA_DIR: data, APP_MEDIA_DIR: media,
      APP_WEB_DIR: path.join(root, 'web/dist'), APP_COOKIE_SECURE: 'false', APP_VERSION: 'e2e'}});
  const log = require('node:fs').createWriteStream(path.join(artifacts, 'server.log'), {flags:'a'});
  server.stdout.pipe(log, {end:false}); server.stderr.pipe(log, {end:false});
  server.once('close', () => log.end());
  let spawnError; server.once('error', error => {spawnError=error;});
  for (let attempt=0; attempt<100; attempt++) {
    if (spawnError) throw spawnError;
    if (server.exitCode !== null) throw new Error(`Server exited: ${server.exitCode}`);
    try {if ((await fetch(base+'/healthz')).ok) return;} catch {}
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  throw new Error('Server did not become live');
}
async function stop() {
  if (server && server.exitCode === null) {
    const exited = new Promise(resolve=>server.once('exit',resolve));
    server.kill(); await exited;
  }
}
async function context() {
  const ctx = await browser.newContext({acceptDownloads:true}); contexts.push(ctx);
  ctx.on('page', p=>{p.setDefaultTimeout(10000);p.on('pageerror',e=>errors.push(e.message));});
  return ctx;
}
async function api(ctx, method, route, data, expected=200, csrf=true) {
  const token = (await ctx.cookies(base)).find(c=>c.name==='app_csrf')?.value;
  const response = await ctx.request.fetch(base+route,{method,data,
    headers:csrf&&token?{'X-CSRF-Token':token}:{}});
  assert.equal(response.status(),expected,`${method} ${route}: ${await response.text()}`);
  return response.status()===204 ? null : response.json();
}
function parseColor(value) {
  const match = String(value).match(/rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/);
  assert.ok(match, 'unparsed color ' + value);
  return match.slice(1, 4).map(Number);
}
function assertNeutralGround(value) {
  const [r, g, b] = parseColor(value);
  const spread = Math.max(r, g, b) - Math.min(r, g, b);
  assert.ok(spread <= 6, `page ground is tinted (${value})`);
  assert.ok(r > 220 && g > 220 && b > 220, `page ground is too dark (${value})`);
  assert.ok(!(r === 251 && g === 246 && b === 239), 'old cream paper is still the page ground');
}
function assertZeroRadius(value, label) {
  assert.equal(typeof value, 'string', label + ' is missing');
  for (const part of value.trim().split(/\s+/)) assert.ok(part === '0px' || part === '0', `${label} radius ${value}`);
}
async function sampleChrome(p, suffix) {
  const routes = [
    {path: '/user', button: 'button.button', input: 'input', card: '.list-mast, .post-card, .empty-state', avatar: '.brand-mark'},
    {path: '/admin', button: 'button.button, button.avatar', input: 'input, select, textarea', card: '.stat-card', avatar: '.avatar'},
    {path: '/setting', button: 'button.button', input: 'input, textarea, select', card: '.settings-block', avatar: '.brand-mark'},
  ];
  const samples = [];
  for (const route of routes) {
    await p.goto(base + route.path, {waitUntil: 'domcontentloaded'});
    await p.locator('h1, h2').first().waitFor();
    const sample = await p.evaluate(({button, input, card, avatar}) => {
      const first = (sel) => document.querySelector(sel);
      const box = (el) => {
        if (!el) return null;
        const r = el.getBoundingClientRect();
        return {w: r.width, h: r.height, top: r.top, bottom: r.bottom};
      };
      const radius = (el) => el ? getComputedStyle(el).borderRadius : null;
      const heading = document.querySelector('h1, h2');
      const form = document.querySelector('form');
      const inView = (b) => !!(b && b.w > 40 && b.h > 16 && b.top < window.innerHeight * 0.92 && b.bottom > 0);
      return {
        ground: getComputedStyle(document.body).backgroundColor,
        backgroundImage: getComputedStyle(document.body).backgroundImage,
        filled: inView(box(heading)) || inView(box(form)),
        headingText: heading ? heading.textContent.trim().slice(0, 48) : '',
        radii: {
          button: radius(first(button)),
          input: radius(first(input)),
          card: radius(first(card)),
          avatar: radius(first(avatar)),
        },
      };
    }, route);
    samples.push({path: route.path, ...sample});
    await p.screenshot({path: path.join(artifacts, `visual-${suffix}-${route.path.replace(/\W+/g, '') || 'root'}.png`), fullPage: false});
  }
  return samples;
}
function chromeSignature(samples) {
  return samples.map((sample) => ({path: sample.path, ground: sample.ground, backgroundImage: sample.backgroundImage, filled: sample.filled, radii: sample.radii}));
}
async function step(name, work) {
  const started=Date.now();
  try {await work();results.push({name,status:'passed',durationMs:Date.now()-started});console.log('PASS '+name);}
  catch(error) {results.push({name,status:'failed',error:error.stack,durationMs:Date.now()-started});throw error;}
}
async function login(ctx, username, password) {
  const p = await ctx.newPage(); await p.goto(base+'/admin');
  await p.getByLabel('用户名',{exact:true}).fill(username);
  await p.getByLabel('密码',{exact:true}).fill(password);
  await p.getByRole('button',{name:'登录管理端',exact:true}).click();
  await p.locator('.stats-grid').waitFor(); return p;
}
function postRow(title) {return page.locator('#posts .admin-post-row').filter({hasText:title});}
async function writePost(title,slug,content,status='published') {
  await page.goto(base+'/admin');
  await page.getByRole('button',{name:'+ 写文章',exact:true}).click();
  await page.getByPlaceholder('文章标题',{exact:true}).fill(title);
  await page.getByPlaceholder('留空则保留现有地址或按标题生成').fill(slug);
  await page.getByPlaceholder('开始写作，支持 Markdown…').fill(content);
  await page.locator('#posts select').selectOption(status);
  await page.getByRole('button',{name:'保存文章',exact:true}).click();
  await postRow(title).waitFor();
}

(async()=>{
  assert.equal(process.platform,'win32','These tests support Windows only');
  await fs.mkdir(artifacts,{recursive:true});
  scratch=await fs.mkdtemp(path.join(os.tmpdir(),'typecho-e2e-'));
  console.log('Temporary data: '+scratch);
  execFileSync('powershell.exe',['-NoProfile','-Command','npm.cmd run build'],{cwd:path.join(root,'web'),stdio:'inherit',windowsHide:true});
  execFileSync('go',['build','-o',path.join(scratch,'server.exe'),'./cmd/server'],{cwd:root,stdio:'inherit',windowsHide:true,env:{...process.env,CGO_ENABLED:'1'}});
  const executablePath=process.env.E2E_BROWSER_PATH || path.join(process.env['ProgramFiles(x86)']||'C:/Program Files (x86)','Microsoft/Edge/Application/msedge.exe');
  browser=await chromium.launch({executablePath,headless:true});
  const dataDir=path.join(scratch,'data');await start(dataDir);
  const owner=await context();page=await owner.newPage();page.on('dialog',d=>d.accept());
  const publicCtx=await context();const reader=await publicCtx.newPage();
  let originalPost, uploaded, backupPath;
  await step('first setup and readiness',async()=>{
    assert.equal((await fetch(base+'/readyz')).status,503);
    await page.goto(base);await page.waitForURL('**/setting');
    await page.getByLabel('站点名称',{exact:true}).fill('E2E 个人博客');
    await page.getByLabel('站点地址',{exact:true}).fill(base);
    await page.getByLabel('管理员邮箱',{exact:true}).fill('owner@example.com');
    await page.getByLabel('用户名',{exact:true}).fill('owner');
    await page.getByLabel('密码',{exact:true}).fill('password-owner-123');
    await page.getByRole('button',{name:'完成设置并进入管理端'}).click();await page.waitForURL('**/admin');
    await page.locator('.stats-grid').waitFor();assert.equal((await fetch(base+'/readyz')).status,200);
  });
  await step('square minimalist chrome on reader, admin and settings',async()=>{
    const before = errors.length;
    const first = await sampleChrome(page, 'a');
    const second = await sampleChrome(page, 'b');
    assert.deepEqual(chromeSignature(second), chromeSignature(first), 'two visual probes disagreed');
    for (const sample of first) {
      assert.equal(sample.filled, true, sample.path + ' viewport is blank');
      assert.equal(sample.backgroundImage, 'none', sample.path + ' still has a paper gradient');
      assertNeutralGround(sample.ground);
      for (const key of ['button', 'input', 'card', 'avatar']) assertZeroRadius(sample.radii[key], sample.path + ' ' + key);
    }
    assert.equal(errors.length, before, 'page errors during visual probe: ' + errors.slice(before).join('; '));
    console.log('VISUAL ' + JSON.stringify(chromeSignature(first)));
  });
  if (process.env.E2E_ONLY === 'visual') return;
  await step('settings navigation, saved state, leave warning and mobile layout',async()=>{
    await page.goto(base+'/setting');const save=page.getByRole('button',{name:'保存设置',exact:true});await save.waitFor();assert.ok(await save.isDisabled());
    await page.getByLabel('站点描述',{exact:true}).fill('E2E 站点介绍');assert.ok(await save.isEnabled());await page.locator('#site').getByText('有未保存的修改',{exact:true}).waitFor();
    await page.locator('.settings-index').getByRole('link',{name:'阅读与评论',exact:true}).click();assert.equal(await page.getByLabel('站点描述',{exact:true}).inputValue(),'E2E 站点介绍');
    await page.locator('.settings-index').getByRole('link',{name:'站点信息',exact:true}).click();await save.click();await page.getByText('设置已保存',{exact:true}).waitFor();assert.ok(await save.isDisabled());
    await page.reload();assert.equal(await page.getByLabel('站点描述',{exact:true}).inputValue(),'E2E 站点介绍');
    await page.setViewportSize({width:390,height:844});await page.locator('.settings-index').getByRole('link',{name:'阅读与评论',exact:true}).click();
    await page.waitForFunction(()=>{const y=document.getElementById('reading').getBoundingClientRect().top;return y>=0&&y<160;});
    assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1),'mobile settings overflow');
    assert.equal(await page.locator('.settings-index').evaluate(el=>getComputedStyle(el).position),'sticky');
    await page.screenshot({path:path.join(artifacts,'mobile-settings.png'),fullPage:false});await page.setViewportSize({width:1280,height:800});
    await page.locator('.settings-index').getByRole('link',{name:'站点信息',exact:true}).click();await page.waitForFunction(()=>{const y=document.getElementById('site').getBoundingClientRect().top;return y>=0&&y<160;});await page.screenshot({path:path.join(artifacts,'settings.png'),fullPage:false});
    let warned=false;const onDialog=d=>{if(d.type()==='beforeunload')warned=true;};page.on('dialog',onDialog);
    await page.getByLabel('站点描述',{exact:true}).fill('Unsaved text');await page.locator('.settings-links').getByRole('link',{name:'管理',exact:true}).click();await page.waitForURL('**/admin');page.off('dialog',onDialog);assert.ok(warned,'unsaved settings must warn before leaving');
    await reader.goto(base+'/setting');await reader.getByRole('heading',{name:'请先登录',exact:true}).waitFor();await reader.getByRole('link',{name:'前往登录',exact:true}).waitFor();
  });
  await step('publish Markdown and sanitize HTML',async()=>{
    await writePost('E2E 公开文章','e2e-public','# 正文标题\n\nOriginal body\n\n<script>window.e2eXss=true</script><img src=x onerror="window.e2eXss=true">');
    originalPost=await api(owner,'GET','/api/v1/content/posts/e2e-public');
    await reader.goto(base+'/user/e2e-public');await reader.getByRole('heading',{name:'正文标题',exact:true}).waitFor();
    assert.equal(await reader.evaluate(()=>window.e2eXss),undefined);
    assert.equal(await reader.locator('.article-body [onerror], .article-body script').count(),0);
  });
  await step('draft privacy and authorized preview',async()=>{
    await writePost('E2E 草稿','e2e-draft','Private draft','draft');
    await api(publicCtx,'GET','/api/v1/content/posts/e2e-draft',undefined,404);
    await reader.goto(base+'/user/e2e-draft');await reader.getByText('Private draft',{exact:true}).waitFor({state:'hidden'});
    const preview=await owner.newPage();await preview.goto(base+'/user/e2e-draft?preview=true');
    await preview.locator('.article-body').filter({hasText:'Private draft'}).waitFor();await preview.close();
  });
  await step('edit preserves permalink and restore revision',async()=>{
    await page.goto(base+'/admin');await postRow('E2E 公开文章').getByRole('button',{name:'编辑',exact:true}).click();
    await page.getByPlaceholder('文章标题',{exact:true}).fill('E2E 修改文章');
    await page.getByPlaceholder('开始写作，支持 Markdown…').fill('Revised body');
    await page.getByRole('button',{name:'保存修改',exact:true}).click();await postRow('E2E 修改文章').waitFor();
    assert.equal((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public')).content,'Revised body');
    await postRow('E2E 修改文章').getByRole('button',{name:'编辑',exact:true}).click();
    await page.getByRole('button',{name:'恢复此版本',exact:true}).first().click();
    await page.waitForFunction(()=>document.querySelector('#posts textarea[required]')?.value.includes('Original body'));
    assert.ok((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public')).content.includes('Original body'));
  });
  await step('page create, edit, public reading and delete',async()=>{
    await page.goto(base+'/admin');await page.getByRole('button',{name:'+ 新建页面',exact:true}).click();
    await page.getByPlaceholder('页面标题',{exact:true}).fill('E2E 关于');
    await page.locator('#pages .editor-slug input').fill('e2e-about');
    await page.getByPlaceholder('页面内容，支持 Markdown…').fill('About version one');
    await page.locator('#pages select').selectOption('published');await page.getByRole('button',{name:'保存页面',exact:true}).click();
    const row=page.locator('#pages .admin-post-row').filter({hasText:'E2E 关于'});await row.waitFor();
    await reader.goto(base+'/page/e2e-about');await reader.locator('.article-body').filter({hasText:'About version one'}).waitFor();
    await row.getByRole('button',{name:'编辑',exact:true}).click();await page.getByPlaceholder('页面内容，支持 Markdown…').fill('About version two');
    await page.locator('#pages').getByRole('button',{name:'保存修改',exact:true}).click();
    await reader.reload();await reader.locator('.article-body').filter({hasText:'About version two'}).waitFor();
    await row.getByRole('button',{name:'删除',exact:true}).click();await row.waitFor({state:'detached'});
    await api(publicCtx,'GET','/api/v1/content/pages/e2e-about',undefined,404);
  });
  await step('taxonomy rename keeps article associations',async()=>{
    await api(owner,'PUT','/api/v1/content/posts/'+originalPost.id,{title:'E2E 公开文章',content:'Original body',status:'published',categories:['E2E 分类'],tags:['E2E 标签']});
    await page.goto(base+'/admin');const row=page.locator('.taxonomy-row').filter({hasText:'E2E 分类'});await row.getByRole('button',{name:'重命名',exact:true}).click();
    const editing=page.locator('.taxonomy-row').filter({has:page.locator('input')});
    await editing.locator('input').fill('E2E 新分类');await editing.getByRole('button',{name:'保存',exact:true}).click();
    await page.locator('.taxonomy-row').getByText('E2E 新分类',{exact:true}).waitFor();
    assert.equal((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public')).categories[0].name,'E2E 新分类');
    await reader.goto(base+'/user');await reader.locator('.term-filter-bar').getByRole('link').filter({hasText:'E2E 新分类'}).click();
    await reader.locator('.post-card').filter({hasText:'E2E 公开文章'}).waitFor();assert.ok(reader.url().includes('category='));
  });
  await step('search, public pagination and feed privacy',async()=>{
    for(let i=0;i<3;i++)await api(owner,'POST','/api/v1/content/posts',{title:'E2E Search '+i,slug:'e2e-search-'+i,content:'needle-search-body',status:'published'},201);
    await api(owner,'POST','/api/v1/content/posts',{title:'E2E Future',slug:'e2e-future',content:'Future secret',status:'published',publishAt:new Date(Date.now()+86400000).toISOString()},201);
    await page.goto(base+'/setting');await page.getByLabel('每页文章数',{exact:true}).fill('2');await page.getByRole('button',{name:'保存阅读与评论设置',exact:true}).click();await page.getByText('阅读与评论设置已保存',{exact:true}).waitFor();
    await reader.goto(base+'/user');await reader.getByLabel('搜索文章',{exact:true}).fill('needle-search-body');await reader.getByRole('button',{name:'搜索',exact:true}).click();
    await reader.waitForFunction(()=>document.querySelectorAll('.post-card').length===2);await reader.getByRole('button',{name:'下一页',exact:true}).click();await reader.waitForFunction(()=>document.querySelectorAll('.post-card').length===1);
    for(const route of ['/feed.xml','/sitemap.xml']){const text=await(await fetch(base+route)).text();assert.ok(text.includes('e2e-public'));assert.ok(!text.includes('e2e-draft'));assert.ok(!text.includes('e2e-future'));}
    assert.ok((await(await fetch(base+'/robots.txt')).text()).includes('Disallow: /admin'));
  });
  const gif=Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7','base64');
  await step('media upload, insertion and image rendering',async()=>{
    await page.goto(base+'/admin');await page.locator('input[type=file]').setInputFiles({name:'e2e.gif',mimeType:'image/gif',buffer:gif});
    await page.waitForFunction(()=>Array.from(document.querySelectorAll('.media-card img')).some(i=>i.complete&&i.naturalWidth>0));
    uploaded=(await api(owner,'GET','/api/v1/media')).items[0];
    await page.getByRole('button',{name:'插入文章',exact:true}).click();
    assert.ok((await page.getByPlaceholder('开始写作，支持 Markdown…').inputValue()).includes(uploaded.url));
    const response=await publicCtx.request.get(base+uploaded.url);assert.equal(response.status(),200);assert.deepEqual(await response.body(),gif);
  });
  await step('protected post hides body and supports password retry',async()=>{
    await api(owner,'POST','/api/v1/content/posts',{title:'E2E Protected',slug:'e2e-protected',content:'Protected secret body',status:'published',passphrase:'read-password-123'},201);
    const locked=await api(publicCtx,'GET','/api/v1/content/posts/e2e-protected');assert.ok(locked.locked);assert.ok(!locked.content);
    assert.ok(!(await(await fetch(base+'/user/e2e-protected')).text()).includes('Protected secret body'));
    await reader.goto(base+'/user/e2e-protected');await reader.getByLabel('阅读口令',{exact:true}).fill('wrong-password');await reader.getByRole('button',{name:'查看正文',exact:true}).click();
    await reader.locator('.form-error').waitFor();await reader.getByLabel('阅读口令',{exact:true}).fill('read-password-123');await reader.getByRole('button',{name:'查看正文',exact:true}).click();
    await reader.locator('.article-body').filter({hasText:'Protected secret body'}).waitFor();
  });
  await step('reject disguised image uploads',async()=>{
    const token=(await owner.cookies(base)).find(c=>c.name==='app_csrf').value;
    const response=await owner.request.post(base+'/api/v1/media',{headers:{'X-CSRF-Token':token},multipart:{file:{name:'fake.png',mimeType:'image/png',buffer:Buffer.from('<script>bad image</script>')}}});
    assert.equal(response.status(),415);assert.equal((await response.json()).error.code,'unsupported_media');
  });
  await step('anonymous comment, moderation and threaded reply',async()=>{
    await reader.goto(base+'/user/e2e-public');await reader.getByLabel('称呼',{exact:true}).fill('E2E 访客');await reader.getByLabel('评论',{exact:true}).fill('E2E 待审核评论');await reader.getByRole('button',{name:'发表评论',exact:true}).click();await reader.locator('.form-success').waitFor();
    assert.equal((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public/comments')).total,0);
    await page.goto(base+'/admin');const row=page.locator('.admin-comment-row').filter({hasText:'E2E 待审核评论'});await row.getByRole('button',{name:'通过',exact:true}).click();await row.waitFor({state:'detached'});
    await reader.reload();const comment=reader.locator('.comment-card').filter({hasText:'E2E 待审核评论'});await comment.getByRole('button',{name:'回复',exact:true}).click();
    await reader.getByLabel('称呼',{exact:true}).fill('E2E 回复者');await reader.getByLabel('评论',{exact:true}).fill('E2E 线程回复');await reader.getByRole('button',{name:'发表回复',exact:true}).click();await reader.locator('.form-success').waitFor();
    await page.reload();await page.locator('.admin-comment-row').filter({hasText:'E2E 线程回复'}).getByRole('button',{name:'通过',exact:true}).click();
    await reader.reload();await reader.locator('.comment-card[data-depth="1"]').filter({hasText:'E2E 线程回复'}).waitFor();
  });
  await step('comment closure and CSRF enforcement',async()=>{
    await page.goto(base+'/setting');await page.getByLabel('开放评论',{exact:true}).uncheck();await page.getByRole('button',{name:'保存阅读与评论设置',exact:true}).click();await page.getByText('阅读与评论设置已保存',{exact:true}).waitFor();
    await reader.goto(base+'/user/e2e-public');await reader.getByText('评论已关闭。',{exact:true}).waitFor();assert.equal(await reader.locator('.comment-form').count(),0);
    await api(owner,'POST','/api/v1/content/posts',{title:'CSRF blocked',content:'blocked',status:'draft'},403,false);
    await page.getByLabel('开放评论',{exact:true}).check();await page.getByRole('button',{name:'保存阅读与评论设置',exact:true}).click();
  });
  await step('editor/moderator permissions and immediate user disable',async()=>{
    await page.goto(base+'/setting');
    for(const role of ['editor','moderator']){
      const form=page.locator('.user-create-form');await form.getByLabel('用户名',{exact:true}).fill(role);await form.getByLabel('邮箱',{exact:true}).fill(role+'@example.com');await form.getByLabel('初始密码',{exact:true}).fill('password-staff-123');await form.locator('select').selectOption(role);await form.getByRole('button',{name:'创建用户',exact:true}).click();await page.locator('.user-row').filter({hasText:role+'@example.com'}).waitFor();
    }
    const editor=await context();const ep=await login(editor,'editor','password-staff-123');assert.equal(await ep.getByRole('link',{name:'备份',exact:true}).count(),0);
    await api(editor,'GET','/api/v1/backup',undefined,403);await api(editor,'GET','/api/v1/settings/users',undefined,403);
    await api(editor,'POST','/api/v1/content/posts',{title:'Staff draft',slug:'staff-draft',content:'Editor allowed',status:'draft'},201);
    const moderator=await context();const mp=await login(moderator,'moderator','password-staff-123');assert.equal(await mp.getByRole('button',{name:'+ 写文章',exact:true}).count(),0);
    await api(moderator,'POST','/api/v1/content/posts',{title:'Blocked',content:'Blocked',status:'draft'},403);await api(moderator,'GET','/api/v1/content/comments');
    await mp.locator('button.avatar').click();await mp.getByRole('button',{name:'登录管理端',exact:true}).waitFor();await api(moderator,'GET','/api/v1/auth/me',undefined,401);
    const row=page.locator('.user-row').filter({hasText:'editor@example.com'});await row.getByLabel('启用',{exact:true}).uncheck();
    await page.getByText('用户信息已保存',{exact:true}).waitFor();
    await api(editor,'GET','/api/v1/auth/me',undefined,401);
  });
  await step('admin pagination beyond 100 and audit pagination',async()=>{
    for(let i=0;i<101;i++)await api(owner,'POST','/api/v1/content/posts',{title:'Paging fixture '+String(i).padStart(3,'0'),slug:'paging-'+i,content:'paging',status:'draft'},201);
    await page.goto(base+'/admin');await page.locator('#posts .pagination').getByRole('button',{name:'下一页',exact:true}).click();await page.locator('#posts .pagination').getByText(/2 \/ 2/).waitFor();
    await page.waitForFunction(()=>{const count=document.querySelectorAll('#posts .admin-post-row').length;return count>0&&count<100;});
    await page.goto(base+'/setting');const nextAudit=page.waitForResponse(r=>r.url().includes('/api/v1/audit?page=2&'));await page.locator('.settings-audit .pagination').getByRole('button',{name:'下一页',exact:true}).click();const audit=await(await nextAudit).json();await page.locator('.audit-row').first().filter({hasText:audit.items[0].resourceId}).waitFor();await page.locator('.settings-audit .pagination').getByText(/2 \//).waitFor();
  });
  await step('owner password change revokes other sessions',async()=>{
    const other=await context();await login(other,'owner','password-owner-123');
    await page.goto(base+'/setting');await page.getByLabel('当前密码',{exact:true}).fill('password-owner-123');await page.getByLabel('新密码',{exact:true}).fill('password-owner-456');await page.getByRole('button',{name:'保存账户信息',exact:true}).click();await page.getByText('账户信息已保存',{exact:true}).waitFor();
    await api(owner,'GET','/api/v1/auth/me');await api(other,'GET','/api/v1/auth/me',undefined,401);
  });
  await step('backup download and missing media failure',async()=>{
    await page.goto(base+'/admin');const downloading=page.waitForEvent('download');await page.getByRole('link',{name:'备份',exact:true}).click();const download=await downloading;assert.equal(await download.failure(),null);
    backupPath=path.join(scratch,'backup.zip');await download.saveAs(backupPath);assert.ok((await fs.stat(backupPath)).size>0);
    const mediaRoot=path.join(dataDir,'uploads');const file=(await fs.readdir(mediaRoot)).find(n=>!n.startsWith('.'));await fs.rename(path.join(mediaRoot,file),path.join(scratch,'held-media'));
    try {await api(owner,'GET','/api/v1/backup',undefined,500);}finally{await fs.rename(path.join(scratch,'held-media'),path.join(mediaRoot,file));}
  });
  await step('restart persists content, media and sessions',async()=>{
    // Persist cookies across server ports, which are not part of cookie scope.
    await stop();await start(dataDir);await page.goto(base+'/admin');await page.locator('.stats-grid').waitFor();await api(owner,'GET','/api/v1/auth/me');
    assert.equal((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public')).title,'E2E 公开文章');
    assert.equal((await publicCtx.request.get(base+uploaded.url)).status(),200);
  });
  await step('restore ZIP into empty directories and verify service',async()=>{
    await stop();const recovery=path.join(scratch,'recovery');
    execFileSync('powershell.exe',['-NoProfile','-Command','Expand-Archive -LiteralPath $env:E2E_ZIP -DestinationPath $env:E2E_RECOVERY'],{env:{...process.env,E2E_ZIP:backupPath,E2E_RECOVERY:recovery},windowsHide:true});
    await start(recovery,path.join(recovery,'media'));await page.goto(base+'/admin');await page.locator('.stats-grid').waitFor();await api(owner,'GET','/api/v1/auth/me');
    const restoredPost=await api(publicCtx,'GET','/api/v1/content/posts/e2e-public');assert.equal(restoredPost.title,'E2E 公开文章');assert.equal(restoredPost.content,'Original body');
    assert.equal((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public/comments')).total,2);
    const response=await publicCtx.request.get(base+uploaded.url);assert.equal(response.status(),200);assert.deepEqual(await response.body(),gif);
    await api(publicCtx,'GET','/api/v1/content/posts/e2e-draft',undefined,404);
  });
  await step('delete post, media, taxonomy and staff through UI',async()=>{
    await page.goto(base+'/admin');const row=postRow('E2E Search 2');await row.getByRole('button',{name:'删除',exact:true}).click();await row.waitFor({state:'detached'});await api(publicCtx,'GET','/api/v1/content/posts/e2e-search-2',undefined,404);
    const asset=page.locator('.media-card').filter({hasText:'e2e.gif'});await asset.getByRole('button',{name:'删除',exact:true}).click();await asset.waitFor({state:'detached'});assert.equal((await publicCtx.request.get(base+uploaded.url)).status(),404);
    const term=page.locator('.taxonomy-row').filter({hasText:'E2E 标签'});await term.getByRole('button',{name:'删除',exact:true}).click();await term.waitFor({state:'detached'});assert.equal(((await api(publicCtx,'GET','/api/v1/content/posts/e2e-public')).tags||[]).length,0);
    await page.goto(base+'/setting');const staff=page.locator('.user-row').filter({hasText:'moderator@example.com'});await staff.getByRole('button',{name:'删除',exact:true}).click();await staff.waitFor({state:'detached'});
    assert.ok(!(await api(owner,'GET','/api/v1/settings/users')).items.some(u=>u.username==='moderator'));
  });
  await step('mobile reader and no browser exceptions',async()=>{
    await reader.setViewportSize({width:390,height:844});await reader.goto(base+'/user/e2e-public');await reader.locator('.article-body').waitFor();
    assert.ok(await reader.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1),'mobile page has horizontal overflow');
    await reader.screenshot({path:path.join(artifacts,'mobile-reader.png'),fullPage:true});await page.screenshot({path:path.join(artifacts,'admin.png'),fullPage:true});assert.deepEqual(errors,[]);
  });
})().catch(async error=>{
  console.error(error);process.exitCode=1;
  if(page)await page.screenshot({path:path.join(artifacts,'failure.png'),fullPage:true}).catch(()=>{});
}).finally(async()=>{
  if(browser)await browser.close();await stop();
  await fs.mkdir(artifacts,{recursive:true});
  await fs.writeFile(path.join(artifacts,'report.json'),JSON.stringify({date:new Date().toISOString(),platform:process.platform,scratch,results,browserErrors:errors},null,2));
  console.log(`${results.filter(r=>r.status==='passed').length}/${results.length} passed; report: ${path.join(artifacts,'report.json')}`);
});
