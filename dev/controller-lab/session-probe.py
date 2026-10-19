#!/usr/bin/env python3
"""Record sanitized authentication rejection shapes on the disposable lab."""
import http.cookiejar
import json
import ssl
import urllib.error
import urllib.parse
import urllib.request

from api import BASE, LabAPI

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

api = LabAPI()
api.discover()
api.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(),
    urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
    urllib.request.HTTPSHandler(context=ssl._create_unverified_context()))
api.login()
original_token = api.token
original_opener = api.opener

def probe(label):
    url = BASE + '/' + api.controller_id + '/api/v2/sites?token=' + urllib.parse.quote(api.token)
    request = urllib.request.Request(url, headers={'Csrf-Token': api.token})
    try:
        response = api.opener.open(request, timeout=30)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        location = urllib.parse.urlsplit(urllib.parse.urljoin(url, response.headers.get('Location', '')))
        source = urllib.parse.urlsplit(BASE)
        try:
            payload = json.loads(body)
        except ValueError:
            payload = {}
        print(json.dumps({'case': label, 'status': response.status, 'body_bytes':len(body), 'content_type':response.headers.get('Content-Type'), 'api_code': payload.get('errorCode'),
                          'location_count': len(response.headers.get_all('Location', [])),
                          'location_same_origin': (location.scheme, location.netloc) == (source.scheme, source.netloc) if response.headers.get('Location') else None,
                          'location_path': location.path.replace(api.controller_id, '<controller-id>') if response.headers.get('Location') else None,
                          'location_has_query': bool(response.headers.get('Location')) and bool(location.query), 'location_has_fragment': bool(response.headers.get('Location')) and bool(location.fragment)}))

probe('valid')
api.token = 'deliberately-invalid-lab-token'
probe('invalid-token-valid-cookie')
api.token = original_token
api.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(),
    urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
    urllib.request.HTTPSHandler(context=ssl._create_unverified_context()))
probe('valid-token-no-cookie')
api.token = 'deliberately-invalid-lab-token'
probe('invalid-token-no-cookie')
api.opener = original_opener
api.token = original_token
api.login()
api.request('/api/v2/logout', 'POST', {})
probe('logged-out')
