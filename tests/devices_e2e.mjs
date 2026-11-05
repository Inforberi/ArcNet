// Opt-in TestDevicesBrowserFixture: real HTTP/auth/SQLite, no live VPN engine.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir} from 'node:fs/promises';
const {chromium}=createRequire(import.meta.url)('playwright');
const origin=process.env.ARCVPN_E2E_URL;
assert(origin&&new URL(origin).hostname==='127.0.0.1','Disposable loopback fixture only');
const browser=await chromium.launch({headless:true,...(process.env.ARCVPN_CHROMIUM?{executablePath:process.env.ARCVPN_CHROMIUM}:{})});
const context=await browser.newContext({viewport:{width:1440,height:1000}});
await context.addCookies([{name:'arcvpn_session',value:'private-fixture-session',url:origin,httpOnly:true,sameSite:'Strict'}]);
const page=await context.newPage();page.setDefaultTimeout(12000);
const errors=[];page.on('pageerror',e=>errors.push(e.message));
const api=(method,path,data)=>context.request.fetch(origin+path,{method,data,headers:{'X-CSRF-Token':'fixture-csrf'}});
await mkdir('tests/screenshots/devices',{recursive:true});
try {
 await page.goto(origin+'/users',{waitUntil:'domcontentloaded'});
 await page.getByRole('button',{name:'Устройства',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'Устройства · Device fixture',exact:true});await dialog.waitFor();
 await dialog.getByText(/При добавлении первого устройства общий ключ/).waitFor();
 assert.equal(await dialog.getByLabel('Имя устройства',{exact:true}).inputValue(),'Устройство 1');
 page.once('dialog',d=>d.accept());await dialog.getByRole('button',{name:'Добавить устройство',exact:true}).click();
 await dialog.getByText('Изменение сохранено. Применение VPN-службой ещё не подтверждено.',{exact:true}).waitFor();
 await dialog.getByText('Устройство 1',{exact:true}).waitFor();
 let state=await (await api('GET','/api/users/1/devices')).json();assert(state.managed);assert.equal(state.active,1);
 assert.equal((await context.request.get(origin+'/sub/'+'t'.repeat(43))).status(),404);
 await dialog.getByText('Расширенные настройки',{exact:true}).click();
 await dialog.getByLabel('Лимит активных доступов',{exact:true}).fill('1');
 page.once('dialog',d=>d.accept());await dialog.getByRole('button',{name:'Сохранить лимит',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('input[type=number]')?.value==='1');
 await page.waitForTimeout(150);assert(await dialog.getByRole('button',{name:'Добавить устройство',exact:true}).isDisabled());
 state=await (await api('GET','/api/users/1/devices')).json();assert.equal(state.limit,1);
 assert.equal((await api('POST','/api/users/1/devices',{name:'bypass',expected_version:state.version})).status(),409);
 // A concurrent limit change must not erase the administrator's entered draft.
 await dialog.getByLabel('Лимит активных доступов',{exact:true}).fill('3');
 assert.equal((await api('PUT','/api/users/1/devices',{limit:2,expected_version:state.version})).status(),200);
 page.once('dialog',d=>d.accept());await dialog.getByRole('button',{name:'Сохранить лимит',exact:true}).click();
 await dialog.getByText('Список изменён. Обновите данные перед повтором.',{exact:true}).waitFor();
 assert.equal(await dialog.getByLabel('Лимит активных доступов',{exact:true}).inputValue(),'3');
 await dialog.getByLabel('Лимит активных доступов',{exact:true}).fill('1');
 page.once('dialog',d=>d.accept());await dialog.getByRole('button',{name:'Сохранить лимит',exact:true}).click();
 await dialog.getByText('Изменение сохранено. Применение VPN-службой ещё не подтверждено.',{exact:true}).waitFor();
 await dialog.getByRole('button',{name:'Подписка',exact:true}).click();
 const url=await dialog.getByLabel('Подписка устройства',{exact:true}).inputValue();
 const devicePath=new URL(url).pathname;
 assert.equal((await context.request.get(origin+devicePath)).status(),200);
 for(const theme of ['light','dark']) {
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  for(const width of [320,390,768,1024,1440]) {
   await page.setViewportSize({width,height:1000});const box=await dialog.boundingBox();assert(box&&box.x>=-1&&box.width<=width+1,`dialog ${theme}/${width}`);
   assert(await dialog.evaluate(n=>n.scrollWidth<=n.clientWidth+1),`overflow ${theme}/${width}`);
   const input=await dialog.getByLabel('Подписка устройства',{exact:true}).boundingBox();assert(input&&input.width>=box.width*.75,`subscription field ${theme}/${width}`);
   await page.screenshot({path:`tests/screenshots/devices/${theme}-${width}.png`,fullPage:true});
  }
 }
 page.once('dialog',d=>d.accept());await dialog.getByRole('button',{name:'Отозвать',exact:true}).click();
 await dialog.getByText('Доступ отозван',{exact:true}).waitFor();
 assert.equal((await context.request.get(origin+devicePath)).status(),404);
 await dialog.getByRole('button',{name:'Добавить устройство',exact:true}).click();
 await dialog.getByText('Устройство 2',{exact:true}).waitFor();
 state=await (await api('GET','/api/users/1/devices')).json();assert.equal(state.active,1);assert.equal(state.items.length,2);
 assert.equal((await api('PUT','/api/users/1/devices',{limit:2,expected_version:0})).status(),409);
 await dialog.getByRole('button',{name:'Закрыть: Устройства · Device fixture',exact:true}).click();
 assert(await page.getByRole('button',{name:'Общий ключ отключён',exact:true}).isDisabled());
 assert.deepEqual(errors,[]);
 console.log('PASS devices UI/API: migration/common-key retirement, individual subscription, limit/API bypass denial, revoke/slot reuse, CAS, pending apply, 5 widths × 2 themes; no VPN enforcement proof.');
}finally {
 await context.request.post(origin+'/fixture-done').catch(()=>{});await browser.close();
}
