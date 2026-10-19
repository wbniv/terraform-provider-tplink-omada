#!/usr/bin/env python3
"""Restart only the disposable lab and verify its last certificate probe."""
import hashlib
import re
import socket
import ssl
import subprocess

from api import LabAPI, RUNTIME

api = LabAPI()  # Checks the compose-owned controller label and fixed URL.
api.discover()
api.login()
settings = api.request('/api/v2/controller/setting')['result']['certificate']
expected = re.findall(r'expected_certificate_sha256=([0-9a-f]{64})',
                      (RUNTIME / 'client-probes.log').read_text())
if len(expected) != 1 or not settings.get('enable'):
    raise RuntimeError('Run the successful certificate probe first')
subprocess.run(['docker', 'restart', 'omada-provider-lab-controller-1'], check=True)
api.discover(wait=True)
api.login()
after = api.request('/api/v2/controller/setting')['result']['certificate']
for field in ('cerId', 'keyId', 'enable'):
    if not settings.get(field) or settings[field] != after.get(field):
        raise RuntimeError('Certificate settings changed across restart')
context = ssl._create_unverified_context()  # Trust is checked by exact fingerprint.
with context.wrap_socket(socket.create_connection(('127.0.0.1', 18043), timeout=10),
                         server_hostname='controller') as connection:
    actual = hashlib.sha256(connection.getpeercert(binary_form=True)).hexdigest()
if actual != expected[0]:
    raise RuntimeError('Restarted controller does not present the uploaded certificate')
print('certificate_settings_persisted=true')
print('tls_certificate_sha256=' + actual)
print('tls_matches_uploaded_certificate=true')
