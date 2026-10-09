#!/usr/bin/env python3
"""Portable Shiji REST client. Python 3.10+, standard library only."""
import argparse
import json
import mimetypes
import os
from pathlib import Path
import re
import sys
import time
from http.client import HTTPException
from hashlib import sha256
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import Request, HTTPRedirectHandler, build_opener
from uuid import UUID, uuid4

MAX_RESPONSE = 12 * 1024**2
USER_AGENT = 'ShijiNotes/1.0 (+https://notes.shier.art/agent/SKILL.md)'

class CliError(Exception):
    def __init__(self, code, message, status=None, details=None):
        super().__init__(message)
        self.code, self.status, self.details = code, status, details

class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None

class Client:
    def __init__(self, env=None, sleep=time.sleep):
        env = os.environ if env is None else env
        if not env.get('NOTES_API_URL') and not env.get('NOTES_API_TOKEN'):
            from agent_auth import load_profile
            try:profile=load_profile()
            except (ValueError,OSError):raise CliError('configuration','Cannot read the private credential profile; check current-user permissions or run login.')
            if profile:env={**env,'NOTES_API_URL':profile['api_url'],'NOTES_API_TOKEN':profile['token']}
        self.base = env.get('NOTES_API_URL', '').rstrip('/')
        self.token = env.get('NOTES_API_TOKEN', '').strip()
        try:
            url = urlsplit(self.base)
            _ = url.port
        except ValueError:
            raise CliError('configuration', 'NOTES_API_URL is not a valid API URL.')
        local = url.scheme == 'http' and url.hostname in {'localhost','127.0.0.1','::1'} and env.get('NOTES_ALLOW_LOCAL_HTTP') == '1'
        if (url.scheme != 'https' and not local) or not url.hostname or url.username is not None or url.password is not None or url.query or url.fragment or not url.path.endswith('/api/v1') or any(c.isspace() for c in self.base):
            raise CliError('configuration', 'Use an HTTPS API URL ending in /api/v1. Local HTTP requires NOTES_ALLOW_LOCAL_HTTP=1.')
        if not self.token or any(ord(c) < 33 or ord(c) > 126 for c in self.token):
            raise CliError('configuration', 'Run shiji-notes login, or provide both NOTES_API_URL and NOTES_API_TOKEN.')
        self.opener = build_opener(NoRedirect())
        self.sleep = sleep

    def request(self, method, path, body=None, key=None, raw=None, content_type=None, binary=False,output=None,expected_size=None,timeout=15):
        if not path.startswith('/') or path.startswith('//'):
            raise CliError('invalid_path','Invalid API path.')
        if output is not None and (method!='GET' or not isinstance(expected_size,int) or expected_size<0):
            raise CliError('arguments','File download needs GET and an expected size.')
        if output is not None:
            output=Path(output)
            if output.exists():raise CliError('file_exists','Output already exists; choose a new filename.')
        headers = {'Accept':'application/json','User-Agent':USER_AGENT}
        if path not in {'/auth/agent/request','/auth/agent/poll','/auth/agent/cancel'}:
            headers['Authorization']='Bearer '+self.token
        if key:
            if not re.fullmatch(r'[A-Za-z0-9_.:-]{8,128}',key):
                raise CliError('invalid_key','Idempotency key needs 8–128 ASCII letters, digits or _.:-.')
            headers['Idempotency-Key'] = key
        data = raw
        if body is not None:
            data = json.dumps(body,ensure_ascii=False).encode('utf8')
            headers['Content-Type'] = 'application/json'
        elif content_type: headers['Content-Type'] = content_type
        retryable = method == 'GET' or (method == 'POST' and path == '/notes' and key is not None)
        for attempt in range(3 if retryable else 1):
            try:
                with self.opener.open(Request(self.base+path,data=data,headers=headers,method=method),timeout=timeout) as response:
                    if output is not None:
                        try:target=output.open('xb')
                        except FileExistsError:raise CliError('file_exists','Output already exists; choose a new filename.')
                        except OSError:raise CliError('local_file','Cannot create the requested output file.')
                        total=0;hashed=sha256()
                        try:
                            with target:
                                while chunk:=response.read(128*1024):
                                    total+=len(chunk)
                                    if total>expected_size:raise CliError('response_size','Download exceeds its declared size.')
                                    try:target.write(chunk)
                                    except OSError:raise CliError('local_file','Cannot write the requested output file.')
                                    hashed.update(chunk)
                            if total!=expected_size:raise CliError('incomplete_download','Download incomplete; file removed. Generate or download the archive again.')
                        except BaseException:
                            output.unlink(missing_ok=True)
                            raise
                        return {'output':str(output),'bytes':total,'sha256':hashed.hexdigest()}
                    result = response.read(MAX_RESPONSE+1)
                    if len(result)>MAX_RESPONSE:raise CliError('response_size','API response exceeded 12 MiB.')
                    if binary:return result
                    if not result:return {'ok':True}
                    try:return json.loads(result)
                    except (ValueError,UnicodeError):raise CliError('invalid_response','API returned invalid JSON. Read the resource before repeating a write.')
            except HTTPError as error:
                if 300<=error.code<400:
                    error.close();raise CliError('redirect_refused','API redirects are refused. Configure the final HTTPS API URL.',error.code)
                payload = error.read(MAX_RESPONSE); error.close()
                try: detail = json.loads(payload).get('error',{})
                except (ValueError,UnicodeError,AttributeError):detail = {}
                if not isinstance(detail,dict):detail = {}
                if error.code==403 and not detail and (error.headers.get('CF-Ray') or b'cloudflare' in payload.lower()):
                    reason = ' (Error 1010: User-Agent/browser integrity policy)' if re.search(rb'\b1010\b',payload) else ''
                    raise CliError('cloudflare_blocked','Cloudflare blocked this request'+reason+'. The request uses the ShijiNotes User-Agent. Check Cloudflare rules for this API; changing Notes permissions or repeating login will not resolve an edge block.',403)
                if retryable and error.code in {429,502,503,504} and attempt<2:
                    try:delay = min(5,max(0,float(error.headers.get('Retry-After',0.25*(2**attempt)))))
                    except ValueError:delay = 0.25*(2**attempt)
                    self.sleep(delay);continue
                raise CliError(detail.get('code','http_error'),detail.get('message',f'API request failed ({error.code}).'),error.code,detail.get('details'))
            except (URLError,TimeoutError,OSError,HTTPException) as error:
                if retryable and attempt<2:
                    self.sleep(0.25*(2**attempt));continue
                details = {'idempotency_key':key} if key else None
                message = 'Connection failed. Retry keyed creation with the same payload and key.' if key else 'Connection failed. Read the resource before repeating any write.'
                raise CliError('connection_failed',message,details=details) from error

def identifier(value):
    try:return str(UUID(value))
    except ValueError:raise argparse.ArgumentTypeError('Expected a UUID.')

def read_file(value, as_json=False):
    path = Path(value)
    if path.stat().st_size>1024**2:raise CliError('input_size','Input file exceeds 1 MiB.')
    content = path.read_text(encoding='utf-8-sig')
    if as_json:
        try:return json.loads(content)
        except ValueError:raise CliError('input_json','File contains invalid JSON.')
    return content

def body_args(parser, required=False):
    group = parser.add_mutually_exclusive_group(required=required)
    group.add_argument('--markdown-file');group.add_argument('--json-file')

def body_fields(args):
    if getattr(args,'markdown_file',None):return {'content_format':'markdown','content':read_file(args.markdown_file)}
    if getattr(args,'json_file',None):
        content=read_file(args.json_file,True)
        if not isinstance(content,dict):raise CliError('input_json','JSON body file must contain a document object.')
        return {'content_format':'json','content':content}
    return {}

class JsonArgumentParser(argparse.ArgumentParser):
    def error(self,message):raise CliError('arguments',message)

def arguments():
    parser=JsonArgumentParser(description=__doc__)
    commands=parser.add_subparsers(dest='command',required=True)
    p=commands.add_parser('login',help='Open Notes for user approval and save a private local credential')
    p.add_argument('--server');p.add_argument('--name',default='My Agent');p.add_argument('--no-browser',action='store_true');p.add_argument('--replace',action='store_true')
    commands.add_parser('whoami',help='Check saved connection, account and scopes')
    commands.add_parser('logout',help='Revoke and remove the saved credential')
    for name in ['list','search']:
        p=commands.add_parser(name)
        if name=='search':p.add_argument('query')
        p.add_argument('--folder',type=identifier);p.add_argument('--tag',type=identifier)
        p.add_argument('--trash',action='store_true');p.add_argument('--favorite',action='store_true')
        p.add_argument('--cursor');p.add_argument('--limit',type=int,default=20,choices=range(1,101),metavar='1..100')
    p=commands.add_parser('get');p.add_argument('id',type=identifier);p.add_argument('--format',choices=['json','markdown'],default='json')
    p=commands.add_parser('create');p.add_argument('--folder',type=identifier,required=True);p.add_argument('--title',default='');p.add_argument('--tag',type=identifier,action='append');p.add_argument('--key');body_args(p)
    p=commands.add_parser('update');p.add_argument('id',type=identifier);p.add_argument('--expected-version',type=int,required=True);p.add_argument('--title');p.add_argument('--tag',type=identifier,action='append');p.add_argument('--clear-tags',action='store_true');p.add_argument('--favorite',choices=['true','false']);body_args(p)
    p=commands.add_parser('move');p.add_argument('id',type=identifier);p.add_argument('--folder',type=identifier,required=True);p.add_argument('--expected-version',type=int,required=True)
    p=commands.add_parser('blocks');p.add_argument('id',type=identifier);p.add_argument('--expected-version',type=int,required=True);p.add_argument('--operations-file',required=True)
    p=commands.add_parser('validate');body_args(p,True)
    for name in ['trash','restore']:
        p=commands.add_parser(name);p.add_argument('id',type=identifier);p.add_argument('--expected-version',type=int,required=True)
    p=commands.add_parser('folders');actions=p.add_subparsers(dest='action',required=True);actions.add_parser('list')
    q=actions.add_parser('create');q.add_argument('name');q.add_argument('--parent',type=identifier)
    p=commands.add_parser('tags');actions=p.add_subparsers(dest='action',required=True);actions.add_parser('list')
    q=actions.add_parser('create');q.add_argument('name')
    q=actions.add_parser('rename');q.add_argument('id',type=identifier);q.add_argument('name')
    q=actions.add_parser('delete');q.add_argument('id',type=identifier)
    p=commands.add_parser('upload-image');p.add_argument('file')
    p=commands.add_parser('download-image');p.add_argument('id',type=identifier);p.add_argument('--output',required=True)
    p=commands.add_parser('history');p.add_argument('id',type=identifier);p.add_argument('--limit',type=int,default=20,choices=range(1,101),metavar='1..100');p.add_argument('--before-version',type=int)
    for name in ['history-get','history-restore']:
        p=commands.add_parser(name);p.add_argument('id',type=identifier);p.add_argument('version',type=int)
        if name=='history-restore':p.add_argument('--expected-version',type=int,required=True)
    commands.add_parser('exports')
    p=commands.add_parser('export');p.add_argument('--output',required=True);p.add_argument('--bundle',type=identifier);p.add_argument('--exclude-trash',action='store_true');p.add_argument('--exclude-history',action='store_true')
    return parser

def execute(client,args):
    command=args.command
    if command=='whoami':return client.request('GET','/auth/agent/me')
    if command in {'list','search'}:
        params={'limit':args.limit,'trash':str(args.trash).lower()}
        for field,param in [('folder','folder_id'),('tag','tag_id'),('cursor','cursor'),('query','query')]:
            if getattr(args,field,None):params[param]=getattr(args,field)
        if args.favorite:params['favorite']='true'
        return client.request('GET','/notes?'+urlencode(params))
    if command=='get':return client.request('GET','/notes/'+args.id+'?'+urlencode({'content_format':args.format}))
    if command=='create':
        key=args.key or str(uuid4())
        payload={'folder_id':args.folder,'title':args.title,'tag_ids':args.tag or [],**body_fields(args)}
        try:result=client.request('POST','/notes',payload,key=key)
        except CliError as error:
            error.details={**(error.details if isinstance(error.details,dict) else {}),'idempotency_key':key}
            raise
        return {**result,'_idempotency_key':key}
    if command in {'update','move'}:
        payload={'expected_version':args.expected_version}
        if command=='move':payload['folder_id']=args.folder
        else:
            payload.update(body_fields(args))
            if args.title is not None:payload['title']=args.title
            if args.clear_tags and args.tag:raise CliError('arguments','Use --tag or --clear-tags, not both.')
            if args.clear_tags or args.tag:payload['tag_ids']=args.tag or []
            if args.favorite is not None:payload['favorite']=args.favorite=='true'
            if len(payload)==1:raise CliError('arguments','Update requires a field to change.')
        return client.request('PATCH','/notes/'+args.id,payload)
    if command=='blocks':
        ops=read_file(args.operations_file,True)
        if not isinstance(ops,list):raise CliError('input_json','Operations file must contain a JSON array.')
        return client.request('POST','/notes/'+args.id+'/blocks',{'expected_version':args.expected_version,'operations':ops})
    if command=='validate':return client.request('POST','/notes/validate-content',body_fields(args))
    if command=='trash':return client.request('DELETE','/notes/'+args.id+'?'+urlencode({'expected_version':args.expected_version}))
    if command=='restore':return client.request('POST','/notes/'+args.id+'/restore',{'expected_version':args.expected_version})
    if command=='folders':
        return client.request('GET','/folders') if args.action=='list' else client.request('POST','/folders',{'name':args.name,'parent_id':args.parent})
    if command=='tags':
        if args.action=='list':return client.request('GET','/tags')
        if args.action=='create':return client.request('POST','/tags',{'name':args.name})
        if args.action=='rename':return client.request('PATCH','/tags/'+args.id,{'name':args.name})
        return client.request('DELETE','/tags/'+args.id)
    if command=='upload-image':
        path=Path(args.file)
        if path.stat().st_size>10*1024**2:raise CliError('input_size','Image exceeds 10 MiB.')
        mime=mimetypes.guess_type(path.name)[0]
        extensions={'image/png':'png','image/jpeg':'jpg','image/webp':'webp','image/gif':'gif'}
        if mime not in extensions:raise CliError('image_type','Use a JPEG, PNG, WebP or GIF file.')
        boundary='shiji'+uuid4().hex
        prefix=f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="image.{extensions[mime]}"\r\nContent-Type: {mime}\r\n\r\n'.encode('ascii')
        raw=prefix+path.read_bytes()+f'\r\n--{boundary}--\r\n'.encode('ascii')
        return client.request('POST','/attachments',raw=raw,content_type='multipart/form-data; boundary='+boundary)
    if command=='download-image':
        path=Path(args.output)
        if path.exists():raise CliError('file_exists','Output already exists; choose a new filename.')
        data=client.request('GET','/attachments/'+args.id,binary=True)
        with path.open('xb') as file:file.write(data)
        return {'id':args.id,'output':str(path),'bytes':len(data)}
    if command=='history':
        params={'limit':args.limit}
        if args.before_version is not None:params['before_version']=args.before_version
        return client.request('GET','/notes/'+args.id+'/history?'+urlencode(params))
    if command in {'history-get','history-restore'}:
        route='/notes/'+args.id+'/history/'+str(args.version)
        return client.request('GET',route) if command=='history-get' else client.request('POST',route+'/restore',{'expected_version':args.expected_version})
    if command=='export':
        output=Path(args.output)
        if output.exists():raise CliError('file_exists','Output already exists; choose a new filename.')
        if args.bundle:
            if args.exclude_trash or args.exclude_history:raise CliError('arguments','Existing bundles cannot change export options.')
            bundles=client.request('GET','/exports')['items']
            bundle=next((item for item in bundles if item['id']==args.bundle),None)
            if bundle is None:raise CliError('not_found','Bundle missing or expired; prepare a new export.',404)
            if not bundle.get('ready'):raise CliError('export_pending','Bundle is not ready yet.',409)
        else:
            bundle=client.request('POST','/exports',{'include_trash':not args.exclude_trash,'include_history':not args.exclude_history},timeout=120)
        # Construct the same-origin path from an ID instead of following an
        # arbitrary URL returned by the service.
        try:bid=str(UUID(bundle['id']));size=bundle['size_bytes']
        except (KeyError,ValueError,TypeError):raise CliError('invalid_response','API returned an invalid export resource.')
        download=client.request('GET','/exports/'+bid+'/download',output=output,expected_size=size,timeout=60)
        return {**download,'id':bid,'counts':bundle.get('counts',{})}
    if command=='exports':return client.request('GET','/exports')
    raise CliError('arguments','Unknown command.')

def main(argv=None):
    client=None
    try:
        args=arguments().parse_args(argv)
        if args.command in {'login','logout'}:
            from agent_auth import login,logout
            result=login(args,Client,CliError) if args.command=='login' else logout(Client,CliError)
            print(json.dumps(result,ensure_ascii=False));return 0
        client=Client()
        result=execute(client,args)
        print(json.dumps(result,ensure_ascii=False))
        return 0
    except CliError as error:
        value={'error':{'code':error.code,'message':str(error),'status':error.status,'details':error.details}}
    except (OSError,UnicodeError) as error:
        value={'error':{'code':'local_file','message':'Cannot read or write the requested UTF-8 file.','details':None}}
    except ValueError as error:
        value={'error':{'code':'configuration','message':str(error),'details':None}}
    except KeyboardInterrupt:
        value={'error':{'code':'canceled','message':'Connection canceled. Any newly approved credential was revoked when reachable; check Notes credential management if the service was unavailable.'}}
    encoded=json.dumps(value,ensure_ascii=False)
    if client:encoded=encoded.replace(client.token,'[REDACTED]')
    print(encoded,file=sys.stderr)
    return 1

if __name__=='__main__':
    for stream in (sys.stdout,sys.stderr):
        if hasattr(stream,'reconfigure'):stream.reconfigure(encoding='utf8')
    raise SystemExit(main())
