"""Installer rejection paths in temporary folders, with OS privilege boundaries mocked."""
import hashlib, io, os, shutil, subprocess, tarfile, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
installer=ROOT/'install-native.sh'
assert installer.is_file(),'Native one-command installer is not implemented'
BASH=shutil.which('bash') if os.name!='nt' else 'C:/Program Files/Git/bin/bash.exe'
with tempfile.TemporaryDirectory(prefix='notes-native-installer-',dir=ROOT.parent) as temporary:
    base=Path(temporary);bin_dir=base/'bin';bin_dir.mkdir();install_dir=base/'install';cfg=base/'config';data=base/'data';units=base/'units'
    for name,body in {'uname':'case $1 in -s) echo "${TEST_OS:-Linux}";; -m) echo x86_64;; esac','id':'echo "${TEST_UID:-0}"','systemctl':'[[ $1 != cat ]]','runuser':'exit 0','getent':'exit 1','stat':'case "$*" in *%a*) echo "${TEST_MODE:-755}";; *) echo "${TEST_OWNER:-0}";; esac'}.items():
        p=bin_dir/name;p.write_text('#!/usr/bin/env bash\n'+body+'\n');p.chmod(0o755)
    jq=shutil.which('jq') if os.name!='nt' else os.environ['NOTES_TEST_JQ']
    shutil.copyfile(jq,bin_dir/('jq.exe' if os.name=='nt' else 'jq'));(bin_dir/('jq.exe' if os.name=='nt' else 'jq')).chmod(0o755)
    def path(p):return '/'+str(p).replace('\\','/').replace(':','').lower() if os.name=='nt' else str(p)
    archive=base/'notes-server_0.0.0-ci_linux_amd64.tar.gz'
    with tarfile.open(archive,'w:gz') as package:
        info=tarfile.TarInfo('../../escape');info.size=4;package.addfile(info,io.BytesIO(b'bad!'))
    checksum=base/'checksums.txt';checksum.write_text('0'*64+'  '+archive.name+'\n')
    arguments=['--domain','notes.example.com','--email','admin@example.com','--version','v0.0.0-ci','--artifact',path(archive),'--checksum-file',path(checksum),'--install-dir',path(install_dir),'--config-dir',path(cfg),'--data-dir',path(data),'--unit-dir',path(units),'--skip-dependencies']
    def run(*extra,**overrides):
        env={**os.environ,'TEST_BIN':str(bin_dir).replace('\\','/'),**overrides}
        script=('export PATH="$(cygpath -u "$TEST_BIN"):$PATH"; ' if os.name=='nt' else 'export PATH="$TEST_BIN:$PATH"; ')+'exec bash "$@"'
        return subprocess.run([BASH,'-c',script,'test',str(installer).replace('\\','/'),*arguments,*extra],env=env,capture_output=True,text=True,encoding='utf-8',errors='replace',timeout=30)
    assert run(TEST_OS='Darwin').returncode!=0
    assert run(TEST_UID='1000').returncode!=0
    assert run('--domain','invalid').returncode!=0
    assert run('--port','80').returncode!=0
    assert run('--install-dir','/').returncode!=0
    for override in [{'TEST_OWNER':'1000'},{'TEST_MODE':'777'}]:
        result=run(**override);assert result.returncode!=0 and '目录祖先' in result.stderr,result.stderr
    for extra in [('--config-dir',path(install_dir/'config')),('--unit-dir',path(data))]:
        result=run(*extra);assert result.returncode!=0 and '互不嵌套' in result.stderr,result.stderr
    result=run();assert result.returncode!=0 and 'SHA-256' in result.stderr and not (install_dir/'current').exists(),result.stderr
    checksum.write_text(hashlib.sha256(archive.read_bytes()).hexdigest()+'  '+archive.name+'\n')
    result=run();assert result.returncode!=0 and '非法路径' in result.stderr and not (base.parent/'escape').exists() and not (install_dir/'current').exists(),result.stderr
    if not install_dir.exists():install_dir.mkdir()
    (install_dir/'unrelated').write_text('retain me')
    assert run().returncode!=0 and (install_dir/'unrelated').read_text()=='retain me'
print('PASS: native installer OS/root/domain/port/path guards, bad SHA, unsafe archive and unrelated directory preservation')
