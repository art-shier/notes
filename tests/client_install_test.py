"""Install real CLI/Skill in disposable directories; never changes user skills/PATH."""
import hashlib, importlib.util, os, subprocess, sys, tempfile
from pathlib import Path
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
installer=ROOT/'install-client.py'
assert installer.is_file(), 'One-command client installer is not implemented'
with tempfile.TemporaryDirectory(prefix='notes-client-test-') as temporary:
    base=Path(temporary); skills=base/'agent skills'; bin_dir=base/'command bin'
    args=['--source-dir',str(ROOT),'--skills-dir',str(skills),'--bin-dir',str(bin_dir)]
    def install(*extra):
        return subprocess.run([sys.executable,str(installer),*args,*extra],capture_output=True,text=True,encoding='utf-8',errors='replace')
    result=install();assert result.returncode==0,(result.stdout,result.stderr)
    target=skills/'shiji-notes'; launcher=bin_dir/('shiji-notes.cmd' if os.name=='nt' else 'shiji-notes')
    for name in ('SKILL.md','scripts/notes.py','scripts/agent_auth.py','references/api.md'):
        assert (target/name).read_bytes()==(ROOT/'notes-skill/shiji-notes'/name).read_bytes()
    command=[str(launcher),'--help']
    if os.name=='nt': command=['cmd.exe','/d','/c',str(launcher),'--help']
    help_result=subprocess.run(command,capture_output=True,text=True,encoding='utf-8')
    assert help_result.returncode==0 and 'search' in help_result.stdout,(help_result.stdout,help_result.stderr)
    assert install().returncode==0
    assert install('--bin-dir',str(target/'scripts')).returncode!=0
    spec=importlib.util.spec_from_file_location('client_installer',installer)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    original_command=launcher.read_bytes();original_skill=(target/'SKILL.md').read_bytes()
    options=type('Options',(),dict(source_dir=str(ROOT),skills_dir=str(skills),bin_dir=str(bin_dir),ref='main'))()
    with patch.object(module.os,'replace',side_effect=OSError('fixture publication failure')):
        try:module.install(options)
        except OSError:pass
        else:raise AssertionError('Command publication failure was ignored')
    assert launcher.read_bytes()==original_command and (target/'SKILL.md').read_bytes()==original_skill
    rename=module.Path.rename
    def interrupted(path,destination):
        if path.name=='new' and path.parent.name.startswith('.shiji-stage-'): raise KeyboardInterrupt()
        return rename(path,destination)
    with patch.object(module.Path,'rename',interrupted):
        try:module.install(options)
        except KeyboardInterrupt:pass
        else:raise AssertionError('Fixture interruption was ignored')
    assert target.is_dir(),'Interrupted update must preserve the previously installed Skill'
    assert launcher.read_bytes()==original_command and (target/'SKILL.md').read_bytes()==original_skill
    replace=module.os.replace;original_mode=launcher.stat().st_mode&0o777
    def late_interruption(source,destination):
        if Path(source).parent!=Path(destination).parent:raise OSError('Fixture cross-filesystem rename')
        replace(source,destination)
        if Path(source).name.startswith('.shiji-command-'):raise KeyboardInterrupt()
    with patch.object(module,'launcher_bytes',return_value=original_command+b'\n'),patch.object(module.os,'replace',late_interruption):
        try:module.install(options)
        except KeyboardInterrupt:pass
        else:raise AssertionError('Late publication interruption was ignored')
    assert launcher.read_bytes()==original_command and (target/'SKILL.md').read_bytes()==original_skill
    if os.name!='nt':assert launcher.stat().st_mode&0o777==original_mode,'Rollback must preserve command execute permissions'
    pinned='a'*40;urls=[]
    def download(url):
        urls.append(url)
        if '/commits/' in url:return ('{"sha":"'+pinned+'"}').encode()
        return b'fixture'
    options.source_dir=None
    with patch.object(module,'fetch',side_effect=download):
        _,revision=module.bundle(options)
    assert revision==pinned and len(urls)==1+len(module.FILES) and all('/'+pinned+'/' in url for url in urls[1:])
    if os.name=='nt':
        junction=base/'junction'
        created=subprocess.run(['cmd.exe','/d','/c','mklink','/J',str(junction),str(skills)],capture_output=True)
        assert created.returncode==0
        assert install('--skills-dir',str(junction)).returncode!=0
        junction.rmdir()
        unicode_skills=base/'中文技能'
        result=install('--skills-dir',str(unicode_skills),'--bin-dir',str(base/'unicode-bin'))
        assert result.returncode==0,(result.stdout,result.stderr)
        unicode_launcher=base/'unicode-bin/shiji-notes.cmd'
        result=subprocess.run(['cmd.exe','/d','/q'],input='chcp 936 >nul\ncall "'+str(unicode_launcher)+'" --help\nexit /b %errorlevel%\n',capture_output=True,text=True,encoding='utf-8',errors='replace')
        assert result.returncode==0 and 'search' in result.stdout,'CLI fails with Unicode skill path under Windows codepage 936: '+result.stderr
        powershell=Path(os.environ['SystemRoot'])/'System32/WindowsPowerShell/v1.0/powershell.exe'
        probes=base/'python-probes';probes.mkdir()
        (probes/'python.cmd').write_text('@echo off\necho unavailable 1>&2\nexit /b 1\n')
        (probes/'python3.cmd').write_text('@echo off\n"'+sys.executable+'" %*\n')
        result=subprocess.run([str(powershell),'-NoProfile','-ExecutionPolicy','Bypass','-File',str(ROOT/'install-client.ps1'),'-SourceDir',str(ROOT),'-SkillsDir',str(base/'ps5-skills'),'-BinDir',str(base/'ps5-bin'),'-NoPathUpdate'],env={**os.environ,'PATH':str(probes)+os.pathsep+os.environ['PATH']},capture_output=True,text=True,errors='replace')
        assert result.returncode==0,'PowerShell 5.1 must skip failed Python candidates: '+result.stderr
    original=(target/'SKILL.md').read_bytes()
    (target/'SKILL.md').write_bytes(original+b'\nCustom local change\n')
    assert install().returncode!=0 and (target/'SKILL.md').read_bytes().endswith(b'Custom local change\n')
    (target/'SKILL.md').write_bytes(original)
    before=hashlib.sha256((target/'scripts/notes.py').read_bytes()).hexdigest()
    assert install('--source-dir',str(base/'missing-source')).returncode!=0
    assert hashlib.sha256((target/'scripts/notes.py').read_bytes()).hexdigest()==before
    launcher.write_text('unrelated command',encoding='utf-8')
    assert install().returncode!=0 and launcher.read_text()=='unrelated command'
    launcher.unlink()
    marker=target/'.shiji-install.json'; marker.unlink()
    assert install().returncode!=0 and not marker.exists()
    if os.name!='nt':
        link=base/'linked skills';link.symlink_to(skills,target_is_directory=True)
        result=install('--skills-dir',str(link));assert result.returncode!=0
print('PASS: client/Skill install, CLI help with spaced paths, safe rerun, custom-file/unmanaged-command preservation, failed source and symlink protection')
