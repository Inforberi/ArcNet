#!/usr/bin/env python3
"""Browser rendering against the real Go API via an isolated Playwright RPC bridge.

Chromium in this execution environment blocks all navigations. We render the
real HTML/CSS/JS via set_content and proxy JS fetch through Python to a real
loopback Go process. This does NOT test browser HTTP cookies, TLS or Origin.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
import urllib.request
import urllib.error
from playwright.sync_api import sync_playwright

ROOT=Path(__file__).resolve().parents[1]
SCREENS=ROOT/'tests/screenshots'
SCREENS.mkdir(exist_ok=True)
BINARY=ROOT/'dist/arcvpn-linux-amd64'
PORT=19104
BACKEND=f'http://127.0.0.1:{PORT}'


def main():
  with tempfile.TemporaryDirectory() as tmp:
    env=dict(os.environ, ARCVPN_DATA_DIR=tmp)
    admin=subprocess.run([str(BINARY),'admin-init'],env=env,text=True,capture_output=True,check=True)
    pwd=re.search(r'Password: (\S+)',admin.stdout).group(1)
    srv=subprocess.Popen([str(BINARY),'serve','--listen',f'127.0.0.1:{PORT}','--public-url','https://198.51.100.25:8443'],env=env,stderr=subprocess.DEVNULL)
    try:
      for _ in range(70):
        try:
          with urllib.request.urlopen(BACKEND+'/healthz',timeout=2) as r:
            if r.status==204:break
        except Exception:time.sleep(.1)
      else:raise RuntimeError('Go server unavailable')
      cookies={}; appearance={'arcvpn_theme':'dark'}
      def bridge(request):
        path=request['path'];method=request['method'];body=request.get('body')
        assert path.startswith('/api/'),path
        headers={k:v for k,v in request.get('headers',{}).items() if k.lower()!='origin'}
        headers['Host']='arcvpn.test'
        if cookies:headers['Cookie']='; '.join(k+'='+v for k,v in cookies.items())
        payload=body.encode('utf-8') if body is not None else None
        req=urllib.request.Request(BACKEND+path, data=payload, headers=headers,method=method)
        try:resp=urllib.request.urlopen(req,timeout=8)
        except urllib.error.HTTPError as e:resp=e
        with resp:
          new_cookie=resp.headers.get('Set-Cookie')
          if new_cookie:
            key,val=new_cookie.split(';',1)[0].split('=',1)
            if val:cookies[key]=val
            else:cookies.pop(key,None)
          return {'status':resp.status,'text':resp.read().decode('utf-8','replace')}
      with sync_playwright() as p:
        browser=p.chromium.launch(headless=True,executable_path='/usr/bin/chromium',args=['--no-sandbox'])
        page=browser.new_page(viewport={'width':1440,'height':900})
        page_errors=[]
        page.on('pageerror',lambda err:page_errors.append(str(err)))
        page.expose_function('bridgeFetch',bridge)
        html=(ROOT/'web/index.html').read_text().replace('<link rel="stylesheet" href="/style.css">','').replace('<script src="/theme.js" defer></script><script src="/app.js" defer></script>','')
        page.set_content(html)
        page.add_style_tag(content=(ROOT/'web/style.css').read_text())
        page.evaluate('''() => {
          const jar = {arcvpn_theme:'dark'};
          Object.defineProperty(document,'cookie',{configurable:true,get:()=>Object.entries(jar).map(([k,v])=>k+'='+v).join('; '),set:value=>{const [k,v]=value.split(';')[0].split('=');jar[k.trim()]=v.trim();}});
          window.fetch=async (path,options={})=>{
            const r=await window.bridgeFetch({path,method:options.method||'GET',headers:options.headers||{},body:options.body??null});
            return new Response(r.text,{status:r.status===204?200:r.status,headers:{'Content-Type':'application/json'}});
          };
        }''')
        page.add_script_tag(content=(ROOT/'web/theme.js').read_text())
        page.add_script_tag(content=(ROOT/'web/app.js').read_text())
        page.locator('input[name="username"]').fill('admin')
        page.locator('input[name="password"]').fill(pwd)
        page.get_by_role('button',name='Войти').click()
        page.get_by_role('heading',name='Обзор').wait_for(timeout=10000)
        page.screenshot(path=str(SCREENS/'overview-dark-1440.png'),full_page=True)
        page.get_by_role('button',name='Светлая тема').first.click()
        assert page.evaluate('document.documentElement.dataset.theme')=='light'
        assert 'arcvpn_theme=light' in page.evaluate('document.cookie')
        page.screenshot(path=str(SCREENS/'overview-light-1440.png'),full_page=True)
        page.get_by_role('button',name=re.compile('Серверы')).first.click()
        page.get_by_role('heading',name='Серверы').wait_for(timeout=10000)
        page.locator('input[name="name"]').fill('Amsterdam-1')
        page.get_by_role('button',name='Получить команду установки агента').click()
        page.get_by_text('curl -fsSL',exact=False).wait_for(timeout=10000)
        for w in (1440,768,390,320):
          page.set_viewport_size({'width':w,'height':850})
          page.wait_for_timeout(100)
          over=page.evaluate('document.documentElement.scrollWidth-document.documentElement.clientWidth')
          assert over<=1,f'horizontal overflow at {w}px: {over}px'
          page.screenshot(path=str(SCREENS/f'nodes-light-{w}.png'),full_page=True)
        page.get_by_role('button',name='Тёмная тема').first.click()
        assert page.evaluate('document.documentElement.dataset.theme')=='dark'
        assert not page_errors,page_errors
        print('PASS: browser UI with real Go API (bridge): admin login, node command, theme, 1440/768/390/320px, no JS errors')
        browser.close()
    finally:
      srv.terminate()
      try:srv.wait(timeout=8)
      except subprocess.TimeoutExpired:srv.kill()
if __name__=='__main__':main()
