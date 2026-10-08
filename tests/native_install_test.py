"""Installer rejection paths in temporary folders, with OS privilege boundaries mocked."""
import hashlib, io, json, os, shutil, subprocess, tarfile, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
installer=ROOT/'install-native.sh'
assert installer.is_file(),'Native one-command installer is not implemented'
BASH=shutil.which('bash') if os.name!='nt' else 'C:/Program Files/Git/bin/bash.exe'
with tempfile.TemporaryDirectory(prefix='notes-native-installer-',dir='/root' if os.name!='nt' and os.geteuid()==0 else None) as temporary:
    base=Path(temporary);bin_dir=base/'bin';bin_dir.mkdir();install_dir=base/'install';cfg=base/'config';data=base/'data';units=base/'units'
    for name,body in {'uname':'case $1 in -s) echo "${TEST_OS:-Linux}";; -m) echo x86_64;; esac','id':'echo "${TEST_UID:-0}"','systemctl':'[[ $1 != cat ]]','runuser':'exit 0','getent':'exit 1','stat':'case "$*" in *%a*service.env*) echo 600;; *%a*) echo "${TEST_MODE:-755}";; *) echo "${TEST_OWNER:-0}";; esac'}.items():
        p=bin_dir/name;p.write_text('#!/usr/bin/env bash\n'+body+'\n');p.chmod(0o755)
    for name,body in {'curl':'cat "$TEST_RELEASES"','cp':'printf "%s\\n" "${@: -1}" >> "$TEST_COPY_LOG";exec /usr/bin/cp "$@"'}.items():
        p=bin_dir/name;p.write_text('#!/usr/bin/env bash\n'+body+'\n',encoding='utf-8',newline='\n');p.chmod(0o755)
    releases=base/'releases.json';copy_log=base/'copies'
    releases.write_text(json.dumps([
        {'tag_name':'v0.2.0','draft':False,'prerelease':False,'assets':[{'name':'notes-v0.2.0.tar.gz'}]},
        {'tag_name':'v0.1.0','draft':False,'prerelease':False,'assets':[{'name':'notes-server_0.1.0_linux_amd64.tar.gz'}]}]))
    jq=shutil.which('jq') if os.name!='nt' else os.environ['NOTES_TEST_JQ']
    shutil.copyfile(jq,bin_dir/('jq.exe' if os.name=='nt' else 'jq'));(bin_dir/('jq.exe' if os.name=='nt' else 'jq')).chmod(0o755)
    def path(p):return '/'+str(p).replace('\\','/').replace(':','').lower() if os.name=='nt' else str(p)
    archive=base/'notes-server_0.0.0-ci_linux_amd64.tar.gz'
    with tarfile.open(archive,'w:gz') as package:
        info=tarfile.TarInfo('../../escape');info.size=4;package.addfile(info,io.BytesIO(b'bad!'))
    checksum=base/'checksums.txt';checksum.write_text('0'*64+'  '+archive.name+'\n'+'0'*64+'  notes-server_0.1.0_linux_amd64.tar.gz\n')
    incoming=base/'service.env';incoming.write_text('DATABASE_URL=postgresql://notes_app:encoded@db.test/notes\n');incoming.chmod(0o600)
    arguments=['--domain','notes.example.com','--email','admin@example.com','--version','v0.0.0-ci','--artifact',path(archive),'--checksum-file',path(checksum),'--install-dir',path(install_dir),'--config-dir',path(cfg),'--data-dir',path(data),'--unit-dir',path(units),'--env-file',path(incoming),'--skip-dependencies']
    def run(*extra,latest=False,**overrides):
        env={**os.environ,'TEST_BIN':str(bin_dir).replace('\\','/'),'TEST_RELEASES':str(releases).replace('\\','/'),'TEST_COPY_LOG':str(copy_log).replace('\\','/'),**overrides}
        active=arguments.copy()
        if latest:
            index=active.index('--version');del active[index:index+2]
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$@"'
        return subprocess.run([BASH,'-c',script,'test',str(installer).replace('\\','/'),*active,*extra],env=env,capture_output=True,text=True,encoding='utf-8',errors='replace',timeout=30)
    assert run(TEST_OS='Darwin').returncode!=0
    assert run(TEST_UID='1000').returncode!=0
    result=run('--token-file','/absent/token');assert result.returncode!=0 and 'ConfigHub' in result.stderr
    assert run('--domain','invalid').returncode!=0
    assert run('--port','80').returncode!=0
    assert run('--install-dir','/').returncode!=0
    for override in [{'TEST_OWNER':'1000'},{'TEST_MODE':'777'}]:
        result=run(**override);assert result.returncode!=0 and '目录祖先' in result.stderr,result.stderr
    for extra in [('--config-dir',path(install_dir/'config')),('--unit-dir',path(data))]:
        result=run(*extra);assert result.returncode!=0 and '互不嵌套' in result.stderr,result.stderr
    result=run();assert result.returncode!=0 and 'SHA-256' in result.stderr and not (install_dir/'current').exists(),result.stderr
    result=run(latest=True);assert result.returncode!=0 and 'SHA-256' in result.stderr,result.stderr
    assert 'notes-server_0.1.0_linux_amd64.tar.gz' in copy_log.read_text(), 'Selected a Team package release for native installation'
    checksum.write_text(hashlib.sha256(archive.read_bytes()).hexdigest()+'  '+archive.name+'\n')
    result=run();assert result.returncode!=0 and '非法路径' in result.stderr and not (base.parent/'escape').exists() and not (install_dir/'current').exists(),result.stderr
    if not install_dir.exists():install_dir.mkdir()
    (install_dir/'unrelated').write_text('retain me')
    assert run().returncode!=0 and (install_dir/'unrelated').read_text()=='retain me'
print('PASS: native installer OS/root/domain/port/path guards, bad SHA, unsafe archive and unrelated directory preservation')
