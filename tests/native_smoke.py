"""Disposable Linux CI: real native release/systemd/non-root app + TLS PG16."""
import hashlib
import json
import os
import secrets
import shutil
import socket
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.parse
import urllib.request
from pathlib import Path

if os.environ.get('CI') != 'true' or os.geteuid() != 0:
    raise SystemExit('Run as root only in disposable Linux CI; uses random dedicated fixtures.')
ROOT = Path(__file__).resolve().parents[1]
packages = Path(sys.argv[1]).resolve()
name = 'notes-ci-' + secrets.token_hex(5)
install = Path('/opt') / name
config = Path('/etc') / name
data = Path('/var/lib') / name
base = Path(tempfile.mkdtemp(prefix=name + '-'))
password = secrets.token_hex(32)
email = name + '@example.test'
unit_dir = Path('/etc')/(name+'-units')
db_project = name + '-db'

def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]

port, db_port = free_port(), free_port()
def run(args, **kwargs):
    result = subprocess.run(args, capture_output=True, text=True, timeout=240, **kwargs)
    if result.returncode:
        raise RuntimeError('Command failed: ' + args[0] + ' ' + result.stderr.replace(password, '[redacted]'))
    return result.stdout.strip()

def system(*args):
    return run(['systemctl', *args])

def db(*args):
    return run(['docker', 'compose', '--project-name', db_project, '-f', str(base/'pg.yaml'), *args],
               env={**os.environ, 'TEST_PASSWORD': password})

def admin(*args):
    return run(['bash', str(install/'current/ops/native/admin.sh'), '--config-dir', str(config), *args])

def get(path):
    with urllib.request.urlopen(f'http://127.0.0.1:{port}{path}', timeout=5) as response:
        return response.read()

def ready():
    for _ in range(60):
        try:
            if json.loads(get('/api/v1/health/ready')) == {'status': 'ready'}:
                return
        except Exception:
            pass
        time.sleep(1)
    raise RuntimeError('Native app did not become ready')

certs = base/'certs'
certs.mkdir(mode=0o755)
base.chmod(0o755)
run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj',
     '/CN=localhost', '-keyout', str(certs/'server.key'), '-out', str(certs/'server.crt')])
uid = int(run(['docker', 'run', '--rm', 'postgres:16-alpine', 'id', '-u', 'postgres']))
os.chown(certs/'server.key', uid, uid)
(certs/'server.key').chmod(0o600)
(base/'pg.yaml').write_text(f'''services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: notes
      POSTGRES_DB: notes
      POSTGRES_PASSWORD: ${{TEST_PASSWORD:?}}
    command: [postgres, -c, ssl=on, -c, ssl_cert_file=/certs/server.crt, -c, ssl_key_file=/certs/server.key]
    ports: ['127.0.0.1:{db_port}:5432']
    volumes:
      - database:/var/lib/postgresql/data
      - {certs}:/certs:ro
    healthcheck:
      test: [CMD-SHELL, pg_isready -U notes -d notes]
      interval: 2s
      timeout: 5s
      retries: 30
volumes:
  database:
''')
try:
    db('up', '-d', '--wait', '--wait-timeout', '120')
    # The CLI fixture must be visible inside systemd's private /tmp and home restrictions.
    tools = Path('/opt')/(name+'-fixture')
    tools.mkdir(mode=0o755)
    config.mkdir(mode=0o700)
    fixture = tools/'config.json'
    fixture.write_text(json.dumps({'project':'shier','environment':'prod','revision':1,'values':{
        'db_address':'127.0.0.1','db_port':str(db_port),'notes_db_username':'notes',
        'notes_db_password':password}}))
    fixture.chmod(0o600)
    cli = tools/'confighub'
    cli.write_text(f'#!/usr/bin/env bash\n[[ ! -e "{config}/fail" ]] || exit 7\necho pull >> "{config}/pulls"\ncat "{fixture}"\n')
    cli.chmod(0o755)
    token = tools/'token'
    token.write_text('ci-token-placeholder\n')
    token.chmod(0o600)
    args = ['bash', str(ROOT/'install-native.sh'), '--domain','notes.ci.invalid','--email',email,
            '--version','v0.0.0-ci','--artifact',str(packages/'notes-server_0.0.0-ci_linux_amd64.tar.gz'),
            '--checksum-file',str(packages/'checksums.txt'),'--service-name',name,'--port',str(port),
            '--install-dir',str(install),'--config-dir',str(config),'--data-dir',str(data),
            '--unit-dir',str(unit_dir),
            '--cli-binary',str(cli),'--token-file',str(token),'--skip-dependencies']
    # Installer first-deployment guard requires an empty configuration directory.
    failure = subprocess.run(args+['--token-file',str(tools/'missing-token')],capture_output=True,text=True,timeout=60)
    assert failure.returncode != 0 and not (install/'current').exists()
    assert not (config/'service.env').exists()
    # Correcting a first-install typo must remain retryable before target publication.
    run(args)
    ready()
    assert b'<html' in get('/').lower()
    pid = int(system('show', name+'.service','--property=MainPID','--value'))
    account_uid = int(run(['id','-u',name]))
    assert account_uid != 0 and Path(f'/proc/{pid}').stat().st_uid == account_uid
    assert (config/'service.env').stat().st_mode & 0o777 == 0o600
    assert admin('bootstrap-status') == 'pending'
    assert (config/'.bootstrap-invite').stat().st_mode & 0o777 == 0o600
    assert int(db('exec','-T','db','psql','-U','notes','-d','notes','-At','-c',
        "SELECT count(*) FROM pg_stat_ssl s JOIN pg_stat_activity a ON a.pid=s.pid WHERE a.datname='notes' AND s.ssl")) > 0
    # Reinstallation keeps the existing pending invite and immutable release.
    invite = (config/'.bootstrap-invite').read_bytes()
    run(args)
    assert (config/'.bootstrap-invite').read_bytes() == invite
    # Reject paths that let an unprivileged account replace root-executed scripts.
    unsafe = tools/'user-owned'
    unsafe.mkdir()
    os.chown(unsafe, account_uid, account_uid)
    rejection = subprocess.run(args+['--install-dir',str(unsafe)],capture_output=True,text=True,timeout=60)
    assert rejection.returncode != 0 and '必须属于root' in rejection.stderr
    # An unrelated site linked at a snippet path must never be overwritten.
    snippet = config/'nginx-location.conf'
    saved_snippet = snippet.read_bytes()
    other_site = tools/'other-site.conf'
    other_site.write_text('retain unrelated site\n')
    snippet.unlink()
    snippet.symlink_to(other_site)
    pid_before = system('show',name+'.service','--property=MainPID','--value')
    rejection = subprocess.run(args,capture_output=True,text=True,timeout=60)
    assert rejection.returncode != 0 and '反向代理片段' in rejection.stderr
    assert other_site.read_text() == 'retain unrelated site\n'
    assert system('show',name+'.service','--property=MainPID','--value') == pid_before
    snippet.unlink()
    snippet.write_bytes(saved_snippet)
    snippet.chmod(0o600)
    # Simulate a new release that passes readonly DB preflight but fails migration.
    previous = (install/'current').resolve()
    env_before = (config/'service.env').read_bytes()
    broken = base/'broken'
    broken.mkdir()
    with tarfile.open(packages/'notes-server_0.0.0-ci_linux_amd64.tar.gz') as archive:
        archive.extractall(broken, filter='data')
    (broken/'version.txt').write_text('v0.0.1-ci\n')
    (broken/'shiji').write_text(f'#!/usr/bin/env bash\nif [[ $1 == database-check ]]; then exec "{previous}/shiji" "$@";fi\nexit 9\n')
    (broken/'shiji').chmod(0o755)
    bad_package = base/'notes-server_0.0.1-ci_linux_amd64.tar.gz'
    with tarfile.open(bad_package,'w:gz') as archive:
        for entry in ['shiji','web','ops','version.txt']:
            archive.add(broken/entry,arcname=entry)
    bad_sha = base/'bad-checksums.txt'
    bad_sha.write_text(hashlib.sha256(bad_package.read_bytes()).hexdigest()+'  '+bad_package.name+'\n')
    rejection = subprocess.run(args+['--version','v0.0.1-ci','--artifact',str(bad_package),
                                    '--checksum-file',str(bad_sha)],capture_output=True,text=True,timeout=120)
    assert rejection.returncode != 0 and '恢复上次程序' in rejection.stderr
    assert (install/'current').resolve() == previous
    assert (config/'service.env').read_bytes() == env_before
    ready()
    start = ['bash', str(install/'current/ops/native/start.sh'),'--config-dir',str(config)]
    before = hashlib.sha256((config/'service.env').read_bytes()).hexdigest()
    pid = system('show', name+'.service','--property=MainPID','--value')
    (config/'fail').touch()
    failure = subprocess.run(start, capture_output=True, text=True, timeout=30)
    assert failure.returncode != 0 and password not in failure.stdout+failure.stderr
    assert hashlib.sha256((config/'service.env').read_bytes()).hexdigest() == before
    assert system('show',name+'.service','--property=MainPID','--value') == pid
    ready()
    # Simulate boot: stopping the config oneshot invalidates its active state.
    system('stop',name+'.service',name+'-config.service')
    failure = subprocess.run(['systemctl','start',name+'.service'],capture_output=True,text=True,timeout=60)
    assert failure.returncode != 0
    assert system('show',name+'.service','--property=MainPID','--value') == '0'
    (config/'fail').unlink()
    pulls = len((config/'pulls').read_text().splitlines())
    system('reset-failed',name+'.service',name+'-config.service')
    system('start',name+'.service')
    ready()
    assert len((config/'pulls').read_text().splitlines()) > pulls
    assert admin('bootstrap-status') == 'pending'
    print('PASS: native release/systemd/non-root/TLS PG16, private config/invite, reinstall, unsafe path/snippet rejection, failed upgrade rollback, failed refresh preserves PID/env, boot refresh recovery')
except Exception:
    logs = subprocess.run(['journalctl','-u',name+'.service','-u',name+'-config.service','--no-pager','-n','60'],capture_output=True,text=True)
    print(logs.stdout.replace(password,'[redacted]'))
    raise
finally:
    subprocess.run(['systemctl','stop',name+'.service',name+'-config.service'],capture_output=True)
    subprocess.run(['systemctl','disable',name+'.service',name+'-config.service'],capture_output=True)
    for unit in [unit_dir/(name+'.service'),unit_dir/(name+'-config.service')]:
        unit.unlink(missing_ok=True)
    subprocess.run(['systemctl','daemon-reload'],capture_output=True)
    subprocess.run(['userdel',name],capture_output=True)
    try:
        db('down','--volumes','--remove-orphans')
    except Exception:
        print('Fixture DB cleanup failed')
    # Only the exact random directories created above are removable.
    for folder in [install,config,data,unit_dir,Path('/opt')/(name+'-fixture'),base]:
        assert name in folder.name and folder.resolve() == folder
        if folder.exists():
            shutil.rmtree(folder)
