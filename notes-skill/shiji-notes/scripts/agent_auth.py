"""Browser authorization and current-user credential storage; standard library only."""
import csv
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import stat
import subprocess
import sys
import tempfile
import time
from urllib.parse import urlsplit
import webbrowser

DEFAULT_SERVER='https://notes.shier.art'

def origin(value,env=None):
    env=os.environ if env is None else env
    value=value.rstrip('/')
    try:p=urlsplit(value);_=p.port
    except ValueError:raise ValueError('Invalid Notes server address.')
    local=p.scheme=='http' and p.hostname in {'localhost','127.0.0.1','::1'} and env.get('NOTES_ALLOW_LOCAL_HTTP')=='1'
    if (p.scheme!='https' and not local) or not p.hostname or p.path or p.query or p.fragment or p.username is not None or any(c.isspace() for c in value):
        raise ValueError('Use the final HTTPS Notes app origin. Local HTTP requires NOTES_ALLOW_LOCAL_HTTP=1.')
    return value

def paths():
    directory=Path.home()/'.shiji-notes';file=directory/'client.json'
    for p in (file,*file.parents):
        if p.is_symlink() or (p.exists() and getattr(p.lstat(),'st_file_attributes',0)&getattr(stat,'FILE_ATTRIBUTE_REPARSE_POINT',0)):
            raise ValueError('Credential paths must not use symlinks or junctions.')
    return directory,file

def windows_sid():
    try:output=subprocess.run(['whoami','/user','/fo','csv','/nh'],check=True,capture_output=True,text=True).stdout
    except (OSError,subprocess.SubprocessError):raise ValueError('Cannot identify the current Windows user.')
    sid=next(csv.reader([output.strip()]))[-1]
    if not re.fullmatch(r'S-1-\d+(?:-\d+)+',sid):raise ValueError('Cannot identify the current Windows user.')
    return sid

def private(path,directory=False):
    st=path.lstat()
    if not (stat.S_ISDIR(st.st_mode) if directory else stat.S_ISREG(st.st_mode)):
        raise ValueError('Credentials must be a regular file in a private directory.')
    if os.name!='nt':
        if st.st_uid!=os.getuid() or stat.S_IMODE(st.st_mode)!=(0o700 if directory else 0o600):
            raise ValueError('Credentials must belong to the current user: directory 700, file 600.')
    else:
        # Paths are data in an environment variable, never PowerShell source text.
        script="$a=Get-Acl -LiteralPath $env:SHIJI_ACL_PATH; @{owner=$a.GetOwner([System.Security.Principal.SecurityIdentifier]).Value; entries=@($a.Access | ForEach-Object { @{sid=$_.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value; type=$_.AccessControlType.ToString()} })} | ConvertTo-Json -Depth 4 -Compress"
        environment={k:v for k,v in os.environ.items() if k.lower()!='psmodulepath'}
        try:run=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',script],env={**environment,'SHIJI_ACL_PATH':str(path)},check=True,capture_output=True,text=True)
        except (OSError,subprocess.SubprocessError):raise ValueError('Cannot verify the private Windows credential ACL.')
        acl=json.loads(run.stdout);sid=windows_sid()
        if acl['owner']!=sid or any(e['type']=='Allow' and e['sid'] not in {sid,'S-1-3-4','S-1-5-18','S-1-5-32-544'} for e in acl['entries']):
            raise ValueError('Credential ACL must restrict access to the current user and system administrators.')

def credential_directory():
    directory,file=paths()
    if not directory.exists():
        directory.mkdir(mode=0o700)
        if os.name=='nt':
            try:
                sid=windows_sid()
                subprocess.run(['icacls',str(directory),'/setowner','*'+sid],check=True,capture_output=True)
                subprocess.run(['icacls',str(directory),'/inheritance:r','/grant:r','*'+sid+':(OI)(CI)F'],check=True,capture_output=True)
            except (OSError,subprocess.SubprocessError):raise ValueError('Cannot create a private Windows credential directory.')
    private(directory,True)
    return directory,file

def load_profile():
    directory,file=paths()
    if not directory.exists():return None
    private(directory,True)
    if not file.exists():return None
    private(file)
    if file.stat().st_size>16384:raise ValueError('Credential profile exceeds its size limit.')
    value=json.loads(file.read_text(encoding='utf-8'))
    if not isinstance(value,dict) or not isinstance(value.get('api_url'),str) or not isinstance(value.get('token'),str) or not re.fullmatch(r'sj_[A-Za-z0-9_-]{43}',value['token']):
        raise ValueError('Invalid credential profile; run login again.')
    if not value['api_url'].endswith('/api/v1'):raise ValueError('Invalid credential API URL.')
    origin(value['api_url'][:-7])
    return value

def save_profile(value):
    directory,file=credential_directory()
    if file.exists():private(file)
    with tempfile.NamedTemporaryFile(mode='w',encoding='utf-8',prefix='.client-',dir=directory,delete=False) as f:
        temp=Path(f.name)
        try:
            if os.name!='nt':os.fchmod(f.fileno(),0o600)
            json.dump(value,f,ensure_ascii=False);f.flush();os.fsync(f.fileno())
        except BaseException:
            temp.unlink(missing_ok=True);raise
    try:
        if os.name=='nt':
            try:subprocess.run(['icacls',str(temp),'/setowner','*'+windows_sid()],check=True,capture_output=True)
            except (OSError,subprocess.SubprocessError):raise ValueError('Cannot assign the credential file to the current Windows user.')
        private(temp)
        os.replace(temp,file)
    finally:temp.unlink(missing_ok=True)

def remove_profile():
    profile=load_profile()
    if profile is not None:paths()[1].unlink()

def login(args,Client,CliError,sleep=time.sleep):
    # Validate private storage before asking the user to authorize.
    credential_directory();previous=load_profile()
    server=origin(args.server or (previous['api_url'][:-7] if previous else DEFAULT_SERVER))
    if previous is not None and not args.replace:
        raise CliError('already_logged_in','A credential is already saved. Use whoami, or login --replace to authorize a replacement. The previous remote credential remains until revoked in Notes.')
    raw='sj_'+secrets.token_urlsafe(32)
    client=Client({**os.environ,'NOTES_API_URL':server+'/api/v1','NOTES_API_TOKEN':raw})
    device=None;saved=False
    try:
        grant=client.request('POST','/auth/agent/request',{'name':args.name,'token_hash':hashlib.sha256(raw.encode()).hexdigest(),'token_prefix':raw[:10]})
        device=grant.get('device_code')
        if not isinstance(device,str) or len(device)!=43:raise CliError('invalid_response','Invalid device authorization response.')
        code=grant.get('user_code')
        if not isinstance(code,str) or not re.fullmatch(r'[A-Z2-7]{4}-[A-Z2-7]{4}',code):raise CliError('invalid_response','Invalid confirmation code.')
        # Construct from the validated origin and code; do not follow arbitrary service URLs.
        url=server+'/#agent-authorize?code='+code
        print(json.dumps({'event':'authorization_required','verification_uri':url,'user_code':code,'message':'Open Notes in a new app/browser tab. The user must check the code and approve; the Agent must not approve for them.'},ensure_ascii=False),file=sys.stderr,flush=True)
        if not args.no_browser:webbrowser.open(url,new=2)
        deadline=time.monotonic()+min(600,max(1,int(grant.get('expires_in',600))))
        while time.monotonic()<deadline:
            response=client.request('POST','/auth/agent/poll',{'device_code':device})
            status=response.get('status')
            if status=='approved':
                identity=client.request('GET','/auth/agent/me')
                token=identity.get('token');account=identity.get('account')
                if not isinstance(token,dict) or not isinstance(account,dict) or token.get('prefix')!=raw[:10] or token.get('revoked'):
                    raise CliError('invalid_response','Invalid authorized identity.')
                metadata={'account':{k:account.get(k) for k in ('id','email','display_name')},'token':{k:token.get(k) for k in ('id','name','prefix','scopes','expires_at','revoked')}}
                save_profile({'api_url':client.base,'token':raw,'account':metadata['account'],'token_info':metadata['token']});saved=True
                return {'status':'connected','server':server,'credential_file':str(paths()[1]),**metadata}
            if status in {'denied','expired','canceled'}:raise CliError('authorization_'+status,'Authorization '+status+'. Run login to start a new request.')
            if status!='pending':raise CliError('invalid_response','Invalid authorization status.')
            sleep(2)
        raise CliError('authorization_expired','Authorization timed out. Run login again.')
    except CliError as e:
        e.args=(str(e).replace(raw,'[REDACTED]').replace(device if isinstance(device,str) and device else raw,'[REDACTED]'),)
        e.details=None
        raise
    finally:
        if not saved and isinstance(device,str) and len(device)==43:
            try:client.request('POST','/auth/agent/cancel',{'device_code':device})
            except Exception:pass

def logout(Client,CliError):
    profile=load_profile()
    if profile is None:raise CliError('not_logged_in','No saved credential.')
    client=Client({**os.environ,'NOTES_API_URL':profile['api_url'],'NOTES_API_TOKEN':profile['token']})
    try:client.request('POST','/auth/agent/logout')
    except CliError as e:
        if e.status!=401:
            e.args=(str(e).replace(client.token,'[REDACTED]'),);e.details=None;raise
    remove_profile()
    return {'status':'logged_out','remote_credential':'revoked_or_expired'}
