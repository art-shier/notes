"""Run authorization/approval race on a fresh, disposable PostgreSQL container."""
import os,secrets,subprocess,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
name='notes-agent-pg-'+secrets.token_hex(6)
password=secrets.token_hex(24)
def docker(*args):
    return subprocess.run(['docker',*args],check=True,capture_output=True,text=True).stdout.strip()
try:
    docker('run','--rm','-d','--name',name,'-e','POSTGRES_PASSWORD='+password,'-e','POSTGRES_DB=notes_agent','-p','127.0.0.1::5432','postgres:16-alpine')
    for _ in range(60):
        try:docker('exec',name,'pg_isready','-U','postgres');break
        except subprocess.CalledProcessError:time.sleep(0.5)
    else:raise RuntimeError('Disposable PostgreSQL did not start.')
    port=docker('port',name,'5432/tcp').split(':')[-1]
    env={**os.environ,'NOTES_AUTH_TEST_DATABASE_URL':f'postgresql://postgres:{password}@127.0.0.1:{port}/notes_agent?sslmode=disable'}
    result=subprocess.run(['go','test','./internal/api','-run','TestAgentBrowserAuthorization','-count=1'],cwd=ROOT/'notes-server-go',env=env,capture_output=True,text=True)
    if result.returncode:raise RuntimeError('PostgreSQL auth test failed: '+(result.stdout+result.stderr).replace(password,'[REDACTED]'))
    print('PASS: PostgreSQL additive migration, scoped authorization, CSRF, expiry, cancel and cross-user approval race')
finally:
    # Only the random container created in this test is removed, including its anonymous volume.
    subprocess.run(['docker','rm','-f','-v',name],capture_output=True)
