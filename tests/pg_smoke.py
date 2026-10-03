"""Disposable Docker/PG16 integration. CI-only, cleans its own random projects."""
import base64,hashlib,http.cookiejar,io,json,os,secrets,struct,subprocess,tempfile,urllib.error,urllib.parse,urllib.request,zipfile,zlib
from pathlib import Path
if os.environ.get('CI')!='true':raise SystemExit('Run only in disposable CI; this test removes its own Docker test volumes.')
ROOT=Path(__file__).resolve().parents[1]
password=secrets.token_hex(32)
env={**os.environ,'DOMAIN':'ci.example.test','POSTGRES_PASSWORD':password}
projects=['shiji-ci-'+secrets.token_hex(5), 'shiji-restore-'+secrets.token_hex(5)]
def compose(index,*args):
    e={**env,'TEST_PORT':str(18000+index)}
    files=['-f',str(ROOT/'notes-server-go'/('compose.yaml' if index==0 else 'compose.external.yaml')),'-f',str(ROOT/'tests/compose.ci.yaml')]
    if index==1:
        e.update(DATABASE_URL=f'postgresql://notes:{password}@db:5432/notes_restore',TEST_DB_NETWORK=projects[0]+'_default')
        files+=['-f',str(ROOT/'tests/compose.external.ci.yaml')]
    r=subprocess.run(['docker','compose','--project-name',projects[index],*files,*args],env=e,capture_output=True,text=True)
    if r.returncode:raise RuntimeError(f'Compose {args[0]} failed: {r.stderr}')
    return r.stdout.strip()
class API:
    def __init__(self,index):
        self.origin=f'http://127.0.0.1:{18000+index}';self.jar=http.cookiejar.CookieJar();self.opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar));self.csrf=''
    def call(self,method,path,data=None,expect=200,headers=None):
        h={'Origin':self.origin,'X-CSRF-Token':self.csrf,**(headers or {})}
        if data is not None and not isinstance(data,bytes):data=json.dumps(data).encode();h['Content-Type']='application/json'
        req=urllib.request.Request(self.origin+'/api/v1'+path,data=data,method=method,headers=h)
        try:r=self.opener.open(req,timeout=60)
        except urllib.error.HTTPError as e:r=e
        raw=r.read();assert r.status==expect,(method,path,r.status,raw[:300])
        return json.loads(raw) if 'application/json' in r.headers.get('Content-Type','') else raw
def chunk(kind,data):return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
image=b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',1,1,8,2,0,0,0))+chunk(b'IDAT',zlib.compress(b'\x00\x77\xaa\xbb'))+chunk(b'IEND',b'')
backup=Path(tempfile.mkdtemp(prefix='shiji-ci-backup-'));backup.chmod(0o777)
try:
    compose(0,'up','-d','--no-build','--wait','--wait-timeout','180','db','app')
    assert compose(0,'exec','-T','app','shiji','bootstrap-status')=='empty'
    api=API(0);email='ci-'+secrets.token_hex(4)+'@example.test';pwd=secrets.token_urlsafe(24)
    invite=urllib.parse.parse_qs(urllib.parse.urlparse(compose(0,'exec','-T','app','shiji','bootstrap','--email',email)).query)['invite'][0]
    assert compose(0,'exec','-T','app','shiji','bootstrap-status')=='pending'
    account=api.call('POST','/auth/register',{'email':email,'password':pwd,'display_name':'CI','invitation':invite},201);api.csrf=account['csrf_token']
    assert compose(0,'exec','-T','app','shiji','bootstrap-status')=='registered'
    cookie='; '.join(c.name+'='+c.value for c in api.jar)
    folder=api.call('GET','/folders')['items'][0]['id']
    boundary='ci-'+secrets.token_hex(8)
    body=('--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="image.png"\r\nContent-Type: image/png\r\n\r\n').encode()+image+('\r\n--'+boundary+'--\r\n').encode()
    pic=api.call('POST','/attachments',body,201,{'Content-Type':'multipart/form-data; boundary='+boundary})
    tag=api.call('POST','/tags',{'name':'验证'},201)
    doc={'type':'doc','content':[{'type':'paragraph','content':[{'type':'text','text':'PostgreSQL图文'}]},{'type':'image','attrs':{'attachment_id':pic['id'],'alt':'CI'}}]}
    payload={'folder_id':folder,'title':'PG16','content_json':doc,'tag_ids':[tag['id']]}
    note=api.call('POST','/notes',payload,201,{'Idempotency-Key':'ci-create-123'});assert api.call('POST','/notes',payload,201,{'Idempotency-Key':'ci-create-123'})==note
    api.call('PATCH','/notes/'+note['id'],{'expected_version':1,'title':'更新'})
    api.call('PATCH','/notes/'+note['id'],{'expected_version':1,'title':'冲突'},409)
    api.call('PATCH','/notes/'+note['id'],{'expected_version':2,'content_json':{'type':'doc','content':[{'type':'paragraph'}]}})
    restored=api.call('POST','/notes/'+note['id']+'/history/1/restore',{'expected_version':3});assert restored['version']==4
    token=api.call('POST','/tokens',{'name':'ci-agent','scopes':['notes:read','notes:update','folders:read','tags:read','attachments:read']},201)['secret']
    agent={'Authorization':'Bearer '+token};assert api.call('GET','/notes/'+note['id'],headers=agent)['version']==4
    assert api.call('GET','/attachments/'+pic['id'],headers=agent)==image
    bundle=api.call('POST','/exports',{'include_history':True,'include_trash':True},201)
    archive=zipfile.ZipFile(io.BytesIO(api.call('GET','/exports/'+bundle['id']+'/download')))
    manifest=json.loads(archive.read('manifest.json'));assert manifest['counts']['attachments']==1
    for f in manifest['files']:raw=archive.read(f['path']);assert len(raw)==f['size_bytes'] and hashlib.sha256(raw).hexdigest()==f['sha256']
    compose(0,'stop','app')
    mount=f'{backup}:/backup'
    compose(0,'run','--rm','--no-deps','--volume',mount,'--entrypoint','shiji','app','backup-create','--output','/backup/snapshot','--app-stopped')
    compose(0,'run','--rm','--no-deps','--volume',mount,'--entrypoint','shiji','app','backup-verify','--backup','/backup/snapshot')
    assert set(compose(1,'config','--services').splitlines())=={'app','caddy'}
    compose(0,'exec','-T','db','psql','-U','notes','-d','postgres','-c','CREATE DATABASE notes_restore OWNER notes')
    compose(1,'run','--rm','--no-deps','--entrypoint','shiji','app','database-check')
    compose(0,'exec','-T','db','psql','-U','notes','-d','notes_restore','-c','CREATE TABLE unrelated_business (id integer)')
    try:
        compose(1,'run','--rm','--no-deps','--entrypoint','shiji','app','database-check')
    except RuntimeError as error:
        assert 'refusing deployment' in str(error)
    else:raise AssertionError('preflight accepted unrelated database')
    compose(0,'exec','-T','db','psql','-U','notes','-d','notes_restore','-c','DROP TABLE unrelated_business')
    restore_env={**env,'TEST_PORT':'18001','COMPOSE_PROJECT_NAME':projects[1],
        'DATABASE_URL':f'postgresql://notes:{password}@db:5432/notes_restore','TEST_DB_NETWORK':projects[0]+'_default',
        'COMPOSE_FILE':os.pathsep.join(str(ROOT/p) for p in ['notes-server-go/compose.external.yaml','tests/compose.ci.yaml','tests/compose.external.ci.yaml'])}
    restored_process=subprocess.run(['sudo','-E','bash',str(ROOT/'notes-server-go/ops/restore.sh'),str(backup/'snapshot')],env=restore_env,capture_output=True,text=True)
    if restored_process.returncode:raise RuntimeError('External restore.sh failed: '+restored_process.stderr)
    compose(1,'up','-d','--no-build','--wait','--wait-timeout','180','app')
    assert compose(1,'exec','-T','app','shiji','bootstrap-status')=='registered'
    compose(1,'exec','-T','app','shiji','database-check')
    recovered=API(1);assert recovered.call('GET','/notes/'+note['id'],headers=agent)['version']==4
    assert recovered.call('GET','/attachments/'+pic['id'],headers=agent)==image
    recovered.call('GET','/me',expect=401,headers={'Cookie':cookie})
    assert recovered.call('POST','/auth/login',{'email':email,'password':pwd})['email']==email
    print('PASS: Docker + PG16 auth/image/tag/CAS/idempotency/history/export, external app-only deployment, empty/unrelated/notes DB preflight and bootstrap state; dump/restore preserves token/password and invalidates session')
finally:
    for i in (1,0):
        try:compose(i,'down','--volumes','--remove-orphans')
        except Exception as e:print('CI cleanup:',e)
