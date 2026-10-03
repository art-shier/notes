"""Execute real startup/jq against disposable config and fake network/containers."""
import json, os, shutil, subprocess, tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BASH = shutil.which('bash') if os.name != 'nt' else 'C:/Program Files/Git/bin/bash.exe'
JQ = shutil.which('jq') if os.name != 'nt' else str(ROOT.parent/'work/jq.exe')
with tempfile.TemporaryDirectory(prefix='confighub-start-') as temp:
    base = Path(temp); server = base/'notes-server-go'; (server/'ops').mkdir(parents=True)
    for name in ('start.sh',):
        source = ROOT/'notes-server-go/ops'/name
        assert source.exists(), 'startup pipeline is not implemented'
        shutil.copyfile(source, server/'ops'/name)
    shutil.copyfile(ROOT/'notes-server-go/compose.external.yaml', server/'compose.external.yaml')
    bin = base/'bin'; bin.mkdir(); shutil.copyfile(JQ, bin/('jq.exe' if os.name == 'nt' else 'jq')); (bin/('jq.exe' if os.name == 'nt' else 'jq')).chmod(0o755)
    config = base/'config.json'; log = base/'log'
    def fixture(password='secret @:$\'\\$(touch PWNED)\nnext', host='db.example.test', user='notes_user'):
        config.write_text(json.dumps({'project':'shier','environment':'prod','revision':6,'values': {'db_address':host,'db_port':'5432','db_username':user,'db_password':password,'irrelevant_smtp_password':'do-not-export'}}))
    fixture()
    stubs = {
        'confighub': '''#!/usr/bin/env bash
printf 'fetch %s\\n' "$*" >> "$TEST_LOG"
[[ ${TEST_FETCH_FAIL:-0} != 1 ]] || exit 7
cat "$TEST_CONFIG"
''',
        'docker': '''#!/usr/bin/env bash
[[ -z ${DATABASE_URL:-} && -z ${COMPOSE_FILE:-} ]] || exit 88
printf 'docker %s\\n' "$*" >> "$TEST_LOG"
if [[ $* == *database-check* && ${TEST_DB_FAIL:-0} == 1 ]]; then exit 9; fi
exit 0
''',
    }
    for name, source in stubs.items():
        p=bin/name; p.write_text(source); p.chmod(0o755)
    def call(*args, **extra):
        env={**os.environ, 'TEST_BIN':str(bin).replace('\\','/'), 'TEST_CONFIG':str(config).replace('\\','/'), 'TEST_LOG':str(log).replace('\\','/'), **extra}
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$@"'
        return subprocess.run([BASH,'-c',script,'test',str(server/'ops/start.sh').replace('\\','/'),*args],env=env,cwd=base,capture_output=True,text=True,encoding='utf-8',timeout=20)
    args=('--domain','notes.example.com','--project','shiji','--config-hub-url','https://config.example.test','--config-hub-project','shier','--config-hub-env','prod')
    r=call(*args); assert r.returncode==0,(r.stdout,r.stderr)
    envfile=server/'.env'; before=envfile.read_text()
    assert "DATABASE_URL='postgresql://notes_user:secret%20%40%3A%24%27%5C%24%28touch%20PWNED%29%0Anext@db.example.test:5432/notes?sslmode=require&connect_timeout=10'" in before
    assert 'POSTGRES_PASSWORD' not in before and 'COMPOSE_FILE=compose.external.yaml' in before
    assert not (base/'PWNED').exists()
    assert 'do-not-export' not in before+r.stdout+r.stderr+log.read_text()
    if os.name!='nt': assert envfile.stat().st_mode&0o777==0o600
    fixture(password='rotated-password')
    r=call(DATABASE_URL='postgresql://ambient/wrong',COMPOSE_FILE='wrong.yaml'); assert r.returncode==0,(r.stdout,r.stderr)
    assert 'rotated-password' in envfile.read_text() and log.read_text().count('fetch ')==2
    stable=envfile.read_bytes(); stable_log=log.read_text()
    r=call(TEST_FETCH_FAIL='1'); assert r.returncode!=0 and envfile.read_bytes()==stable
    assert 'up -d' not in log.read_text()[len(stable_log):]
    checkpoint=log.read_text(); fixture(host='other.example.test')
    r=call(); assert r.returncode!=0 and envfile.read_bytes()==stable
    assert 'up -d' not in log.read_text()[len(checkpoint):]
    fixture(password='bad-db-password'); checkpoint=log.read_text()
    r=call(TEST_DB_FAIL='1'); assert r.returncode!=0 and envfile.read_bytes()==stable
    assert 'up -d' not in log.read_text()[len(checkpoint):]
    config.write_text(json.dumps({'project':'shier','environment':'prod','values':{'db_address':'db.example.test'}}))
    r=call(); assert r.returncode!=0 and envfile.read_bytes()==stable
    fixture(); (server/'.install.lock').mkdir()
    r=call(); assert r.returncode!=0 and envfile.read_bytes()==stable
    (server/'.install.lock').rmdir()
    r=call('--database-name','postgres'); assert r.returncode!=0 and envfile.read_bytes()==stable
    # An existing standalone configuration must never be silently switched.
    (server/'.database-target').unlink(); (server/'.config-hub.json').unlink()
    envfile.write_text('DOMAIN=notes.example.com\nPOSTGRES_PASSWORD=local-secret\n')
    local=envfile.read_bytes(); r=call(*args)
    assert r.returncode!=0 and envfile.read_bytes()==local
    if os.name!='nt':
        envfile.unlink(); envfile.symlink_to(config)
        r=call(*args); assert r.returncode!=0
print('PASS: ConfigHub startup, URI encoding, refresh, secret isolation, fetch/DB/validation failure preservation, target pin, locks, standalone protection')
