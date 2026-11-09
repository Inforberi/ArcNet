// Targeted regression after shared layout/focus/feedback changes.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
const require=createRequire(import.meta.url),{chromium}=require('playwright');
const base=process.env.ARCVPN_E2E_URL||'http://127.0.0.1:18394',output=process.env.ARCVPN_E2E_OUTPUT||'/private/tmp/arcnet-ux-review';
assert.ok(['127.0.0.1','localhost'].includes(new URL(base).hostname));await mkdir(output,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.ARCVPN_CHROMIUM||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true});
const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage();page.setDefaultTimeout(8000);
const results={steps:[],widths:[],errors:[]};page.on('pageerror',e=>results.errors.push(e.message));
const dialog=()=>page.getByRole('dialog').last();
async function step(name,fn){await fn();results.steps.push(name);console.log('PASS '+name)}
async function goto(path){await page.goto(base+path);await page.locator('.main>.top').waitFor();}
try{
 await page.goto(base+'/users');await page.getByLabel('Логин',{exact:true}).fill('browser-owner');await page.getByLabel('Пароль',{exact:true}).fill('disposable-browser-password-12345');await page.getByRole('button',{name:'Войти',exact:true}).click();await page.locator('.users-toolbar').waitFor();
 await step('user filter labels fit their controls at mobile/tablet/desktop widths',async()=>{
  for(const width of [320,375,390,430,768,1024,1440]){
   await page.setViewportSize({width,height:900});const controls=page.getByRole('group',{name:'Фильтр доступа',exact:true}).getByRole('button');
   for(const control of await controls.all()){const fits=await control.evaluate(el=>{const range=document.createRange();range.selectNodeContents(el);const text=range.getBoundingClientRect(),box=el.getBoundingClientRect();return text.left>=box.left&&text.right<=box.right+1&&text.height<=box.height});assert.equal(fits,true,`Filter text clipped at ${width}`)}
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth+1),true);results.widths.push(width);if(width===390)await page.screenshot({path:output+'/users-390-final.png',fullPage:true});
  }
 });
 await step('user editor heading stays anchored and save remains reachable across sections',async()=>{
  for(const width of [390,1440]){
   await page.setViewportSize({width,height:900});await page.getByRole('button',{name:/^Настроить ·/}).first().click();await dialog().getByLabel('Имя пользователя',{exact:true}).waitFor();const initial=await dialog().locator('.dialog-heading').boundingBox();
   for(const name of ['Пакет и срок','Подключения','Профиль']){await dialog().getByRole('button',{name,exact:true}).click();const heading=await dialog().locator('.dialog-heading').boundingBox();assert.ok(Math.abs(heading.y-initial.y)<2);const button=await dialog().getByRole('button',{name:'Сохранить изменения',exact:true}).boundingBox();assert.ok(button.y>=0&&button.y+button.height<=900)}
   await page.screenshot({path:`${output}/user-editor-${width}-final.png`});await dialog().getByRole('button',{name:/Закрыть:/}).click();await page.getByRole('dialog').waitFor({state:'hidden'});
  }
 });
 await step('403 offers no useless retry; 429 respects Retry-After without automatic requests',async()=>{
  let count=0;await page.route('**/api/events',route=>{count++;return route.fulfill({status:403,json:{error:'Тест: доступ запрещён'}})});await goto('/events');await page.getByRole('alert').filter({hasText:'Тест: доступ запрещён'}).waitFor();assert.equal(await page.getByRole('button',{name:'Повторить загрузку',exact:true}).count(),0);assert.equal(count,1);await page.unroute('**/api/events');
  count=0;await page.route('**/api/events',route=>{count++;return route.fulfill({status:429,headers:{'Retry-After':'1'},json:{error:'Тест: лимит запросов'}})});await goto('/events');const retry=page.getByRole('button',{name:'Повторить загрузку',exact:true});await retry.waitFor();assert.equal(await retry.isDisabled(),true);await page.waitForFunction(()=>document.querySelector('.error-state button')?.disabled===false);assert.equal(count,1);await page.unroute('**/api/events');await retry.click();await page.getByRole('alert').waitFor({state:'hidden'});
 });
 await step('one clipboard confirmation, one modal notification, explicit dismissal',async()=>{
  await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async()=>{}}}));await goto('/users');await page.getByRole('button',{name:/^Подключить ·/}).first().click();const copy=dialog().getByRole('button',{name:'Копировать подписку',exact:true});await copy.click();await dialog().getByText('Ссылка скопирована',{exact:true}).waitFor();await copy.click();assert.equal(await dialog().getByText('Ссылка скопирована',{exact:true}).count(),1);await dialog().getByRole('button',{name:/Закрыть:/}).click();await page.getByRole('dialog').waitFor({state:'hidden'});
  await page.getByRole('button',{name:'Добавить пользователя',exact:true}).click();await dialog().getByLabel('Имя пользователя',{exact:true}).fill(`Toast audit ${Date.now()}`);await dialog().getByRole('button',{name:'Создать и настроить',exact:true}).click();await page.getByRole('dialog',{name:/Пользователь · Toast audit/}).waitFor();await dialog().getByText('Пользователь создан',{exact:true}).waitFor();assert.equal(await page.locator('.feedback').count(),1);await dialog().getByRole('button',{name:'Закрыть уведомление',exact:true}).click();assert.equal(await page.locator('.feedback').count(),0);await dialog().getByRole('button',{name:/Закрыть:/}).click();await page.getByRole('dialog').waitFor({state:'hidden'});
 });
 await step('mobile login/MFA validation and errors preserve input and return focus',async()=>{
  const authContext=await browser.newContext({viewport:{width:375,height:812},reducedMotion:'reduce'}),auth=await authContext.newPage();
  await auth.route('**/api/session',route=>route.fulfill({status:401,json:{error:'login required'}}));await auth.route('**/api/setup',route=>route.fulfill({json:{required:false,available:true}}));let calls=0;
  await auth.route('**/api/login',route=>{calls++;return calls===1?route.fulfill({json:{ok:false,mfa_required:true}}):route.fulfill({status:422,json:{error:'Тест: неверный код'}})});
  await auth.goto(base);await auth.getByLabel('Логин',{exact:true}).fill('fixture');await auth.getByLabel('Пароль',{exact:true}).fill('fixture-password');await auth.getByRole('button',{name:'Войти',exact:true}).click();const code=auth.getByLabel('Код подтверждения',{exact:true});await code.waitFor();assert.equal(await code.evaluate(el=>document.activeElement===el),true);await code.fill('123');await auth.getByRole('button',{name:'Подтвердить',exact:true}).click();assert.equal(calls,1);await code.fill('123456');await auth.getByRole('button',{name:'Подтвердить',exact:true}).click();await auth.getByRole('alert').filter({hasText:'Тест: неверный код'}).waitFor();assert.equal(await code.inputValue(),'123456');await auth.getByRole('button',{name:'Назад',exact:true}).click();assert.equal(await auth.getByLabel('Логин',{exact:true}).inputValue(),'fixture');assert.equal(await auth.getByLabel('Пароль',{exact:true}).inputValue(),'');await authContext.close();
 });
 assert.deepEqual(results.errors,[]);console.log(JSON.stringify({steps:results.steps.length,widths:results.widths.length,frontendErrors:results.errors.length}));
}catch(error){results.failure=error.message;await page.screenshot({path:output+'/layout-failure.png',fullPage:true}).catch(()=>{});console.error(error.stack);process.exitCode=1}
finally{await writeFile(output+'/layout-results.json',JSON.stringify(results,null,2));await browser.close()}
