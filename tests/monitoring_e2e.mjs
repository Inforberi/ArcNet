/** Real disposable API/SQLite + UI. Configured mode uses a seeded config,
 * never claims Xray, target reachability, device limits or VPN traffic. */
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir, readFile} from 'node:fs/promises';
const {chromium} = createRequire(import.meta.url)('playwright');
const origin = process.env.ARCVPN_E2E_URL;
assert(origin && process.env.ARCVPN_E2E_PASSWORD_FILE, 'Disposable loopback backend required');
assert(['127.0.0.1', 'localhost'].includes(new URL(origin).hostname), 'Only disposable loopback fixtures');
const password = (await readFile(process.env.ARCVPN_E2E_PASSWORD_FILE, 'utf8')).trim();
const browser = await chromium.launch({headless: true, ...(process.env.ARCVPN_CHROMIUM ? {executablePath: process.env.ARCVPN_CHROMIUM} : {})});
const context = await browser.newContext({viewport: {width: 1440, height: 1000}});
const page = await context.newPage(); page.setDefaultTimeout(20000);
const errors = []; page.on('pageerror', error => errors.push(error.message));
await mkdir('tests/screenshots/monitoring', {recursive: true});
try {
 await page.goto(origin, {waitUntil: 'domcontentloaded'});
 await page.getByLabel('Логин', {exact:true}).fill('admin'); await page.getByLabel('Пароль', {exact:true}).fill(password);
 await page.getByRole('button', {name:'Войти',exact:true}).click();
 await page.getByText(/Обновляется автоматически · каждые/).waitFor();
 const session = await (await context.request.get(origin+'/api/session')).json();
 const api = (method,path,data) => context.request.fetch(origin+path,{method,data,headers:{'X-CSRF-Token':session.csrf}});
 if (process.env.ARCVPN_E2E_CONFIGURED !== '1' || process.env.ARCVPN_E2E_MONITORING === '1') {
  assert.equal((await (await api('GET','/api/settings/monitoring')).json()).interval_seconds,5);
  await page.getByText('Расширенные настройки мониторинга',{exact:true}).click();
  await page.getByLabel('Интервал обновления',{exact:true}).selectOption('10');
  await page.getByRole('button',{name:'Сохранить интервал',exact:true}).click();
  await page.getByText('Обновляется автоматически · каждые 10 с',{exact:true}).waitFor();
  assert.equal((await (await api('GET','/api/settings/monitoring')).json()).interval_seconds,10);
  await page.reload({waitUntil:'domcontentloaded'});
  await page.getByText('Обновляется автоматически · каждые 10 с',{exact:true}).waitFor();
  assert.equal((await api('PUT','/api/settings/monitoring',{interval_seconds:5})).status(),200);
  await page.getByText('Обновляется автоматически · каждые 5 с',{exact:true}).waitFor();
  // A second authenticated browser session receives the same saved cadence.
  const second = await context.newPage(); await second.goto(origin,{waitUntil:'domcontentloaded'});
  await second.getByText('Обновляется автоматически · каждые 5 с',{exact:true}).waitFor(); await second.close();
  // Fresh samples arrive without clicking Refresh or reloading the page.
  const first = (await (await api('GET','/api/servers/local/telemetry?history=0')).json()).server.telemetry.snapshot.sampled_at;
  await page.waitForResponse(async response => {
   if (!response.url().endsWith('/api/servers/local/telemetry?history=0') || response.status() !== 200) return false;
   return (await response.json()).server.telemetry.snapshot.sampled_at > first;
  },{timeout:15000});
  // Simulate a dropped stream; native reconnect restores it.
  const cdp = await context.newCDPSession(page);
  await cdp.send('Network.enable'); await cdp.send('Network.emulateNetworkConditions',{offline:true,latency:0,downloadThroughput:-1,uploadThroughput:-1});
  await page.getByText(/Восстанавливаем подписку/).waitFor();
  await cdp.send('Network.emulateNetworkConditions',{offline:false,latency:0,downloadThroughput:-1,uploadThroughput:-1});
  await page.getByText('Обновляется автоматически · каждые 5 с',{exact:true}).waitFor();
 }
 await page.goto(origin+'/vpn',{waitUntil:'domcontentloaded'});
 await page.getByRole('heading',{name:'Подключения',exact:true}).waitFor();
 if (process.env.ARCVPN_E2E_CONFIGURED !== '1') {
  await page.getByRole('button',{name:'Добавить VLESS + Reality TCP',exact:true}).click();
 }
 const advanced = page.locator('details').filter({has:page.locator('summary').getByText('Расширенные настройки',{exact:true})}).first();
 await advanced.waitFor(); assert.equal(await advanced.getAttribute('open'),null);
 await advanced.locator('summary').first().click();
 if (process.env.ARCVPN_E2E_CONFIGURED === '1') {
  await page.getByLabel('TCP-порт',{exact:true}).fill('23001');
  await page.getByLabel('SNI',{exact:true}).fill('manual.example.com');
  await page.getByLabel('Тип подключения',{exact:true}).selectOption('wireguard');
  assert.equal(await page.getByLabel('UDP-порт',{exact:true}).inputValue(),'51820');
  await page.getByLabel('UDP-порт',{exact:true}).fill('53001');
  await page.getByLabel('Тип подключения',{exact:true}).selectOption('hysteria2');
  assert.equal(await page.getByLabel('UDP-порт',{exact:true}).inputValue(),'8444');
  assert.equal(await page.getByLabel('SNI',{exact:true}).count(),0);
  await page.getByLabel('Тип подключения',{exact:true}).selectOption('wireguard');
  assert.equal(await page.getByLabel('UDP-порт',{exact:true}).inputValue(),'53001');
  await page.getByLabel('Тип подключения',{exact:true}).selectOption('vless-xhttp');
  assert.equal(await page.getByLabel('TCP-порт',{exact:true}).inputValue(),'23001');
  assert.equal(await page.getByLabel('SNI',{exact:true}).inputValue(),'manual.example.com');
 } else {
  assert.equal(await page.getByLabel('Публичный IP VPS',{exact:true}).inputValue(),'198.51.100.25');
  assert.equal(await page.getByLabel('TCP-порт',{exact:true}).inputValue(),'443');
  await page.getByLabel('Reality target и SNI',{exact:true}).fill('manual.example.com');
  assert(await page.getByRole('button',{name:'Создать и активировать',exact:true}).isEnabled());
  // Let any automatic diagnostic finish: manual edits must survive it.
  await page.waitForTimeout(5500);
  assert.equal(await page.getByLabel('Reality target и SNI',{exact:true}).inputValue(),'manual.example.com');
 }
 for (const theme of ['light','dark']) {
  await context.addCookies([{name:'arcvpn_theme',value:theme,url:origin}]);
  for (const width of [320,390,768,1024,1440]) {
   await page.setViewportSize({width,height:900}); await page.reload({waitUntil:'domcontentloaded'});
   await page.getByRole('heading',{name:'Подключения',exact:true}).waitFor();
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),`overflow ${theme}/${width}`);
   await page.screenshot({path:`tests/screenshots/monitoring/${process.env.ARCVPN_E2E_CONFIGURED==='1'?'configured':'setup'}-${theme}-${width}.png`,fullPage:true});
  }
 }
 assert.deepEqual(errors,[]); console.log('Monitoring subscription, cadence, reconnect, forms and responsive UI: PASS');
} finally {await context.close(); await browser.close();}
