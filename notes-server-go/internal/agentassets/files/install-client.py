#!/usr/bin/env python3
"""Install the Shiji REST CLI and Skill; Python 3.10+, standard library only."""
import argparse
import ast
import io
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import stat
import sys
import tempfile
from urllib.parse import quote
from urllib.parse import urlsplit
from urllib.request import Request, urlopen, build_opener, HTTPRedirectHandler
import zipfile

FILES=('SKILL.md','scripts/notes.py','scripts/agent_auth.py','references/api.md')
MARKER='.shiji-install.json'
PREFIX='# Installed by art-shier/notes client installer'

def digest(data): return hashlib.sha256(data).hexdigest()

def is_link(entry):
    if entry.is_symlink(): return True
    try: attributes=getattr(entry.lstat(),'st_file_attributes',0)
    except FileNotFoundError: return False
    return bool(attributes & getattr(stat,'FILE_ATTRIBUTE_REPARSE_POINT',0))

def plain_path(raw):
    path=Path(os.path.abspath(Path(raw).expanduser()))
    for entry in (path,*path.parents):
        if is_link(entry):
            raise ValueError('Installation paths must not use symlinks or junctions.')
    return path

def fetch(url):
    with urlopen(Request(url,headers={'User-Agent':'shiji-notes-installer','Accept':'application/vnd.github+json'}),timeout=30) as response:
        data=response.read(1024*1024+1)
    if len(data)>1024*1024: raise ValueError('Download exceeded the installation size limit.')
    return data

def bundle(args):
    if getattr(args,'server',None):
        if args.source_dir:raise ValueError('Choose --server or --source-dir.')
        server=args.server.rstrip('/');p=urlsplit(server)
        local=p.scheme=='http' and p.hostname in {'localhost','127.0.0.1','::1'} and os.environ.get('NOTES_ALLOW_LOCAL_HTTP')=='1'
        if (p.scheme!='https' and not local) or not p.hostname or p.username is not None or p.path or p.query or p.fragment or any(c.isspace() for c in server):
            raise ValueError('Use the final HTTPS Notes app origin.')
        class NoRedirect(HTTPRedirectHandler):
            def redirect_request(self,*args,**kwargs):return None
        opener=build_opener(NoRedirect())
        def download(path):
            with opener.open(Request(server+path,headers={'User-Agent':'shiji-notes-installer'}),timeout=30) as response:data=response.read(1024*1024+1)
            if len(data)>1024*1024:raise ValueError('Skill download exceeds size limit.')
            return data
        metadata=json.loads(download('/api/v1/agent-access'))
        if not isinstance(metadata,dict):raise ValueError('Invalid Skill metadata.')
        archive=download('/agent/shiji-notes.zip')
        if digest(archive)!=metadata.get('sha256'):raise ValueError('Skill archive checksum mismatch.')
        with zipfile.ZipFile(io.BytesIO(archive)) as z:
            expected={'shiji-notes/'+name for name in FILES}
            if set(z.namelist())!=expected or len(z.infolist())!=len(expected):raise ValueError('Unexpected Skill archive contents.')
            if any(info.file_size>1024*1024 or stat.S_ISLNK(info.external_attr>>16) for info in z.infolist()):raise ValueError('Invalid Skill archive entry.')
            return {name:z.read('shiji-notes/'+name) for name in FILES},'server:'+str(metadata.get('version','unknown'))
    if args.source_dir:
        source=plain_path(args.source_dir)/'notes-skill/shiji-notes'
        return {name:(source/name).read_bytes() for name in FILES},'local'
    if not re.fullmatch(r'[A-Za-z0-9_.\-/]{1,128}',args.ref): raise ValueError('Invalid repository ref.')
    revision=json.loads(fetch('https://api.github.com/repos/art-shier/notes/commits/'+quote(args.ref,safe='')))['sha']
    if not re.fullmatch(r'[0-9a-f]{40}',revision): raise ValueError('Invalid repository revision.')
    root='https://raw.githubusercontent.com/art-shier/notes/'+revision+'/notes-skill/shiji-notes/'
    return {name:fetch(root+name) for name in FILES},revision

def launcher_bytes(target):
    client=target/'scripts/notes.py'
    if os.name=='nt':
        python=str(Path(sys.executable)).replace('%','%%');script=str(client).replace('%','%%')
        return ('@echo off\r\nchcp 65001 >nul\r\nrem '+PREFIX+'\r\n"'+python+'" "'+script+'" %*\r\n').encode('utf-8')
    return ('#!/usr/bin/env sh\n'+PREFIX+'\nexec '+shlex.quote(sys.executable)+' '+shlex.quote(str(client))+' "$@"\n').encode('utf-8')

def check_existing(target,launcher):
    if not target.exists():
        if launcher.exists(): raise ValueError('An existing command is not managed by this installer; choose another bin directory.')
        return
    if not target.is_dir() or not (target/MARKER).is_file():
        raise ValueError('Existing Skill is not managed by this installer; choose another skills directory.')
    saved=json.loads((target/MARKER).read_text(encoding='utf-8'))
    previous=set(saved.get('files',{}))
    if not {'SKILL.md','scripts/notes.py','references/api.md'}<=previous or not previous<=set(FILES):
        raise ValueError('Unrecognized managed Skill; no files changed.')
    expected={*previous,MARKER,'scripts','references'}
    entries=list(target.rglob('*'))
    if any(is_link(entry) for entry in entries):
        raise ValueError('Existing Skill contains symlinks or junctions; no files changed.')
    if {entry.relative_to(target).as_posix() for entry in entries}!=expected:
        raise ValueError('Existing Skill contains custom files; no files changed.')
    if any(digest((target/name).read_bytes())!=saved.get('files',{}).get(name) for name in previous):
        raise ValueError('Existing Skill has local changes; no files changed.')
    if launcher.exists() and (not launcher.is_file() or digest(launcher.read_bytes())!=saved.get('launcher')):
        raise ValueError('Existing command has local changes; no files changed.')

def install(args):
    target=plain_path(args.skills_dir)/'shiji-notes'
    plain_path(target)
    bin_dir=plain_path(args.bin_dir)
    if bin_dir==target or target in bin_dir.parents:
        raise ValueError('The command directory must be outside the installed Skill directory.')
    launcher=bin_dir/('shiji-notes.cmd' if os.name=='nt' else 'shiji-notes')
    plain_path(launcher)
    contents,revision=bundle(args)  # Finish all downloads before changing installed files.
    if b'name: shiji-notes' not in contents['SKILL.md']: raise ValueError('Invalid Skill package.')
    for name in FILES:
        if name.endswith('.py'):ast.parse(contents[name].decode('utf-8'))
    for data in contents.values(): data.decode('utf-8')
    command=launcher_bytes(target)
    marker=json.dumps({'revision':revision,'files':{name:digest(data) for name,data in contents.items()},'launcher':digest(command)},indent=2).encode('utf-8')
    target.parent.mkdir(parents=True,exist_ok=True)
    lock=target.parent/'.shiji-notes-install.lock'
    try: lock.mkdir()
    except FileExistsError: raise ValueError('Another client installation is running; no files changed.')
    try:
        check_existing(target,launcher)
        bin_dir.mkdir(parents=True,exist_ok=True)
        # Both staging directories are created within explicitly checked install parents.
        with tempfile.TemporaryDirectory(prefix='.shiji-stage-',dir=target.parent) as temporary:
            stage=Path(temporary);new=stage/'new';old=stage/'old'
            for name,data in {**contents,MARKER:marker}.items():
                destination=new/name;destination.parent.mkdir(parents=True,exist_ok=True);destination.write_bytes(data)
            with tempfile.NamedTemporaryFile(prefix='.shiji-command-',dir=bin_dir,delete=False) as handle:
                staged_command=Path(handle.name);handle.write(command)
            command_backup=None
            try:
                staged_command.chmod(0o755)
                existed=target.exists()
                previous_command=launcher.read_bytes() if launcher.exists() else None
                if previous_command is not None:
                    with tempfile.NamedTemporaryFile(prefix='.shiji-backup-',dir=bin_dir,delete=False) as handle:
                        command_backup=Path(handle.name);handle.write(previous_command)
                    command_backup.chmod(launcher.stat().st_mode&0o777)
                try:
                    if existed: target.rename(old)
                    new.rename(target)
                    os.replace(staged_command,launcher)
                except BaseException:
                    # Restore on Ctrl-C as well as ordinary publication failures.
                    if old.exists():
                        if target.exists(): target.rename(new)
                        old.rename(target)
                    elif not existed and target.exists(): target.rename(new)
                    if previous_command is None:
                        launcher.unlink(missing_ok=True)
                    elif not launcher.exists() or launcher.read_bytes()!=previous_command:
                        os.replace(command_backup,launcher)
                    raise
            finally:
                staged_command.unlink(missing_ok=True)
                if command_backup is not None: command_backup.unlink(missing_ok=True)
    finally:
        lock.rmdir()
    print('Installed Shiji CLI:',launcher)
    print('Installed Agent Skill:',target)
    print('Source revision:',revision)
    print('Add the command directory to PATH if needed:',bin_dir)
    print('Connect: shiji-notes login'+(' --server '+args.server if getattr(args,'server',None) else ''))
    print('Notes opens for the user to approve. Credentials stay in ~/.shiji-notes/client.json; do not paste them into chat.')
    print('Verify with: shiji-notes whoami')

def main():
    if sys.version_info<(3,10): raise SystemExit('Python 3.10 or newer is required.')
    parser=argparse.ArgumentParser(description='Install Shiji CLI + Agent Skill without Docker, Git or third-party Python packages.')
    parser.add_argument('--skills-dir',default=str(Path.home()/'.agents/skills'))
    parser.add_argument('--bin-dir',default=str(Path.home()/'.local/bin'))
    parser.add_argument('--ref',default='main',help='Repository branch, tag or commit')
    parser.add_argument('--source-dir',help='Install from a local notes repository instead of downloading')
    parser.add_argument('--server',help='Install the checksum-verified Skill served by this Notes app origin')
    try: install(parser.parse_args())
    except (OSError,ValueError,KeyError,SyntaxError,zipfile.BadZipFile):
        print('Installation failed: download, package, path or existing-file validation did not succeed. Existing local customizations are preserved.',file=sys.stderr)
        return 1
    return 0

if __name__=='__main__':
    for stream in (sys.stdout,sys.stderr):
        if hasattr(stream,'reconfigure'): stream.reconfigure(encoding='utf-8')
    raise SystemExit(main())
