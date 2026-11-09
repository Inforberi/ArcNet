// Disposable Go/SQLite + production React. Engines in this fixture are simulated.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
const {chromium}=createRequire(import.meta.url)('playwright');
const base='http://127.0.0.1:18394',out='/private/tmp/arcnet-operations-review';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',headless:true});
const page=await browser.newPage({viewport:{width:1440,height:1000},reducedMotion:'reduce'});page.setDefaultTimeout(10000);
const result={steps:[],widths:[],errors:[],scope:'Local Go/SQLite + React; fixture engines simulated. No VPS.'};page.on('pageerror',e=>result.errors.push(e.message));
const dialog=()=>page.getByRole('dialog').last();
async function step(name,fn){await fn();result.steps.push(name);console.log('PASS '+name)}
const name='Operations '+Date.now();
try{
 await page.goto(base+'/users');await page.getByLabel('Логин',{exact:true}).fill('browser-owner');await page.getByLabel('Пароль',{exact:true}).fill('disposable-browser-password-12345');await page.getByRole('button',{name:'Войти',exact:true}).click();await page.getByRole('heading',{name:'Пользователи',exact:true}).waitFor();
 await step('create, one navigation, draft survives all sections and pending runtime save',async()=>{
  await page.getByRole('button',{name:'Добавить пользователя',exact:true}).click();await dialog().getByLabel('Имя пользователя',{exact:true}).fill(name);await dialog().getByRole('button',{name:'Создать и настроить',exact:true}).click();await dialog().getByRole('navigation',{name:'Разделы пользователя',exact:true}).waitFor();assert.equal(await dialog().locator('nav').count(),1);
  await dialog().getByLabel('Имя пользователя',{exact:true}).fill(name+' saved');for(const tab of ['Доступ','Лимиты и срок','Активность','Основное'])await dialog().getByRole('navigation').getByRole('button',{name:tab,exact:true}).click();assert.equal(await dialog().getByLabel('Имя пользователя',{exact:true}).inputValue(),name+' saved');await dialog().getByRole('button',{name:'Сохранить изменения',exact:true}).click();await dialog().getByText('Все настройки сохранены.',{exact:false}).waitFor();await dialog().getByRole('button',{name:'Закрыть',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});await page.getByRole('button',{name:name+' saved',exact:true}).waitFor();
 });
 await step('refresh failure cannot turn a committed toggle into a mutation failure',async()=>{
  let committed=false;await page.route('**/api/users/*',async r=>{if(r.request().method()==='PATCH'){const response=await r.fetch();assert.equal(response.status(),200);committed=true;return r.fulfill({response})}return r.continue()});
  await page.route('**/api/users',r=>committed&&r.request().method()==='GET'?r.fulfill({status:503,json:{error:'Fixture read failure'}}):r.continue());
  await page.getByRole('switch',{name:'Отключить доступ · '+name+' saved',exact:true}).click();await page.getByRole('status').filter({hasText:'Изменение применено, список не обновлён'}).waitFor();assert.equal(await page.getByRole('alert').count(),0);await page.unroute('**/api/users');await page.unroute('**/api/users/*');await page.getByRole('button',{name:'Обновить данные',exact:true}).click();await page.getByRole('switch',{name:'Включить доступ · '+name+' saved',exact:true}).waitFor();
 });
 await step('context delete, single confirmation, reload confirms deletion',async()=>{
  await page.getByRole('button',{name:'Действия пользователя · '+name+' saved',exact:true}).click();await page.getByRole('group',{name:'Действия пользователя · '+name+' saved',exact:true}).getByRole('button',{name:'Удалить пользователя',exact:true}).click();const confirm=page.getByRole('dialog',{name:'Удалить пользователя?',exact:true});await confirm.getByRole('button',{name:'Удалить пользователя',exact:true}).click();await confirm.waitFor({state:'hidden'});await page.getByRole('button',{name:name+' saved',exact:true}).waitFor({state:'hidden'});await page.reload();await page.getByRole('heading',{name:'Пользователи',exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:name+' saved',exact:true}).count(),0);
 });
 await step('connection delete is visible with explanation; menu Escape restores focus',async()=>{
  await page.goto(base+'/vpn');const opener=page.getByRole('button',{name:'Действия подключения · 443',exact:true});await opener.click();const menu=page.getByRole('group',{name:'Действия подключения · 443',exact:true});assert.equal(await menu.getByRole('button',{name:'Удалить подключение',exact:true}).isDisabled(),true);await menu.getByText('Сначала отключите подключение',{exact:false}).waitFor();await page.keyboard.press('Escape');assert.equal(await opener.evaluate(el=>el===document.activeElement),true);
 });
 await step('responsive users, connections and flat editor',async()=>{
  for(const width of [320,375,390,430,768,1024,1280,1440,1600,1920]){
   await page.setViewportSize({width,height:900});for(const path of ['/users','/vpn']){await page.goto(base+path);await page.locator('table tbody tr').first().waitFor();const bounds=await page.evaluate(()=>({w:document.documentElement.clientWidth,s:document.documentElement.scrollWidth}));assert.ok(bounds.s<=bounds.w+1,JSON.stringify({width,path,...bounds}));if([390,1440].includes(width))await page.screenshot({path:out+'/'+path.slice(1)+'-'+width+'.png',fullPage:true})}
   await page.goto(base+'/users');await page.getByRole('button',{name:/^Настроить ·/}).first().click();await dialog().getByLabel('Имя пользователя',{exact:true}).waitFor();for(const tab of ['Доступ','Лимиты и срок','Активность','Основное'])await dialog().getByRole('navigation').getByRole('button',{name:tab,exact:true}).click();const footer=await dialog().getByRole('button',{name:'Сохранить изменения',exact:true}).boundingBox();assert.ok(footer.y>=0&&footer.y+footer.height<=901);for(let i=0;i<8;i++){await page.keyboard.press('Tab');assert.equal(await dialog().evaluate(el=>el.contains(document.activeElement)),true)}await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});result.widths.push(width);
  }
 });
 assert.deepEqual(result.errors,[]);console.log(JSON.stringify({steps:result.steps.length,widths:result.widths.length,errors:result.errors.length}));
}catch(e){result.failure=e.message;console.error(e.stack);await page.screenshot({path:out+'/failure.png',fullPage:true}).catch(()=>{});process.exitCode=1}
finally{await writeFile(out+'/results.json',JSON.stringify(result,null,2));await browser.close()}
