#!/usr/bin/env python3
"""Real-browser checks of the production React bundle against a real Go API.

Prerequisites: frontend/dist from npm ci && npm run build, Linux backend binary,
Python playwright, and Playwright Chromium. No Xray daemon or public VPS needed
for these UI-only scenarios. This test does NOT assert real VPN connectivity.
"""
from __future__ import annotations

import os
from pathlib import Path
import re
import socket
import subprocess
import tempfile
import time
from urllib.error import URLError
from urllib.request import urlopen

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
BINARY = Path(os.environ.get('ARCVPN_BINARY', str(ROOT / 'dist' / 'arcvpn-linux-amd64')))
SCREENSHOTS = ROOT / 'tests' / 'screenshots' / 'react'
WIDTHS = [320, 390, 768, 1024, 1440]
SECTIONS = [
    ('/', 'Обзор', 'Обзор'),
    ('/users', 'Пользователи', 'Пользователи'),
    ('/vpn', 'Подключения', 'Подключения'),
    ('/nodes', 'Серверы', 'Серверы'),
    ('/routing', 'Маршруты', 'Маршруты'),
    ('/security', 'Настройки', 'Настройки'),
    ('/events', 'Журнал', 'Журнал'),
    ('/backup', 'Резервные копии', 'Резервные копии'),
]


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return int(sock.getsockname()[1])


def await_ready(origin: str, server: subprocess.Popen[str]) -> None:
    for _ in range(120):
        if server.poll() is not None:
            raise RuntimeError(f'Go server exited early: code {server.returncode}')
        try:
            with urlopen(f'{origin}/healthz', timeout=0.5) as response:
                if response.status == 204:
                    return
        except (OSError, URLError):
            time.sleep(0.1)
    raise RuntimeError('Go server did not start')


def navigate(page, origin: str, path: str, heading: str) -> None:
    page.goto(origin + path, wait_until='networkidle', timeout=15000)
    page.get_by_role('heading', name=heading, exact=True).first.wait_for(timeout=15000)
    if page.get_by_role('heading', name='Не удалось открыть раздел').count():
        raise AssertionError(f'React render failure at {path}')


def main() -> None:
    if not BINARY.is_file():
        raise SystemExit('Missing Go binary. Run scripts/build-release.sh first.')
    subprocess.run([str(BINARY), 'frontend-check'], check=True, capture_output=True)
    SCREENSHOTS.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='arcvpn-react-e2e-') as data_dir:
        env = dict(os.environ, ARCVPN_DATA_DIR=data_dir)
        init = subprocess.run([str(BINARY), 'admin-init'], env=env, text=True,
                              capture_output=True, check=True)
        match = re.search(r'Password: (\S+)', init.stdout)
        if not match:
            raise RuntimeError('Could not read temporary admin password')
        password = match.group(1)
        port = free_port()
        origin = f'http://127.0.0.1:{port}'
        server = subprocess.Popen([
            str(BINARY), 'serve', '--listen', f'127.0.0.1:{port}',
            '--public-url', 'https://198.51.100.25:8443',
        ], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, text=True)
        try:
            await_ready(origin, server)
            with sync_playwright() as p:
                browser = p.chromium.launch(headless=True)
                context = browser.new_context(viewport={'width': 1440, 'height': 900})
                page = context.new_page()
                errors: list[str] = []
                page.on('pageerror', lambda exc: errors.append(str(exc)))
                navigate(page, origin, '/', 'Войти в панель')
                page.get_by_label('Логин').fill('admin')
                page.get_by_label('Пароль').fill(password)
                page.get_by_role('button', name='Войти').click()
                page.get_by_role('heading', name='Обзор', exact=True).wait_for(timeout=15000)

                for width in WIDTHS:
                    page.set_viewport_size({'width': width, 'height': 900})
                    for path, _, heading in SECTIONS:
                        navigate(page, origin, path, heading)
                        overflow = page.evaluate('document.documentElement.scrollWidth - document.documentElement.clientWidth')
                        if overflow > 1:
                            raise AssertionError(f'Horizontal page overflow at {width}px on {path}: {overflow}px')
                        page.screenshot(path=str(SCREENSHOTS / f'{width}-{path.strip("/") or "overview"}.png'))

                    # Test navigation itself, not only direct URL access.
                    navigate(page, origin, '/', 'Обзор')
                    if width <= 680:
                        page.get_by_role('button', name=re.compile('Открыть меню')).click()
                        page.get_by_role('dialog', name='Меню', exact=True).get_by_role('link', name='Серверы').click()
                    else:
                        page.locator('.sidebar').get_by_role('link', name='Серверы').click()
                    page.get_by_role('heading', name='Серверы', exact=True).wait_for(timeout=10000)
                    assert page.url.endswith('/nodes')

                page.set_viewport_size({'width': 1440, 'height': 900})
                navigate(page, origin, '/', 'Обзор')
                navigate(page, origin, '/security?section=interface', 'Настройки')
                page.get_by_label('Режим интерфейса', exact=True).select_option('advanced')
                assert page.evaluate("localStorage.getItem('arcnet-interface-mode')") == 'advanced'
                page.reload(wait_until='networkidle')
                assert page.get_by_label('Режим интерфейса', exact=True).input_value() == 'advanced'
                page.get_by_text('Тёмная · ArcNet v7', exact=True).wait_for()

                # Real stage-4 registration, followed by the non-destructive
                # drain operation. No VPS or shell command is executed.
                navigate(page, origin, '/nodes', 'Серверы')
                page.get_by_role('button', name='Добавить сервер', exact=True).click()
                page.get_by_label('Название', exact=True).fill('e2e-node')
                page.get_by_label('IP-адрес или DNS VPS').fill('9.9.9.9')
                page.get_by_role('button', name='Закрыть: Добавить сервер', exact=True).click()
                page.get_by_role('button', name='Показать ручную регистрацию').click()
                page.get_by_role('button', name='Выдать одноразовый токен (10 минут)').click()
                token_field = page.get_by_label('Токен регистрации (никому не передавать)')
                token_field.wait_for(timeout=10000)
                enrollment_token = token_field.input_value()
                assert len(enrollment_token) >= 40
                # The registration endpoint is agent-owned and intentionally
                # does not accept a browser CSRF token in place of enrollment.
                registration = context.request.post(origin + '/api/nodes/register', data={'token': enrollment_token})
                if registration.status != 201:
                    raise AssertionError(f'Node registration failed: HTTP {registration.status}')
                identity = registration.json()
                assert identity['node_id'] and identity['secret']
                page.get_by_role('button', name='Скрыть токен').click()
                page.get_by_role('button', name='Скрыть ручную регистрацию').click()
                page.get_by_role('button', name='Обновить', exact=True).click()
                row = page.get_by_role('row').filter(has_text='e2e-node')
                row.wait_for(timeout=10000)
                assert page.get_by_role('columnheader', name='Проверка VPN').count() == 1
                assert row.get_by_text('Нет подтверждённой внешней проверки').count() > 0
                row.get_by_role('button', name='Drain: скрыть ссылки').click()
                page.get_by_role('dialog', name='Подтвердите действие', exact=True).get_by_role('button', name='Подтвердить', exact=True).click()
                row.get_by_text('Drain: новые ссылки скрыты').wait_for(timeout=10000)
                # A direct diagnostic link is not available while drained.
                suppress = context.request.get(origin + '/api/nodes/' + identity['node_id'] + '/links')
                assert suppress.status == 409

                navigate(page, origin, '/users', 'Пользователи')
                page.get_by_role('button', name='Добавить пользователя').click()
                page.get_by_label('Имя пользователя').fill('test-client')
                page.get_by_role('button', name='Создать и настроить', exact=True).click()
                page.get_by_role('alert').wait_for(timeout=10000)  # no VPN configured
                user_list = context.request.get(origin + '/api/users')
                assert user_list.status == 200
                assert not any(user['name'] == 'test-client' for user in user_list.json()['items'])

                navigate(page, origin, '/routing', 'Маршруты')
                page.get_by_role('button', name='Серверы и каскады', exact=True).click()
                page.get_by_text('Локальный VPN ещё не настроен', exact=False).wait_for()
                if errors:
                    raise AssertionError('JS errors: ' + '; '.join(errors))
                context.close()
                browser.close()
                print('PASS: actual React bundle/API, auth, all routes, interface persistence, invalid form, node enrollment, '
                      'node drain, 320/390/768/1024/1440 responsiveness')
        finally:
            server.terminate()
            try:
                server.wait(timeout=8)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait(timeout=8)


if __name__ == '__main__':
    main()
