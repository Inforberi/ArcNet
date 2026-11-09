// Local React + Go signed fixture. Restart outcomes are explicitly simulated.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
const {chromium}=createRequire(import.meta.url)('playwright');
const base=process.env.ARCVPN_E2E_URL||'http://127.0.0.1:18394';
assert.ok(['localhost','127.0.0.1'].includes(new URL(base).hostname));
const out='/private/tmp/arcnet-version-review';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true});
const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage();
page.setDefaultTimeout(10000);const results={steps:[],widths:[],errors:[],scope:'Local signed Go fixture. Restart outcomes mocked. No systemd/VPS.'};
page.on('pageerror',e=>results.errors.push(e.message));
const update=()=>page.locator('.update-settings');
async function step(name,fn){await fn();results.steps.push(name);console.log('PASS '+name)}
async function go(){await page.goto(base+'/versions');await update().waitFor();await page.getByRole('button',{name:'Скачать обновление',exact:true}).waitFor()}
try{
 await page.route('**/api/system/update/history',r=>r.fulfill({json:{items:[{version:'0.6.9',notes:'Old public release',published_at:'2026-10-01T12:00:00Z'},{version:'9.0.0',notes:'Unsigned future release'}]}}));
 await page.goto(base+'/users');await page.getByLabel('Логин',{exact:true}).fill('browser-owner');await page.getByLabel('Пароль',{exact:true}).fill('disposable-browser-password-12345');await page.getByRole('button',{name:'Войти',exact:true}).click();await page.getByRole('heading',{name:'Пользователи',exact:true}).waitFor();
 await step('alias, normal version, signed and informational history',async()=>{
  await go();assert.match(page.url(),/security\?section=updates/);assert.equal(await page.getByRole('link',{name:'Версии',exact:true}).count(),0);
  await update().getByRole('heading',{name:'Доступно обновление · 0.7.1',exact:true}).waitFor();assert.equal(await update().locator('select').count(),0);
  await update().getByText('Другие версии и история',{exact:true}).click();await update().locator('.update-history-summary').filter({hasText:'Unsigned future release'}).waitFor();
  for(const text of ['Old public release','Unsigned future release','Исторические подключения и UI'])assert.equal(await update().locator('li').filter({hasText:text}).getByRole('button').count(),0);
  assert.equal(await update().getByRole('button',{name:'Выбрать этот выпуск',exact:true}).count(),1);
 });
 await step('responsive updates and expanded history',async()=>{
  for(const width of [320,375,390,430,768,1024,1280,1440,1600,1920]){await page.setViewportSize({width,height:900});const d=await page.evaluate(()=>({w:document.documentElement.clientWidth,s:document.documentElement.scrollWidth}));assert.ok(d.s<=d.w+1,JSON.stringify({width,...d}));results.widths.push(width);if([390,1440].includes(width))await page.screenshot({path:out+'/updates-'+width+'.png',fullPage:true})}
  await page.setViewportSize({width:1440,height:1000});
 });
 let downloads=0;page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/api/system/update/download')){downloads++;assert.equal(r.postDataJSON().version,'0.7.1')}});
 await step('real signed download, exact version, discard and reprepare',async()=>{
  await page.getByRole('button',{name:'Скачать обновление',exact:true}).dblclick();await page.getByRole('button',{name:'Установить обновление',exact:true}).waitFor();assert.equal(downloads,1);
  await page.getByRole('button',{name:'Отменить подготовку',exact:true}).click();await page.getByText('Подготовленный пакет удалён',{exact:true}).waitFor();await page.getByRole('button',{name:'Скачать обновление',exact:true}).click();await page.getByRole('button',{name:'Установить обновление',exact:true}).waitFor();assert.equal(downloads,2);
 });
 let applied=0;page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/api/system/update/apply'))applied++});
 await step('confirmation, single apply, actual version success after simulated restart',async()=>{
  await page.getByRole('button',{name:'Установить обновление',exact:true}).click();let d=page.getByRole('dialog',{name:'Установить обновление?',exact:true});await d.getByRole('button',{name:'Отмена',exact:true}).click();assert.equal(applied,0);
  await page.getByRole('button',{name:'Установить обновление',exact:true}).click();d=page.getByRole('dialog',{name:'Установить обновление?',exact:true});await d.getByRole('button',{name:'Установить',exact:true}).click();await update().getByText(/ожидаем перезапуск/).waitFor();assert.equal(applied,1);
  await page.route('**/api/system/update',r=>r.request().method()==='GET'?r.fulfill({json:{available:true,trusted:true,staged:false,current_version:'0.7.1',result:'success',started_at:20}}):r.continue());
  await page.getByRole('button',{name:'Проверить состояние',exact:true}).click();await page.getByText('Установлена версия 0.7.1',{exact:true}).waitFor();assert.equal(await page.evaluate(()=>sessionStorage.getItem('arcnet.pending-update')),null);await page.unroute('**/api/system/update');
 });
 await step('unknown deadline survives reload and cannot reinstall',async()=>{
  await page.route('**/api/system/update',r=>r.fulfill({json:{available:true,trusted:true,staged:true,version:'0.7.1',current_version:'0.7.0-stage5-a10',result:'success',started_at:0}}));
  await page.evaluate(()=>sessionStorage.setItem('arcnet.pending-update',JSON.stringify({version:'0.7.1',until:Date.now()-1000,started_at:0})));await page.reload();await update().getByRole('alert').filter({hasText:'не подтверждён за 3 минуты'}).waitFor();assert.equal(await page.getByRole('button',{name:'Установить обновление',exact:true}).count(),0);
  await page.reload();await update().getByRole('alert').filter({hasText:'не подтверждён за 3 минуты'}).waitFor();assert.equal(applied,1);
 });
 await step('later failed operation is distinct from an older successful result',async()=>{
  await page.unroute('**/api/system/update');await page.route('**/api/system/update',r=>r.fulfill({json:{available:true,trusted:true,staged:false,current_version:'0.7.0-stage5-a10',result:'failed',started_at:21}}));await page.getByRole('button',{name:'Проверить состояние',exact:true}).click();await page.getByRole('alert').filter({hasText:'Установка завершилась ошибкой'}).waitFor();assert.equal(await page.evaluate(()=>sessionStorage.getItem('arcnet.pending-update')),null);
 });
 await step('catalog and helper errors remain actionable',async()=>{
  await page.unroute('**/api/system/update');await page.route('**/api/system/update',r=>r.fulfill({json:{available:true,trusted:true,staged:false,current_version:'0.7.0-stage5-a10'}}));await page.route('**/api/system/update/releases',r=>r.fulfill({status:502,json:{error:'Тест: каталог недоступен'}}));await page.reload();await update().getByRole('alert').filter({hasText:'Тест: каталог недоступен'}).waitFor();await page.unroute('**/api/system/update/releases');await update().getByRole('button',{name:'Повторить загрузку',exact:true}).click();await page.getByRole('button',{name:'Скачать обновление',exact:true}).waitFor();
  await page.unroute('**/api/system/update');await page.route('**/api/system/update',r=>r.fulfill({json:{available:false,trusted:false,staged:false,current_version:'0.7.0-stage5-a10',message:'Тест: helper недоступен'}}));await page.getByRole('button',{name:'Проверить обновления',exact:true}).click();await page.getByText('Тест: helper недоступен',{exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:'Скачать обновление',exact:true}).count(),0);
 });
 await step('eight Unicode password preserves spaces; four emoji rejected',async()=>{
  await page.goto(base+'/security?section=account');await page.getByLabel('Новый пароль',{exact:true}).fill('😀😀😀😀');await page.getByLabel('Повторите новый пароль',{exact:true}).fill('😀😀😀😀');await page.getByLabel('Текущий пароль',{exact:true}).fill('disposable-browser-password-12345');await page.getByRole('button',{name:'Сохранить и войти заново',exact:true}).click();await page.getByRole('alert').filter({hasText:/8/}).waitFor();
  let body;await page.route('**/api/security/account',r=>{body=r.request().postDataJSON();return r.fulfill({status:422,json:{error:'Fixture: password captured without changing account'}})});
  await page.getByLabel('Новый пароль',{exact:true}).fill(' абвгде ');await page.getByLabel('Повторите новый пароль',{exact:true}).fill(' абвгде ');await page.getByRole('button',{name:'Сохранить и войти заново',exact:true}).click();await page.getByRole('alert').filter({hasText:'Fixture: password captured'}).waitFor();assert.equal(body.password,' абвгде ');
 });
 assert.deepEqual(results.errors,[]);console.log(JSON.stringify({steps:results.steps.length,widths:results.widths.length,frontendErrors:results.errors.length}));
}catch(e){results.failure=e.message;await page.screenshot({path:out+'/failure.png',fullPage:true}).catch(()=>{});console.error(e.stack);process.exitCode=1}
finally{await writeFile(out+'/results.json',JSON.stringify(results,null,2));await browser.close()}
