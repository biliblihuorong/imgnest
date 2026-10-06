"""Local fixture-only UI regression workflow. Requires Python Playwright + Chromium.
This script was prepared but browser execution was blocked in the authoring runtime.
"""
import argparse
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

parser=argparse.ArgumentParser()
parser.add_argument('--origin',default='http://127.0.0.1:8088')
parser.add_argument('--screenshots',default='web-vben/e2e/results')
parser.add_argument('--chromium',default=None)
args=parser.parse_args()
if args.origin not in ['http://127.0.0.1:8088','http://localhost:8088']:
    raise SystemExit('This workflow only supports its disposable local fixture server.')
out=Path(args.screenshots);out.mkdir(parents=True,exist_ok=True)
with sync_playwright() as p:
    options={'headless':True}
    if args.chromium:options['executable_path']=args.chromium
    browser=p.chromium.launch(**options)
    page=browser.new_page(viewport={'width':1440,'height':980})
    errors=[]
    page.on('pageerror',lambda error:errors.append(str(error)))
    def open_path(path):
        page.goto(args.origin+path,wait_until='networkidle')
    def screenshot(name):
        page.screenshot(path=str(out/(name+'.png')),full_page=True)
    open_path('/login')
    expect(page.get_by_placeholder('邮箱',exact=True)).to_be_visible()
    expect(page.locator('a[href="/register"]')).to_have_count(0)
    screenshot('login-desktop')
    page.get_by_placeholder('邮箱',exact=True).fill('user@example.test')
    page.get_by_placeholder('密码',exact=True).fill('fixture-password-123')
    page.get_by_role('button',name='登录',exact=True).click()
    page.wait_for_url('**/upload')
    page.wait_for_load_state('networkidle')
    screenshot('upload-desktop')
    for path,name in [('/images','images-empty'),('/albums','albums-empty'),('/tokens','account-tokens'),('/gallery','gallery-empty'),('/dashboard','personal-overview')]:
        open_path(path)
        expect(page.locator('body')).not_to_contain_text('页面加载失败')
        screenshot(name)
    open_path('/admin/users')
    page.wait_for_url('**/forbidden')
    expect(page.get_by_text('无权访问',exact=True)).to_be_visible()
    page.set_viewport_size({'width':390,'height':844})
    open_path('/upload');screenshot('upload-mobile')
    open_path('/albums');screenshot('albums-mobile')
    # New browser context is deliberately a separate fixture account/session.
    page.close()
    page=browser.new_page(viewport={'width':1440,'height':980})
    page.on('pageerror',lambda error:errors.append(str(error)))
    open_path('/login')
    page.get_by_placeholder('邮箱',exact=True).fill('admin@example.test')
    page.get_by_placeholder('密码',exact=True).fill('fixture-password-123')
    page.get_by_role('button',name='登录',exact=True).click()
    page.wait_for_url('**/dashboard');page.wait_for_load_state('networkidle')
    screenshot('admin-overview')
    for route in ['users','groups','storages','policies','settings','images']:
        open_path('/admin/'+route)
        screenshot('admin-'+route)
    assert not errors, '\n'.join(errors)
    browser.close()
print('Fixture smoke scenarios completed; screenshots saved to',out)
