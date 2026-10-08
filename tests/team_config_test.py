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
        'confighub':'''printf '%s\\n' "$@" > "$TEST_CLI_LOG"
if [[ ${TEST_FETCH_FAIL:-0} == 1 ]];then echo 'ConfigHub: fixture authentication rejected' >&2;exit 9;fi
while (($#));do
  if [[ $1 == --token-file ]];then
    [[ -f $2 ]] || { echo 'ConfigHub: fixture token file missing' >&2;exit 7; }
    shift 2
  else shift;fi
done
cat "$TEST_VALUES"
''',
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
    script_file=ROOT/'deploy/prepare.sh'
    hook=bool(os.environ.get('NOTES_TEST_HOOK'))
    hook_env={}
    if hook:
        # Execute the exact packaged script from outside the checkout.
        script_file=base/'isolated-pre.sh'
        shutil.copyfile(ROOT/'deploy/hooks/pre-install.sh',script_file)
        # Inject one test-only fault precisely between successful publication
        # commands; all normal executions still take the pristine code path.
        script_file.write_text(script_file.read_text(encoding='utf-8').replace(
            'mv -Tf -- "$stage/config.env" "$config_dir/config.env"',
            'if [[ ${TEST_SIGNAL_ZERO_STATUS:-0} == 1 || ${TEST_SIGNAL_FAIL:-0} == 1 ]];then '
            'sed -i s/NOTE_HISTORY_LIMIT=200/NOTE_HISTORY_LIMIT=201/ "$stage/config.env";fi\n'
            'mv -Tf -- "$stage/config.env" "$config_dir/config.env"\n'
            'if [[ ${TEST_SIGNAL_ZERO_STATUS:-0} == 1 ]];then trap - EXIT;cleanup;exit 143;fi\n'
            'if [[ ${TEST_SIGNAL_FAIL:-0} == 1 ]];then kill -TERM $$;fi'),encoding='utf-8',newline='\n')
        hook_env={'DEPLOYCTL_APPLICATION':'notes','DEPLOYCTL_ENVIRONMENT':'prod',
            'DEPLOYCTL_IMAGE':image,'DEPLOYCTL_CONFIG_DIR':path(cfg/'notes/prod'),
            'DEPLOYCTL_PARAM_CLI_BINARY':path(bins/'confighub')}
        args=[]
    def run(*extra,**overrides):
        env={**os.environ,'TEST_BIN':str(bins).replace('\\','/'),'TEST_VALUES':str(values).replace('\\','/'),'TEST_LOG':str(log).replace('\\','/'),'TEST_CLI_LOG':str(base/'cli.log').replace('\\','/'),**hook_env,**overrides}
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$@"'
        return subprocess.run([BASH,'-c',script,'test',str(script_file).replace('\\','/'),*args,*extra],env=env,capture_output=True,text=True,encoding='utf-8',timeout=60)
    if hook:
        snapshot=base/'snapshot';snapshot.mkdir()
        (snapshot/'.env.json').write_text('{}')
        (snapshot/'effective.env').write_text('DATABASE_URL=postgresql://fixture:secret@db.test/notes\n')
        managed=run(DATABASE_URL='postgresql://fixture:secret@db.test/notes',DEPLOYCTL_ENV_FILE=path(snapshot/'.env.json'),TEST_FETCH_FAIL='1',DEPLOYCTL_PARAM_CLI_BINARY=path(base/'absent-cli'),DEPLOYCTL_PARAM_TOKEN_FILE=path(base/'absent-token'))
        assert managed.returncode==0,('Managed DATABASE_URL must not fetch ConfigHub',managed.stdout,managed.stderr)
        assert not (cfg/'notes/prod/secrets.env').exists(),'Managed check must not overwrite local config'
        assert run(DATABASE_URL='sqlite:///bad',DEPLOYCTL_ENV_FILE=path(snapshot/'.env.json')).returncode!=0
    result=run();assert result.returncode==0,(result.stdout,result.stderr)
    assert '--token-file' not in (base/'cli.log').read_text(), 'Default must use existing ConfigHub authentication'
    assert password not in result.stdout+result.stderr and 'never-export' not in result.stdout+result.stderr
    target=cfg/'notes/prod';files=['config.env','secrets.env','.notes-team.json','.database-target']
    before={n:(target/n).read_bytes() for n in files}
    result=run(TEST_FETCH_FAIL='1')
    assert result.returncode==9,(result.returncode,result.stdout,result.stderr)
    assert 'ConfigHub: fixture authentication rejected' in result.stderr
    assert all((target/n).read_bytes()==data for n,data in before.items())
    for invalid in ['{invalid json',json.dumps({'project':'shier','environment':'prod','values':{}})]:
        values.write_text(invalid,encoding='utf-8')
        result=run()
        assert result.returncode==1 and 'ConfigHub返回的配置格式或数据库字段无效' in result.stderr
        assert invalid not in result.stderr and all((target/n).read_bytes()==data for n,data in before.items())
    fixture()
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
    if hook:
        # Bash can enter EXIT with status zero during signal termination.
        assert run(TEST_SIGNAL_ZERO_STATUS='1').returncode!=0
        assert all((target/n).read_bytes()==data for n,data in before.items()), 'Zero-status interrupted publication was not restored'
        assert run(TEST_SIGNAL_FAIL='1').returncode!=0
        assert all((target/n).read_bytes()==data for n,data in before.items()), 'Interrupted publication was not restored'
    fixture(host='other.example.test');assert run().returncode!=0
    assert all((target/n).read_bytes()==data for n,data in before.items())
    fixture(secret='rotated');assert run(**({'APP_ORIGIN':'https://wrong.test'} if hook else {'DATABASE_URL':'wrong','APP_ORIGIN':'https://wrong.test'})).returncode==0
    assert b'rotated' in (target/'secrets.env').read_bytes() and b'wrong.test' not in (target/'config.env').read_bytes()
    if hook:
        assert run(DEPLOYCTL_PARAM_CLI_BINARY='').returncode==0, 'Default must invoke confighub from PATH'
        result=run(DEPLOYCTL_PARAM_CLI_BINARY=path(base/'absent-cli'))
        assert result.returncode==127 and 'absent-cli' in result.stderr,(result.returncode,result.stderr)
        assert run(DEPLOYCTL_IMAGE='ghcr.io/example/fixture:latest').returncode!=0
        assert run(DEPLOYCTL_PARAM_DOMAIN='changed.example.com').returncode!=0
        result=run(DEPLOYCTL_PARAM_TOKEN_FILE=path(base/'missing'))
        assert result.returncode==7 and 'ConfigHub: fixture token file missing' in result.stderr,(result.returncode,result.stderr)
        assert run(DEPLOYCTL_APPLICATION='other').returncode!=0
        assert run(DEPLOYCTL_PARAM_TOKEN_FILE='').returncode==0
    else:
        assert run('--image','ghcr.io/example/fixture:latest').returncode!=0
        assert run('--domain','changed.example.com').returncode!=0
    if os.name!='nt':assert all((target/n).stat().st_mode&0o777==0o600 for n in files)
print('PASS: raw container config, URI encoding, secret isolation, Docker DB preflight, failure/partial publication preservation, target/domain pin and password rotation')
