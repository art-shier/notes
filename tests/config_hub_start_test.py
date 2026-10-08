"""Existing standalone env starts without ConfigHub, with readonly DB preflight."""
import os, shutil, subprocess, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix='notes-local-start-') as temporary:
    base=Path(temporary); server=base/'notes-server-go';(server/'ops').mkdir(parents=True)
    for name in ('start.sh',):
      shutil.copyfile(ROOT/'notes-server-go/ops'/name,server/'ops'/name)
    bins=base/'bin';bins.mkdir();log=base/'log'
    for name,raw in {
      'confighub':'echo unexpected-fetch >> "$TEST_LOG";exit 99\n',
      'docker':'''printf "%s\\n" "$*" >> "$TEST_LOG"
if [[ $* == *"config --services"* ]]; then
  printf 'app\\ncaddy\\n'
  [[ ${TEST_BUNDLED:-0} != 1 ]] || printf 'db\\n'
  exit 0
fi
if [[ ${TEST_BUNDLED:-0} == 1 && $* == *"up -d --wait"* && ${@: -1} == db ]]; then touch "$TEST_DB_READY";fi
if [[ $* == *database-check* ]]; then
  [[ ${TEST_DB_FAIL:-0} != 1 ]] || exit 8
  [[ ${TEST_BUNDLED:-0} != 1 || -f $TEST_DB_READY ]] || { echo 'bundled database is stopped' >&2;exit 9; }
fi
''',
    }.items():
      file=bins/name;file.write_text('#!/usr/bin/env bash\n'+raw);file.chmod(0o755)
    current=server/'.env';current.write_text('DOMAIN=notes.example.com\nCOMPOSE_FILE=compose.external.yaml\nDATABASE_URL=postgresql://notes_app:encoded@db.test/notes\n');current.chmod(0o600)
    (server/'.config-hub.json').write_text('{"cli":"/absent/confighub"}')
    before=current.read_bytes()
    env={**os.environ,'PATH':str(bins)+os.pathsep+os.environ['PATH'],'TEST_LOG':str(log),'TEST_DB_READY':str(base/'db-ready'),'DATABASE_URL':'wrong-ambient','DB_HOST':'wrong-ambient'}
    def run(*args,**overrides):
      return subprocess.run(['bash',str(server/'ops/start.sh'),*args],env={**env,**overrides},capture_output=True,text=True,timeout=30)
    result=run();assert result.returncode==0,(result.stdout,result.stderr)
    assert 'database-check' in log.read_text() and 'unexpected-fetch' not in log.read_text()
    assert current.read_bytes()==before
    checkpoint=log.read_text()
    assert run(TEST_DB_FAIL='1').returncode!=0 and current.read_bytes()==before
    assert 'up -d' not in log.read_text()[len(checkpoint):]
    current.write_text('DOMAIN=notes.example.com\nPOSTGRES_PASSWORD=local-encoded-secret\n')
    bundled=current.read_bytes()
    result=run(TEST_BUNDLED='1')
    assert result.returncode==0, ('Stopped bundled database must start before readonly preflight', result.stdout, result.stderr)
    assert current.read_bytes()==bundled and (base/'db-ready').exists()
    (base/'db-ready').unlink()
    result=run(TEST_BUNDLED='1')
    assert result.returncode==0 and current.read_bytes()==bundled, ('Bundled database restart must remain repeatable', result.stderr)
    current.write_bytes(before)
    assert run('--config-hub-url','https://config.test').returncode!=0 and current.read_bytes()==before
    (server/'.install.lock').mkdir();assert run().returncode!=0;(server/'.install.lock').rmdir()
    current.chmod(0o644);assert run().returncode!=0;current.chmod(0o600)
    current.unlink();current.symlink_to(server/'.config-hub.json');assert run().returncode!=0
print('PASS: existing standalone env, no ConfigHub, readonly preflight, failure preservation, removed parameters, locks and private file protections')
