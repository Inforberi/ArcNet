/** Real disposable API/SQLite/auth. A stored synthetic user/config is required;
 * this browser fixture deliberately has no working Xray source or systemd. */
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir, readFile} from 'node:fs/promises';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin=process.env.ARCVPN_E2E_URL;
assert(origin && ['127.0.0.1','localhost'].includes(new URL(origin).hostname),'Disposable loopback fixture only');
const password=(await readFile(process.env.ARCVPN_E2E_PASSWORD_FILE,'utf8')).trim();
const browser=await chromium.launch({headless:true,...(process.env.ARCVPN_CHROMIUM?{executablePath:process.env.ARCVPN_CHROMIUM}:{})});
const context=await browser.newContext({viewport:{width:1440,height:1000},timezoneId:'Europe/Moscow'});
const page=await context.newPage();page.setDefaultTimeout(15000);
const errors=[];page.on('pageerror',error=>errors.push(error.message));
await mkdir('tests/screenshots/user-usage',{recursive:true});
try {
 await page.goto(origin,{waitUntil:'domcontentloaded'});
 await page.getByLabel('Логин',{exact:true}).fill('admin');await page.getByLabel('Пароль',{exact:true}).fill(password);await page.getByRole('button',{name:'Войти',exact:true}).click();
 await page.getByRole('heading',{name:'Обзор',exact:true}).waitFor();
 const session=await (await context.request.get(origin+'/api/session')).json();
 const api=(method,path,data)=>context.request.fetch(origin+path,{method,data,headers:{'X-CSRF-Token':session.csrf}});
 await page.goto(origin+'/users',{waitUntil:'domcontentloaded'});
 await page.getByText('Нет данных о расходе',{exact:true}).waitFor();
 assert.equal((await (await api('GET','/api/users/usage')).json()).source_fresh,false);
 await page.getByRole('button',{name:'Пакет / расход',exact:true}).click();
 let dialog=page.getByRole('dialog',{name:'Пакет · Usage fixture',exact:true});await dialog.waitFor();
 const unlimited=dialog.getByRole('checkbox',{name:'Без ограничения трафика',exact:true});assert(await unlimited.isChecked());
 await unlimited.uncheck();assert(await dialog.getByRole('button',{name:'Сохранить пакет',exact:true}).isDisabled());
 await unlimited.check();await dialog.getByText('Расширенные настройки',{exact:true}).click();
 await dialog.getByLabel('Действует до',{exact:true}).fill('2027-01-01T00:00');
 const concurrent=await api('PUT','/api/users/1/package',{quota_bytes:0,expires_at:1801515600,expected_version:0,reset_usage:false});assert.equal(concurrent.status(),200);
 await dialog.getByRole('button',{name:'Сохранить пакет',exact:true}).click();
 await dialog.getByText('Пакет изменён. Обновите данные перед сохранением.',{exact:true}).waitFor();
 assert.equal(await dialog.getByLabel('Действует до',{exact:true}).inputValue(),'2027-01-01T00:00');
 await dialog.getByRole('button',{name:'Обновить версию',exact:true}).click();
 assert.equal(await dialog.getByLabel('Действует до',{exact:true}).inputValue(),'2027-01-01T00:00');
 await dialog.getByLabel('Действует до',{exact:true}).fill('2027-03-01T00:00');
 await dialog.getByRole('button',{name:'Сохранить пакет',exact:true}).click();
 await dialog.getByText(/Пакет сохранён. Применение доступа ещё не подтверждено/).waitFor();
 let listing=await (await api('GET','/api/users/usage')).json();let saved=listing.items.find(item=>item.user_id===1);
 assert.equal(saved.version,2);assert.equal(saved.expires_at,Date.parse('2027-03-01T00:00:00+03:00')/1000);assert.equal(saved.observed_at,0);
 for(const theme of ['light','dark']){
  await page.evaluate(theme=>{document.documentElement.dataset.theme=theme},theme);
  for(const width of [320,390,768,1024,1440]){
   await page.setViewportSize({width,height:1000});const box=await dialog.boundingBox();assert(box && box.width<=width+1 && box.x>=-1,`dialog ${theme}/${width}`);
   assert(await dialog.evaluate(node=>node.scrollWidth<=node.clientWidth+1),`overflow ${theme}/${width}`);
   await page.screenshot({path:`tests/screenshots/user-usage/${theme}-${width}.png`,fullPage:true});
  }
 }
 await dialog.getByRole('button',{name:'Закрыть',exact:true}).click();
 const denied=await api('PUT','/api/users/1/package',{quota_bytes:1000000000,expires_at:0,expected_version:2,reset_usage:false});assert.equal(denied.status(),422);
 const expiry=await api('PUT','/api/users/1/package',{quota_bytes:0,expires_at:Math.floor(Date.now()/1000)-1,expected_version:2,reset_usage:false});assert.equal(expiry.status(),200);
 await page.getByText('Срок истёк',{exact:true}).waitFor();
 assert.equal((await api('GET','/api/users/1/subscription')).status(),404);
 assert.deepEqual(errors,[]);console.log('PASS user packages UI/API: unknown usage, finite-source guard, CAS/draft, pending apply, expiry/export denial, 5 widths × 2 themes. Synthetic config; no VPN enforcement proof.');
} finally {await context.close();await browser.close();}
