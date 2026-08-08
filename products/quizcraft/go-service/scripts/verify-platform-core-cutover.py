#!/usr/bin/env python3
import getpass
import json
import os
import re
import secrets
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from http.cookiejar import CookieJar


def required(name):
    value = os.environ.get(name, '')
    if not value:
        raise SystemExit(f'{name} is required')
    return value


account_origin = required('PLATFORM_ACCOUNT_ORIGIN').rstrip('/')
console_origin = required('CONSOLE_ORIGIN').rstrip('/')
student_email = required('CUTOVER_TEST_EMAIL').lower()
if not account_origin.startswith('https://') or not console_origin.startswith('https://'):
    raise SystemExit('Platform Core and Console verification require HTTPS')
cookies = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))


def request(url, method='GET', payload=None, idempotency=None, expected_status=200):
    body = None if payload is None else json.dumps(payload).encode()
    parsed = urllib.parse.urlparse(url)
    headers = {'Accept': 'application/json', 'Origin': f'{parsed.scheme}://{parsed.netloc}'}
    if body is not None:
        headers['Content-Type'] = 'application/json'
    if idempotency:
        headers['Idempotency-Key'] = idempotency
    try:
        response = opener.open(
            urllib.request.Request(url, data=body, headers=headers, method=method),
            timeout=15,
        )
    except urllib.error.HTTPError as error:
        if error.code != expected_status:
            raise SystemExit(
                f'{method} {parsed.path} returned HTTP {error.code}; expected {expected_status}'
            ) from None
        response = error
    if response.status != expected_status:
        raise SystemExit(
            f'{method} {parsed.path} returned HTTP {response.status}; expected {expected_status}'
        )
    return response, response.read()


nonce = secrets.token_urlsafe(18)
response, _ = request(
    f'{account_origin}/api/v1/auth/email-codes', 'POST',
    {'email': student_email, 'purpose': 'login', 'client_id': 'console-gateway'},
    f'cutover_mail_{nonce}',
    202,
)

try:
    with open('/dev/tty', encoding='utf-8'):
        pass
except OSError as error:
    raise SystemExit('manual real-mail verification requires an interactive terminal') from error
code = getpass.getpass('Enter the current code from the real HENU Kit verification email: ')
if not re.fullmatch(r'[0-9]{6,10}', code):
    raise SystemExit('the manually entered verification code has an invalid shape')

response, body = request(
    f'{account_origin}/api/v1/auth/email-codes/verify', 'POST',
    {'email': student_email, 'code': code, 'purpose': 'login'},
    f'cutover_verify_{nonce}',
)
data = json.loads(body)['data']
expiry = datetime.fromisoformat(data['session_expires_at'].replace('Z', '+00:00'))
remaining = (expiry - datetime.now(timezone.utc)).total_seconds()
if response.status != 200 or not data.get('user', {}).get('id') or not 29.9 * 86400 <= remaining <= 30 * 86400 + 60:
    raise SystemExit('30-day Core Session verification failed')

response, _ = request(f'{console_origin}/api/v1/auth/login?return_to=%2F', 'GET')
if response.status != 200 or urllib.parse.urlparse(response.geturl()).netloc != urllib.parse.urlparse(console_origin).netloc:
    raise SystemExit('Console OAuth callback did not complete')
response, body = request(f'{console_origin}/api/v1/session', 'GET')
session = json.loads(body)['data']
local_expiry = datetime.fromisoformat(session['expires_at'].replace('Z', '+00:00'))
local_remaining = (local_expiry - datetime.now(timezone.utc)).total_seconds()
if response.status != 200 or not 7.9 * 3600 <= local_remaining <= 8 * 3600 + 60:
    raise SystemExit('eight-hour Console Session verification failed')
response, _ = request(f'{console_origin}/api/v1/session/logout', 'POST', expected_status=204)
request(f'{console_origin}/api/v1/session', 'GET', expected_status=401)

response, body = request(f'{account_origin}/api/v1/sessions/revoke', 'POST', {'all_sessions': False}, f'cutover_revoke_{nonce}')
if response.status != 200 or json.loads(body).get('data', {}).get('revoked') is not True:
    raise SystemExit('Platform Core Session revocation failed')
response, _ = request(f'{console_origin}/api/v1/auth/login?return_to=%2F', 'GET')
final = urllib.parse.urlparse(response.geturl())
if final.netloc != urllib.parse.urlparse(account_origin).netloc or final.path != '/login':
    raise SystemExit('revoked Platform Core Session still authorized OAuth')
