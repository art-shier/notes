"""Run the packaged pre hook in isolation; ctl owns the effective configuration."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix='notes-managed-config-', dir='/root' if os.geteuid() == 0 else None) as temporary:
    base = Path(temporary)
    bins = base/'bin'; bins.mkdir()
    snapshot = base/'snapshot'; snapshot.mkdir(mode=0o700)
    config = base/'config/notes/prod'; config.mkdir(mode=0o700, parents=True)
    (config/'secrets.env').write_text('retain-old-configuration\n')
    (config/'secrets.env').chmod(0o600)
    effective = snapshot/'effective.env'
    context = snapshot/'.env.json'; context.write_text('{}'); context.chmod(0o600)
    log = base/'docker.log'
    for name, body in {
        'confighub': 'echo unexpected-config-fetch >&2; exit 99\n',
        'docker': """printf '%s\\n' "$@" > "$TEST_LOG"
[[ $1 == run && "$*" == *"--pull never"* && "$*" == *"--entrypoint shiji"* && ${@: -1} == database-check ]] || exit 2
while (($#));do
  if [[ $1 == --env-file ]];then cmp -s "$2" "$TEST_EFFECTIVE" || exit 3;shift 2;else shift;fi
done
if [[ ${TEST_DB_FAIL:-0} == 1 ]];then echo 'private-password diagnostic' >&2;exit 8;fi
""",
    }.items():
        path = bins/name; path.write_text('#!/usr/bin/env bash\n'+body); path.chmod(0o755)
    hook = base/'pre-install.sh'; shutil.copyfile(ROOT/'deploy/hooks/pre-install.sh', hook)
    image = 'ctl.example.test/notes@sha256:'+'a'*64
    env = {**os.environ, 'PATH': str(bins)+os.pathsep+os.environ['PATH'],
        'DEPLOYCTL_APPLICATION': 'notes', 'DEPLOYCTL_ENVIRONMENT': 'prod',
        'DEPLOYCTL_CONFIG_DIR': str(config), 'DEPLOYCTL_IMAGE': image,
        'DEPLOYCTL_ENV_FILE': str(context), 'TEST_LOG': str(log), 'TEST_EFFECTIVE': str(effective),
        'DATABASE_URL': '', 'DEPLOYCTL_PARAM_TOKEN_FILE': '/absent/token',
        'DEPLOYCTL_PARAM_CLI_BINARY': '/absent/confighub'}
    def run(**overrides):
        return subprocess.run(['bash', str(hook)], env={**env, **overrides}, capture_output=True, text=True, timeout=30)
    for raw in ['DATABASE_URL=postgresql://notes_app:encoded-secret@db.test/notes\n',
                'DB_HOST=db.test\nDB_PORT=5432\nDB_USER=notes_app\nDB_PASSWORD=private-password\nDB_NAME=notes\nDB_SSLMODE=require\n']:
        effective.write_text(raw); effective.chmod(0o600)
        result = run()
        assert result.returncode == 0, (result.returncode, result.stdout, result.stderr)
        assert image in log.read_text() and str(effective) in log.read_text()
        assert 'private-password' not in result.stdout+result.stderr
        assert (config/'secrets.env').read_text() == 'retain-old-configuration\n'
    result = run(TEST_DB_FAIL='1')
    assert result.returncode != 0 and 'private-password' not in result.stdout+result.stderr
    assert run(DEPLOYCTL_IMAGE='ctl.example.test/notes:latest').returncode != 0
    assert run(DEPLOYCTL_APPLICATION='other').returncode != 0
    effective.chmod(0o644)
    assert run().returncode != 0, 'Public effective configuration must be rejected'
    effective.chmod(0o600)
    effective.unlink(); effective.symlink_to(config/'secrets.env')
    assert run().returncode != 0, 'Linked effective configuration must be rejected'
print('PASS: isolated pre uses ctl snapshot, fixed pulled image, private files, no ConfigHub/config rewrites and masked DB failure')
