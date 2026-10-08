"""CI-only: real Compose consumes existing private env against TLS PostgreSQL."""
import hashlib, json, os, secrets, shutil, subprocess, tempfile
from pathlib import Path

if os.environ.get('CI') != 'true':
    raise SystemExit('Run only in disposable CI; deletes only its own random Docker fixture projects.')
ROOT=Path(__file__).resolve().parents[1]
base=Path(tempfile.mkdtemp(prefix='notes-startup-ci-'))
db_project='notes-tls-'+secrets.token_hex(5); app_project='notes-start-'+secrets.token_hex(5)
bundled_project='notes-bundled-'+secrets.token_hex(5)
password=secrets.token_hex(32)
env={**os.environ}
def run(args, **kwargs):
    result=subprocess.run(args, capture_output=True, text=True, **kwargs)
    if result.returncode:
        # Child errors must not expose generated credentials.
        raise RuntimeError('Command failed: '+args[0]+' '+result.stderr.replace(password,'[redacted]'))
    return result.stdout.strip()

certs=base/'certs'; certs.mkdir(); certs.chmod(0o755)
run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-subj','/CN=host.docker.internal','-keyout',str(certs/'server.key'),'-out',str(certs/'server.crt')])
uid=run(['docker','run','--rm','postgres:16-alpine','id','-u','postgres'])
run(['sudo','chown',uid+':'+uid,str(certs/'server.key')])
run(['sudo','chmod','600',str(certs/'server.key')])
gateway=run(['docker','network','inspect','bridge','--format','{{(index .IPAM.Config 0).Gateway}}'])
pg_file=base/'pg.yaml'
pg_file.write_text(f'''services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: notes
      POSTGRES_DB: notes
      POSTGRES_PASSWORD: ${{TEST_PASSWORD:?}}
    command: [postgres, -c, ssl=on, -c, ssl_cert_file=/certs/server.crt, -c, ssl_key_file=/certs/server.key]
    ports:
      - '{gateway}:15432:5432'
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
db_env={**env,'TEST_PASSWORD':password}
def db(*args):return run(['docker','compose','--project-name',db_project,'-f',str(pg_file),*args],env=db_env)
fixture=base/'checkout'
shutil.copytree(ROOT,fixture,ignore=shutil.ignore_patterns('.git','node_modules','dist','data','__pycache__'))
server=fixture/'notes-server-go'
current=server/'.env'
current.write_text(f'DOMAIN=notes.ci.invalid\nCOMPOSE_PROJECT_NAME={app_project}\nCOMPOSE_FILE=compose.external.yaml\nDATABASE_URL=postgresql://notes:{password}@host.docker.internal:15432/notes?sslmode=require\n');current.chmod(0o600)
start_env=env
bundled_server=None
def app(*args):return run(['docker','compose','--project-name',app_project,'--project-directory',str(server),'--env-file',str(server/'.env'),*args],env=env,cwd=server)
def bundled(*args):return run(['docker','compose','--project-name',bundled_project,'--project-directory',str(bundled_server),'--env-file',str(bundled_server/'.env'),*args],env=env,cwd=bundled_server)
try:
    db('up','-d','--wait','--wait-timeout','120')
    run(['bash',str(server/'ops/start.sh')],env=start_env)
    assert set(app('config','--services').splitlines())=={'app','caddy'}
    assert (server/'.env').stat().st_mode&0o777==0o600
    assert app('exec','-T','app','shiji','bootstrap-status')=='empty'
    assert int(db('exec','-T','db','psql','-U','notes','-d','notes','-At','-c',"SELECT count(*) FROM pg_stat_ssl s JOIN pg_stat_activity a ON a.pid=s.pid WHERE a.datname='notes' AND s.ssl"))>0
    before=hashlib.sha256((server/'.env').read_bytes()).hexdigest(); container=app('ps','-q','app')
    db('stop','db')
    failure=subprocess.run(['bash',str(server/'ops/start.sh')],env=start_env,capture_output=True,text=True)
    assert failure.returncode!=0 and password not in failure.stdout+failure.stderr
    assert hashlib.sha256((server/'.env').read_bytes()).hexdigest()==before and app('ps','-q','app')==container
    # Exercise the real default Compose graph with no existing DB container,
    # then with its DB stopped. This cannot rely on a fixture command stub.
    app('stop','caddy')  # Release 80/443 for the independent bundled fixture.
    bundled_checkout=base/'bundled-checkout';shutil.copytree(fixture,bundled_checkout)
    bundled_server=bundled_checkout/'notes-server-go'
    bundled_env=bundled_server/'.env'
    bundled_env.write_text(f'DOMAIN=notes.bundled.invalid\nCOMPOSE_PROJECT_NAME={bundled_project}\nPOSTGRES_PASSWORD={password}\n');bundled_env.chmod(0o600)
    bundled_before=bundled_env.read_bytes()
    run(['bash',str(bundled_server/'ops/start.sh')],env=start_env)
    assert set(bundled('config','--services').splitlines())=={'app','db','caddy'}
    assert bundled('exec','-T','app','shiji','bootstrap-status')=='empty'
    bundled('stop','app','db')
    run(['bash',str(bundled_server/'ops/start.sh')],env=start_env)
    assert bundled('exec','-T','app','shiji','database-check')==''
    assert bundled_env.read_bytes()==bundled_before
    print('PASS: private external env/TLS PG16 and failure preservation; default bundled DB startup and stopped-DB recovery')
finally:
    if bundled_server is not None:
        subprocess.run(['docker','compose','--project-name',bundled_project,'--project-directory',str(bundled_server),'--env-file',str(bundled_server/'.env'),'down','--volumes','--remove-orphans'],env=env,cwd=bundled_server,capture_output=True,text=True)
    # Down only these fixture names. Dummy env supports cleanup before config publication.
    cleanup_env={**env,'DOMAIN':'notes.ci.invalid','DATABASE_URL':'postgresql://unused/notes'}
    subprocess.run(['docker','compose','--project-name',app_project,'-f',str(server/'compose.external.yaml'),'down','--volumes','--remove-orphans'],env=cleanup_env,capture_output=True,text=True)
    try:db('down','--volumes','--remove-orphans')
    except Exception as error:print('CI cleanup failed:',type(error).__name__)
