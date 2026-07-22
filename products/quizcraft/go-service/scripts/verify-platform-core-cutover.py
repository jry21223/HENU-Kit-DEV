#!/usr/bin/env python3
import email
import email.policy
import email.utils
import imaplib
import json
import os
import re
import secrets
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from email.header import decode_header, make_header
from http.cookiejar import CookieJar


def required(name):
    value = os.environ.get(name, '')
    if not value:
        raise SystemExit(f'{name} is required')
    return value


account_origin = required('PLATFORM_ACCOUNT_ORIGIN').rstrip('/')
console_origin = required('CONSOLE_ORIGIN').rstrip('/')
student_email = required('CUTOVER_TEST_EMAIL').lower()
imap_host = required('CUTOVER_IMAP_HOST')
imap_user = required('CUTOVER_IMAP_USERNAME')
imap_password = required('CUTOVER_IMAP_PASSWORD')
if not account_origin.startswith('https://') or not console_origin.startswith('https://'):
    raise SystemExit('Platform Core and Console verification require HTTPS')
started_at = time.time()
cookies = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))


def request(url, method='GET', payload=None, idempotency=None):
    body = None if payload is None else json.dumps(payload).encode()
    parsed = urllib.parse.urlparse(url)
    headers = {'Accept': 'application/json', 'Origin': f'{parsed.scheme}://{parsed.netloc}'}
    if body is not None:
        headers['Content-Type'] = 'application/json'
    if idempotency:
        headers['Idempotency-Key'] = idempotency
    response = opener.open(urllib.request.Request(url, data=body, headers=headers, method=method), timeout=15)
    return response, response.read()


nonce = secrets.token_urlsafe(18)
response, _ = request(
    f'{account_origin}/api/v1/auth/email-codes', 'POST',
    {'email': student_email, 'purpose': 'login', 'client_id': 'console-gateway'},
    f'cutover_mail_{nonce}',
)
if response.status != 202:
    raise SystemExit(f'verification request returned {response.status}')

code = None
deadline = time.time() + 180
while time.time() < deadline and code is None:
    with imaplib.IMAP4_SSL(imap_host, int(os.environ.get('CUTOVER_IMAP_PORT', '993')), ssl_context=ssl.create_default_context()) as mailbox:
        mailbox.login(imap_user, imap_password)
        mailbox.select('INBOX', readonly=True)
        status, identifiers = mailbox.search(None, 'ALL')
        if status != 'OK':
            raise SystemExit('IMAP search failed')
        for identifier in reversed(identifiers[0].split()[-30:]):
            status, payload = mailbox.fetch(identifier, '(RFC822)')
            if status != 'OK' or not payload or not isinstance(payload[0], tuple):
                continue
            message = email.message_from_bytes(payload[0][1], policy=email.policy.default)
            subject = str(make_header(decode_header(message.get('Subject', ''))))
            received = email.utils.parsedate_to_datetime(message.get('Date')) if message.get('Date') else None
            if subject != 'HENU Kit 登录验证码' or (received and received.timestamp() < started_at - 120):
                continue
            content = message.get_body(preferencelist=('plain',)).get_content() if message.is_multipart() else message.get_payload(decode=True).decode(message.get_content_charset() or 'utf-8', 'replace')
            match = re.search(r'验证码是[：:]\s*([0-9]{6,10})', content)
            if match:
                code = match.group(1)
                break
    if code is None:
        time.sleep(5)
if code is None:
    raise SystemExit('real verification email did not arrive within 180 seconds')

response, body = request(
    f'{account_origin}/api/v1/auth/email-codes/verify', 'POST',
    {'email': student_email, 'code': code, 'purpose': 'login'},
    f'cutover_verify_{nonce}',
)
data = json.loads(body)['data']
expiry = datetime.fromisoformat(data['session_expires_at'].replace('Z', '+00:00'))
remaining = (expiry - datetime.now(timezone.utc)).total_seconds()
if response.status != 200 or not data.get('user', {}).get('id') or not 14.9 * 86400 <= remaining <= 15 * 86400 + 60:
    raise SystemExit('15-day Core Session verification failed')

response, _ = request(f'{console_origin}/api/v1/auth/login?return_to=%2F', 'GET')
if response.status != 200 or urllib.parse.urlparse(response.geturl()).netloc != urllib.parse.urlparse(console_origin).netloc:
    raise SystemExit('Console OAuth callback did not complete')
response, body = request(f'{console_origin}/api/v1/session', 'GET')
session = json.loads(body)['data']
local_expiry = datetime.fromisoformat(session['expires_at'].replace('Z', '+00:00'))
local_remaining = (local_expiry - datetime.now(timezone.utc)).total_seconds()
if response.status != 200 or not 7.9 * 3600 <= local_remaining <= 8 * 3600 + 60:
    raise SystemExit('eight-hour Console Session verification failed')
response, _ = request(f'{console_origin}/api/v1/session/logout', 'POST')
if response.status != 204:
    raise SystemExit('Console logout failed')
try:
    request(f'{console_origin}/api/v1/session', 'GET')
except urllib.error.HTTPError as error:
    if error.code != 401:
        raise
else:
    raise SystemExit('logged-out Console Session remained usable')

response, body = request(f'{account_origin}/api/v1/sessions/revoke', 'POST', {'all_sessions': False}, f'cutover_revoke_{nonce}')
if response.status != 200 or json.loads(body).get('data', {}).get('revoked') is not True:
    raise SystemExit('Platform Core Session revocation failed')
response, _ = request(f'{console_origin}/api/v1/auth/login?return_to=%2F', 'GET')
final = urllib.parse.urlparse(response.geturl())
if final.netloc != urllib.parse.urlparse(account_origin).netloc or final.path != '/login':
    raise SystemExit('revoked Platform Core Session still authorized OAuth')
