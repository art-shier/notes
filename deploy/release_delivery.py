"""Recover fixed ctl artifacts and complete only missing GitHub release assets."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def gh(argv):
    result=subprocess.run(['gh',*map(str,argv)],capture_output=True,text=True,timeout=120)
    # Child diagnostics can contain authenticated links; never echo them.
    return result.returncode,result.stdout


def publish_github(repository, version, package):
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+',repository):raise ValueError('invalid repository')
    from deployctl.contract import version as validate_version
    validate_version(version)
    package=Path(package);sidecar=package.with_name(package.name+'.sha256')
    expected={package.name:package,sidecar.name:sidecar}
    if any(not path.is_file() for path in expected.values()):raise ValueError('release artifacts missing')
    code,raw=gh(['release','view',version,'--repo',repository,'--json','tagName,assets'])
    if code:
        code,_=gh(['release','create',version,str(package),str(sidecar),'--repo',repository,'--verify-tag','--title',version,'--generate-notes'])
        if code:raise RuntimeError('GitHub publication incomplete; retry to recover the registered ctl version')
        return
    current=json.loads(raw)
    if current.get('tagName')!=version or not isinstance(current.get('assets'),list):raise ValueError('unexpected existing GitHub release')
    names=[asset['name'] for asset in current['assets']]
    if len(names)!=len(set(names)):raise ValueError('duplicate GitHub artifact names')
    with tempfile.TemporaryDirectory(prefix='notes-release-verify-') as temp:
        for name,path in expected.items():
            if name not in names:continue
            code,_=gh(['release','download',version,'--repo',repository,'--pattern',name,'--dir',temp])
            if code:raise RuntimeError('cannot verify existing GitHub artifact')
            downloaded=Path(temp)/name
            if not downloaded.is_file() or downloaded.read_bytes()!=path.read_bytes():raise ValueError('GitHub version already contains different immutable content')
        for name,path in expected.items():
            if name in names:continue
            code,_=gh(['release','upload',version,'--repo',repository,str(path)])
            if code:raise RuntimeError('GitHub asset upload incomplete; retry with the registered ctl package')


def recover_managed(application,version,commit,output):
    if not os.environ.get('CTL_SERVER_URL'):return None
    from deployctl.platform_client import PlatformClient
    from deployctl.platform_credentials import Credentials
    client_file=Path(os.environ['RUNNER_TEMP'])/'ctl-publisher/client.json'
    credentials=Credentials.save(client_file,os.environ['CTL_SERVER_URL'],os.environ['CTL_PUBLISH_TOKEN'])
    return PlatformClient(credentials).recover_release(application,version,output,commit)


if __name__=='__main__':
    parser=argparse.ArgumentParser();commands=parser.add_subparsers(dest='action',required=True)
    recover=commands.add_parser('recover');recover.add_argument('--application',required=True);recover.add_argument('--version',required=True);recover.add_argument('--commit',required=True);recover.add_argument('--output',default='dist')
    publish=commands.add_parser('github');publish.add_argument('--repository',required=True);publish.add_argument('--version',required=True);publish.add_argument('--package',required=True)
    args=parser.parse_args()
    try:
        if args.action=='github':publish_github(args.repository,args.version,args.package)
        else:
            recovered=recover_managed(args.application,args.version,args.commit,args.output)
            with open(os.environ['GITHUB_OUTPUT'],'a',encoding='utf-8') as outputs:
                print('recovered='+('true' if recovered else 'false'),file=outputs)
                if recovered:print('image='+recovered['image'],file=outputs)
    except (ValueError,OSError,RuntimeError):
        raise SystemExit('Release delivery failed; immutable content was preserved. Check configuration/permissions and retry.')
