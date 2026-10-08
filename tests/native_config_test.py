"""Native preflight uses a supplied private environment without remote fetching."""
import json, os, subprocess, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix='notes-native-config-', dir='/root') as temporary:
    base=Path(temporary); cfg=base/'config';cfg.mkdir(mode=0o700); bins=base/'bin';bins.mkdir()
    installed=base/'install';installed.mkdir();data=base/'data';data.mkdir()
    incoming=base/'incoming.env';incoming.write_text('DB_HOST=db.test\nDB_USER=notes_app\nDB_PASSWORD=private-password\n');incoming.chmod(0o600)
    meta=dict(domain='notes.example.com',port='18083',service_user='shiji',install_dir=str(installed),data_dir=str(data),cli='/absent/confighub',token_file='/absent/token',hub_url='removed')
    (cfg/'native.json').write_text(json.dumps(meta));(cfg/'native.json').chmod(0o600)
    for name,raw in {
      'confighub':'exit 99\n',
      'runuser':'while [[ $1 != -- ]];do shift;done;shift;exec "$@"\n',
      'shiji':'[[ $1 == database-check && $DB_HOST == db.test && $DB_PASSWORD == private-password && $REQUIRE_DATABASE_URL == true ]] || exit 3; [[ ${TEST_DB_FAIL:-0} != 1 ]]\n',
    }.items():
      file=bins/name;file.write_text('#!/usr/bin/env bash\n'+raw);file.chmod(0o755)
    env={**os.environ,'PATH':str(bins)+os.pathsep+os.environ['PATH'],'DATABASE_URL':'wrong-ambient','DB_HOST':'wrong-ambient'}
    def run(*extra,**overrides):
      return subprocess.run(['bash',str(ROOT/'notes-server-go/ops/native/prepare.sh'),'--config-dir',str(cfg),'--binary',str(bins/'shiji'),*extra],env={**env,**overrides},capture_output=True,text=True,timeout=30)
    result=run('--env-file',str(incoming));assert result.returncode==0,(result.stdout,result.stderr)
    current=cfg/'service.env';before=current.read_bytes()
    assert b'DB_PASSWORD=private-password' in before and b'LISTEN_ADDR=127.0.0.1:18083' in before
    assert current.stat().st_mode & 0o777 == 0o600
    result=run();assert result.returncode==0,(result.stdout,result.stderr)
    assert current.read_bytes()==before
    assert run(TEST_DB_FAIL='1').returncode!=0 and current.read_bytes()==before
    incoming.chmod(0o644);assert run('--env-file',str(incoming)).returncode!=0 and current.read_bytes()==before
    incoming.chmod(0o600);incoming.write_text('UNKNOWN_SETTING=never-evaluate\n')
    assert run('--env-file',str(incoming)).returncode!=0 and current.read_bytes()==before
print('PASS: native private input, DB fields, ambient isolation, no ConfigHub and unchanged configuration on failed preflight')
