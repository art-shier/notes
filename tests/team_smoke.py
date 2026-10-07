"""Disposable root Linux CI: exact packaged hooks + real ctl/Docker + TLS PG16."""
import copy
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen

if os.environ.get('CI')!='true' or os.geteuid()!=0:
    raise SystemExit('Only run as root in disposable Linux CI.')
ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(Path(sys.argv[1]).resolve()))
from deployctl.contract import read_yaml
from deployctl.release import build_release
from deployctl.runtime import Manager, project_name

name='notes-team-ci-'+secrets.token_hex(5)
# Root-only ancestors are part of the real hook contract; /tmp is unsuitable.
base=Path('/')/name;base.mkdir(mode=0o700)
password=secrets.token_hex(32)
deploy_env='ci-'+name.rsplit('-',1)[1]
registry=name+'-registry';db_project=name+'-db';app_project=project_name('notes',deploy_env)
def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1',0));return sock.getsockname()[1]
port,db_port,registry_port=free_port(),free_port(),free_port()
def run(args,**kwargs):
    result=subprocess.run([str(x) for x in args],capture_output=True,text=True,timeout=240,**kwargs)
    if result.returncode:
        # Never echo child logs, credentials or invitation URLs.
        raise RuntimeError('Fixture command failed: '+str(args[0]))
    return result.stdout.strip()
def db(*args):
    return run(['docker','compose','-p',db_project,'-f',base/'pg.yaml',*args],env={**os.environ,'TEST_PASSWORD':password})
def container():
    return run(['docker','ps','-q','--filter','label=com.docker.compose.project='+app_project])
def values():
    return json.loads(run(['docker','exec',container(),'sh','-c','cat "$DEPLOYCTL_ENV_FILE"']))
manager=Manager(base/'apps',base/'config')
folder=base/'config/notes'/deploy_env
certs=base/'certs';certs.mkdir(mode=0o755)
cli=base/'confighub';payload=base/'hub.json';failure_marker=base/'fetch-failed'
cli.write_text(f'#!/usr/bin/env bash\n[[ ! -e {failure_marker} ]] || exit 7\ncat {payload}\n');cli.chmod(0o700)
token=base/'token';token.write_text('fixture-only');token.chmod(0o600)
params={'CLI_BINARY':str(cli),'TOKEN_FILE':str(token)}
try:
    gateway=run(['docker','network','inspect','bridge','--format','{{(index .IPAM.Config 0).Gateway}}'])
    run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-subj','/CN=notes-ci','-keyout',certs/'server.key','-out',certs/'server.crt'])
    uid=int(run(['docker','run','--rm','postgres:16-alpine','id','-u','postgres']))
    os.chown(certs/'server.key',uid,uid);(certs/'server.key').chmod(0o600)
    (base/'pg.yaml').write_text(f'''services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: notes
      POSTGRES_DB: notes
      POSTGRES_PASSWORD: ${{TEST_PASSWORD:?}}
    command: [postgres, -c, ssl=on, -c, ssl_cert_file=/certs/server.crt, -c, ssl_key_file=/certs/server.key]
    ports: ['{gateway}:{db_port}:5432']
    volumes:
      - {certs}:/certs:ro
    healthcheck:
      test: [CMD-SHELL, pg_isready -U notes -d notes]
      interval: 2s
      timeout: 5s
      retries: 30
''')
    db('up','-d','--wait','--wait-timeout','120')
    payload.write_text(json.dumps({'project':'shier','environment':'prod','values':{
        'db_address':gateway,'db_port':str(db_port),'notes_db_username':'notes','notes_db_password':password}}));payload.chmod(0o600)
    run(['docker','run','-d','--name',registry,'-p',f'127.0.0.1:{registry_port}:5000','registry:2'])
    for attempt in range(30):
        try:
            with urlopen(f'http://127.0.0.1:{registry_port}/v2/',timeout=1):break
        except OSError:
            if attempt==29:raise RuntimeError('Fixture registry not ready')
            time.sleep(1)
    tag=f'127.0.0.1:{registry_port}/notes:fixture'
    run(['docker','tag','shiji-ci',tag]);run(['docker','push',tag])
    image=run(['docker','image','inspect',tag,'--format','{{index .RepoDigests 0}}'])
    config=read_yaml(ROOT/'deploy/deployment.yaml')
    package=build_release(config,image,'v0.0.0-ci',base/'packages',project_root=ROOT)
    assert not (folder/'secrets.env').exists()
    first=manager.deploy('notes',deploy_env,package,port=port,install_params=params)
    assert first['transaction'] is None and 'DATABASE_URL' in values()
    assert run(['docker','exec',container(),'shiji','bootstrap-status'])=='empty'
    assert int(db('exec','-T','db','psql','-U','notes','-d','notes','-At','-c',"SELECT count(*) FROM pg_stat_ssl s JOIN pg_stat_activity a ON a.pid=s.pid WHERE a.datname='notes' AND s.ssl"))>0
    for file in ('config.env','secrets.env','.notes-team.json','.database-target'):
        assert (folder/file).stat().st_mode&0o777==0o600
    before=container();source=(folder/'secrets.env').read_bytes()
    failure_marker.touch()
    try:
        manager.deploy('notes',deploy_env,package,upgrade=True,install_params=params)
        raise AssertionError('Failed fetch was accepted')
    except RuntimeError as error:
        assert password not in str(error)
        assert 'pre_install' in str(error)
    assert container()==before and (folder/'secrets.env').read_bytes()==source
    failure_marker.unlink()
    second=manager.deploy('notes',deploy_env,package,upgrade=True,install_params=params,
        runtime_env={'APP_ORIGIN':'https://override.example.test'})
    assert second['previous']==first['current'] and values()['APP_ORIGIN']=='https://override.example.test'
    manager.rollback('notes',deploy_env)
    assert values()['APP_ORIGIN']=='https://notes.shier.art'
    # A real failing post hook must restore the old successful runtime.
    failing=copy.deepcopy(config);failing['hooks']['post_install']['script']='post-fail.sh'
    (base/'post-fail.sh').write_text('exit 7\n')
    (base/'pre.sh').write_bytes((ROOT/'deploy/hooks/pre-install.sh').read_bytes())
    failing['hooks']['pre_install']['script']='pre.sh'
    bad_package=build_release(failing,image,'v0.0.1-ci',base/'packages',project_root=base)
    try:
        manager.deploy('notes',deploy_env,bad_package,upgrade=True,install_params=params,
            runtime_env={'APP_ORIGIN':'https://failed.example.test'})
        raise AssertionError('Failed post was accepted')
    except RuntimeError as error:
        assert password not in str(error)
        assert 'post_install' in str(error)
    diagnostic=json.loads((base/'apps/notes'/deploy_env/'last-failure.json').read_text())
    assert diagnostic['phase']=='post_install' and diagnostic['version']=='v0.0.1-ci'
    current=json.loads((base/'apps/notes'/deploy_env/'state.json').read_text())
    assert current['transaction'] is None and current['current']==first['current']
    assert values()['APP_ORIGIN']=='https://notes.shier.art'
    # Bootstrap uses effective overrides, then preserves the pending invitation.
    manager.deploy('notes',deploy_env,package,upgrade=True,install_params={**params,'ADMIN_EMAIL':'ci@example.test'})
    assert run(['docker','exec',container(),'shiji','bootstrap-status'])=='pending'
    manager.deploy('notes',deploy_env,package,upgrade=True,install_params={**params,'ADMIN_EMAIL':'other@example.test'})
    assert run(['docker','exec',container(),'shiji','bootstrap-status'])=='pending'
    missing=subprocess.run(['docker','run','--rm','--entrypoint','shiji',image,'database-check'],capture_output=True,text=True)
    assert missing.returncode!=0 and 'DATABASE_URL must be generated' in missing.stderr
    print('PASS: packaged pre -> config refresh -> real Notes/TLS PG16, effective overrides, rollback, failed fetch/post and bootstrap idempotence')
finally:
    # These names and the root directory are unique fixtures created above.
    identifiers=subprocess.run(['docker','ps','-aq','--filter','label=com.docker.compose.project='+app_project],capture_output=True,text=True).stdout.split()
    if identifiers:subprocess.run(['docker','rm','-f',*identifiers],capture_output=True)
    subprocess.run(['docker','network','rm',app_project+'_default'],capture_output=True)
    subprocess.run(['docker','compose','-p',db_project,'-f',str(base/'pg.yaml'),'down','--volumes'],env={**os.environ,'TEST_PASSWORD':password},capture_output=True)
    subprocess.run(['docker','rm','-f',registry],capture_output=True)
    shutil.rmtree(base)
