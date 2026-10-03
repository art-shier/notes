import os,subprocess,tempfile,stat
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
import shutil
BASH=shutil.which('bash') if os.name!='nt' else 'C:/Program Files/Git/bin/bash.exe'
with tempfile.TemporaryDirectory(prefix='deploy-test-',dir=None) as temp:
    base=Path(temp);bin=base/'bin';bin.mkdir();fixture=base/'fixture';(fixture/'notes-server-go').mkdir(parents=True);(fixture/'notes-server-go/compose.yaml').write_text('services: {}\n')
    fake={
    'uname':'#!/usr/bin/env bash\necho Linux\n',
    'git':'''#!/usr/bin/env bash
printf '%s\\n' "$*" >> "$TEST_LOG"
if [[ $1 == -c ]]; then shift 2; fi
if [[ $1 == clone ]]; then dest=${@: -1}; mkdir -p "$dest/.git"; cp -R "$TEST_SOURCE"/. "$dest/"; exit; fi
if [[ $* == *"remote get-url origin"* ]]; then echo https://github.com/art-shier/notes.git; exit; fi
exit 0
''',
    'docker':'''#!/usr/bin/env bash
printf '%s\\n' "$*" >> "$TEST_LOG"
if [[ $1 == info || $* == *"compose version"* ]]; then exit 0; fi
if [[ $* == *"psql"* ]]; then echo "${TEST_STATE:-empty}"; exit 0; fi
if [[ $* == *"shiji bootstrap"* ]]; then echo 'https://notes.example.com/?invite=test-invitation-secret&email=admin@example.test'; exit 0; fi
if [[ $* == *"build"* && ${TEST_BUILD_FAIL:-0} == 1 ]]; then exit 12; fi
exit 0
''',
    'curl':'''#!/usr/bin/env bash
printf '%s\\n' "$*" >> "$TEST_LOG"
if [[ ${TEST_TLS_FAIL:-0} == 1 ]]; then exit 60; fi
echo '{"status":"ready"}'
'''}
    for name,data in fake.items():p=bin/name;p.write_text(data);p.chmod(0o755)
    def call(directory,*args,**extra):
        env={**os.environ,'PATH':str(bin).replace('\\','/')+os.pathsep+os.environ['PATH'],'TEST_SOURCE':str(fixture).replace('\\','/'),'TEST_LOG':str(base/'log').replace('\\','/'),**extra}
        # Set PATH inside Bash so Git for Windows does not reorder test command stubs.
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$TEST_INSTALL" "$@"'
        env.update(TEST_BIN=str(bin).replace('\\','/'),TEST_INSTALL=str(ROOT/'install.sh').replace('\\','/'))
        return subprocess.run([BASH,'-c',script,'test','--dir',str(directory).replace('\\','/'),*args],env=env,text=True,encoding='utf8',capture_output=True,timeout=20)
    app=base/'app';args=('--domain','notes.example.com','--email','admin@example.test')
    r=call(app,*args);assert r.returncode==0,(r.stdout,r.stderr)
    envfile=app/'notes-server-go/.env';before=envfile.read_text();password=[l.split('=',1)[1] for l in before.splitlines() if l.startswith('POSTGRES_PASSWORD=')][0]
    assert len(password)==64 and all(c in '0123456789abcdef' for c in password)
    assert 'test-invitation-secret' in r.stdout
    assert (app/'notes-server-go/.bootstrap-invite').exists()
    r=call(app,*args,TEST_STATE='pending');assert r.returncode==0,(r.stdout,r.stderr);assert envfile.read_text()==before
    r=call(app,*args,TEST_STATE='registered');assert r.returncode==0 and 'test-invitation-secret' not in r.stdout,(r.stdout,r.stderr)
    r=call(app,'--domain','other.example.com','--email','admin@example.test');assert r.returncode!=0;assert envfile.read_text()==before
    r=call(base/'invalid','--domain','bad;domain','--email','admin@example.test');assert r.returncode!=0 and not (base/'invalid').exists()
    r=call(base/'failed',*args,TEST_BUILD_FAIL='1');assert r.returncode!=0
    r=call(base/'tls-failed',*args,TEST_TLS_FAIL='1');assert r.returncode!=0
    r=call(base/'private',*args,GITHUB_TOKEN='test-ephemeral-private-token');assert r.returncode==0,(r.stdout,r.stderr)
    assert 'test-ephemeral-private-token' not in r.stdout+r.stderr+(base/'log').read_text()+(base/'private/notes-server-go/.env').read_text()
    assert '-c credential.helper= clone' in (base/'log').read_text()
    existing=base/'occupied';existing.mkdir();(existing/'keep.txt').write_text('keep');r=call(existing,*args);assert r.returncode!=0 and (existing/'keep.txt').read_text()=='keep'
    log=(base/'log').read_text();assert 'down' not in log and '--wait' in log
print('PASS: fresh install, secret generation, rerun preservation, pending/registered admin, domain mismatch, invalid input, build/TLS failures, nonrepo preservation')
