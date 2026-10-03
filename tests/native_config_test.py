"""Real Bash/jq native configuration; external CLI/privilege/Go boundaries are fixtures."""
import json, os, shutil, subprocess, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
BASH=shutil.which('bash') if os.name!='nt' else 'C:/Program Files/Git/bin/bash.exe'
JQ=shutil.which('jq') if os.name!='nt' else os.environ['NOTES_TEST_JQ']
prepare=ROOT/'notes-server-go/ops/native/prepare.sh'
assert prepare.is_file(),'Native startup configuration is not implemented'
with tempfile.TemporaryDirectory(prefix='notes-native-config-',dir=ROOT.parent) as temporary:
    base=Path(temporary);cfg=base/'config';cfg.mkdir();bin_dir=base/'bin';bin_dir.mkdir();data=base/'data';data.mkdir();installed=base/'install';installed.mkdir()
    shutil.copyfile(JQ,bin_dir/('jq.exe' if os.name=='nt' else 'jq'));(bin_dir/('jq.exe' if os.name=='nt' else 'jq')).chmod(0o755)
    raw=base/'values.json';log=base/'log'
    def fixture(password='s ecret\'$(touch PWNED)\nnext',host='db.example.test'):
        raw.write_text(json.dumps({'project':'shier','environment':'prod','revision':7,'values':{'db_address':host,'db_port':'5432','notes_db_username':'notes_app','notes_db_password':password,'unrelated_secret':'never-export'}}))
    fixture()
    def posix(path):return '/'+str(path).replace('\\','/').replace(':','').lower() if os.name=='nt' else str(path)
    for name,content in {
        'id':'echo 0\n',
        'stat':'case "$*" in *%a*) echo 600;; *) echo 0;; esac\n',
        'confighub':'echo fetch >> "$TEST_LOG"; [[ ${TEST_FAIL:-0} != 1 ]] || exit 9; cat "$TEST_JSON"\n',
        'runuser':'while [[ $1 != -- ]]; do shift; done; shift; exec "$@"\n',
        'shiji':'[[ $1 == database-check ]] || exit 3; [[ $DATABASE_URL == postgresql://notes_app:* ]] || exit 4; echo preflight >> "$TEST_LOG"; [[ ${TEST_DB_FAIL:-0} != 1 ]]\n',
    }.items():
        p=bin_dir/name;p.write_text('#!/usr/bin/env bash\n'+content);p.chmod(0o755)
    meta=dict(domain='notes.example.com',port='18083',service_user='shiji',install_dir=posix(installed),data_dir=posix(data),hub_url='https://config.example.test',hub_project='shier',hub_env='prod',database='notes',cli=posix(bin_dir/'confighub'),token_file='')
    (cfg/'native.json').write_text(json.dumps(meta));(cfg/'native.json').chmod(0o600)
    def call(**extra):
        env={**os.environ,'TEST_BIN':str(bin_dir).replace('\\','/'),'TEST_JSON':str(raw).replace('\\','/'),'TEST_LOG':str(log).replace('\\','/'),**extra}
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$@"'
        return subprocess.run([BASH,'-c',script,'test',str(prepare).replace('\\','/'),'--config-dir',posix(cfg),'--binary',posix(bin_dir/'shiji')],env=env,capture_output=True,text=True,encoding='utf-8',timeout=20)
    result=call();assert result.returncode==0,(result.stdout,result.stderr)
    generated=cfg/'service.env';original=generated.read_bytes()
    assert b'LISTEN_ADDR=127.0.0.1:18083' in original and b'COOKIE_SECURE=true' in original and b's%20ecret%27%24%28touch%20PWNED%29%0Anext' in original
    assert b'never-export' not in original and not (base/'PWNED').exists()
    assert call(TEST_FAIL='1').returncode!=0 and generated.read_bytes()==original
    assert call(TEST_DB_FAIL='1').returncode!=0 and generated.read_bytes()==original
    fixture(host='another.example.test');assert call().returncode!=0 and generated.read_bytes()==original
    fixture(password='rotated');assert call(DATABASE_URL='wrong',APP_ORIGIN='https://wrong.example',COOKIE_SECURE='false').returncode==0
    assert b'rotated' in generated.read_bytes() and b'wrong.example' not in generated.read_bytes()
    if os.name!='nt':assert generated.stat().st_mode&0o777==0o600
print('PASS: native private config, shared address/port + dedicated account, URI encoding, env isolation, failure preservation, target pin and password refresh')
