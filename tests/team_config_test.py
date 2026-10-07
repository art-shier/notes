"""Real Bash/jq config rendering; Docker/CLI/root boundaries are mocked."""
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASH=shutil.which('bash') if os.name!='nt' else 'C:/Program Files/Git/bin/bash.exe'
JQ=shutil.which('jq') if os.name!='nt' else os.environ['NOTES_TEST_JQ']
with tempfile.TemporaryDirectory(prefix='notes-team-config-',dir=ROOT.parent) as temporary:
    base=Path(temporary);bins=base/'bin';bins.mkdir();cfg=base/'config';values=base/'values.json'
    log=base/'docker.log';password="s ecret'$(touch PWNED)\nnext"
    def path(p):return '/'+str(p).replace('\\','/').replace(':','').lower() if os.name=='nt' else str(p)
    def fixture(secret=password,host='db.example.test'):
        values.write_text(json.dumps({'project':'shier','environment':'prod','values':{
            'db_address':host,'db_port':'5432','notes_db_username':'notes_app','notes_db_password':secret,'unrelated_secret':'never-export'}}),encoding='utf-8')
    fixture()
    jq=bins/('jq.exe' if os.name=='nt' else 'jq');shutil.copyfile(JQ,jq);jq.chmod(0o755)
    scripts={
        'uname':'echo Linux\n','id':'echo 0\n',
        'stat':'case "$*" in *%a*) echo 600;; *) echo 0;; esac\n',
        'confighub':'[[ ${TEST_FETCH_FAIL:-0} != 1 ]] || exit 9;cat "$TEST_VALUES"\n',
        'mv':'if [[ ${TEST_PUBLISH_FAIL:-0} == 1 && "${@: -1}" == */.notes-team.json ]];then exit 9;fi;exec /usr/bin/mv "$@"\n',
        'docker':'''[[ $1 == run ]] || exit 2
echo check >> "$TEST_LOG"
while (($#)); do
  if [[ $1 == --env-file ]]; then
    [[ -f $2 ]] || exit 3
    case $2 in */config.env) grep -qx 'LISTEN_ADDR=0.0.0.0:8000' "$2" || exit 4;; */secrets.env) grep -q '^DATABASE_URL=postgresql://notes_app:' "$2" || exit 5;; esac
    shift 2
  else shift;fi
done
if [[ ${TEST_DB_FAIL:-0} == 1 ]];then echo 'sensitive-db-error' >&2;exit 8;fi
'''}
    for name,body in scripts.items():
        p=bins/name;p.write_text('#!/usr/bin/env bash\n'+body,encoding='utf-8',newline='\n');p.chmod(0o755)
    image='ghcr.io/example/fixture@sha256:'+'0'*64  # Test fixture, never published.
    args=['--image',image,'--config-root',path(cfg),'--cli-binary',path(bins/'confighub')]
    def run(*extra,**overrides):
        env={**os.environ,'TEST_BIN':str(bins).replace('\\','/'),'TEST_VALUES':str(values).replace('\\','/'),'TEST_LOG':str(log).replace('\\','/'),**overrides}
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$@"'
        return subprocess.run([BASH,'-c',script,'test',str(ROOT/'deploy/prepare.sh').replace('\\','/'),*args,*extra],env=env,capture_output=True,text=True,encoding='utf-8',timeout=60)
    result=run();assert result.returncode==0,(result.stdout,result.stderr)
    target=cfg/'notes/prod';files=['config.env','secrets.env','.notes-team.json','.database-target']
    before={n:(target/n).read_bytes() for n in files}
    assert b'COOKIE_SECURE=true' in before['config.env'] and b'WEB_DIR=/app/web' in before['config.env']
    assert b'APP_ORIGIN=https://notes.shier.art\n' in before['config.env']
    assert b's%20ecret%27%24%28touch%20PWNED%29%0Anext' in before['secrets.env']
    assert b'never-export' not in before['secrets.env'] and not (base/'PWNED').exists()
    for overrides in [{'TEST_FETCH_FAIL':'1'},{'TEST_DB_FAIL':'1'}]:
        result=run(**overrides);assert result.returncode!=0 and 'sensitive-db-error' not in result.stdout+result.stderr
        assert all((target/n).read_bytes()==data for n,data in before.items())
    fixture(secret='failed-rotation')
    assert run(TEST_PUBLISH_FAIL='1').returncode!=0
    assert all((target/n).read_bytes()==data for n,data in before.items()), 'Partial config publication was not restored'
    fixture(host='other.example.test');assert run().returncode!=0
    assert all((target/n).read_bytes()==data for n,data in before.items())
    fixture(secret='rotated');assert run(DATABASE_URL='wrong',APP_ORIGIN='https://wrong.test').returncode==0
    assert b'rotated' in (target/'secrets.env').read_bytes() and b'wrong.test' not in (target/'config.env').read_bytes()
    assert run('--image','ghcr.io/example/fixture:latest').returncode!=0
    assert run('--domain','changed.example.com').returncode!=0
    if os.name!='nt':assert all((target/n).stat().st_mode&0o777==0o600 for n in files)
print('PASS: raw container config, URI encoding, secret isolation, Docker DB preflight, failure/partial publication preservation, target/domain pin and password rotation')
