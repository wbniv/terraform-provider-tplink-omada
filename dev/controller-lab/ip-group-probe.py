#!/usr/bin/env python3
"""Inspect profile-group wire formats on the disposable controller only."""
import json
from api import LabAPI
api=LabAPI();api.discover();api.login()
base='/api/v2/sites/'+api.credentials()['site_id']+'/setting/profiles/groups'
for suffix in ('','/0','/1','/2'):
 for page in (1,2):
  result=api.request(base+suffix+f'?currentPage={page}&currentPageSize=1')['result']
  print(json.dumps({'route':suffix or 'all','page':page,'result':result}))
for g in api.request(base+'/1')['result']['data']:
 if g['name'] in ('L4 Probe ports','L4 Probe all','L4 Probe ports renamed','L4 Probe all renamed'):
  api.request(base+'/1/'+g['groupId'],'DELETE')
created=[]
try:
 for ports in (['80','7000-7100'],[]):
  payload={'name':'L4 Probe '+('ports' if ports else 'all'),'type':1,'ipList':[{'ip':'192.0.2.0','mask':24},{'ip':'198.51.100.10','mask':32}],'portType':0,'portList':ports}
  result=api.request(base,'POST',payload,allow_error=True)
  print(json.dumps({'create':payload,'response':result}))
  if result.get('errorCode')==0:
   gid=result['result'];created.append(gid)
   changed=dict(payload,name=payload['name']+' renamed')
   print(json.dumps({'update':api.request(base+'/1/'+gid,'PATCH',changed,allow_error=True)}))
 for page in (1,2):
  print(json.dumps({'populated_page':page,'result':api.request(base+f'/1?currentPage={page}&currentPageSize=1')['result']}))
 if created:
  aclbase='/api/v2/sites/'+api.credentials()['site_id']+'/setting/firewall/acls'
  rule={'name':'L4 Reference','type':0,'status':False,'policy':1,'protocols':[6],'sourceType':2,'sourceIds':[created[0]],'destinationType':2,'destinationIds':[created[0]],'direction':{'lanToWan':True,'lanToLan':False}}
  result=api.request(aclbase,'POST',rule,allow_error=True)
  print(json.dumps({'acl_reference_create':result}))
  if result.get('errorCode')==0:
   rows=api.request(aclbase+'?type=0&currentPage=1&currentPageSize=100')['result']['data']
   for rule in rows:
    if rule['name']=='L4 Reference':
     print(json.dumps({'acl_reference_read':rule}))
     print(json.dumps({'acl_reference_delete':api.request(aclbase+'/'+rule['id'],'DELETE',allow_error=True)}))
finally:
 for gid in created:print(json.dumps({'delete':api.request(base+'/1/'+gid,'DELETE',allow_error=True)}))
