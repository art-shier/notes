"""Build deterministic public skill resources for embedding in every Notes runtime."""
import argparse, hashlib, io, json, sys, zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
FILES=('SKILL.md','scripts/notes.py','scripts/agent_auth.py','references/api.md')
DEST=ROOT/'notes-server-go/internal/agentassets/files'

def resources():
    files={name:(ROOT/'notes-skill/shiji-notes'/name).read_bytes().replace(b'\r\n',b'\n') for name in FILES}
    archive=io.BytesIO()
    with zipfile.ZipFile(archive,'w',compression=zipfile.ZIP_STORED) as z:
        for name,data in sorted(files.items()):
            entry=zipfile.ZipInfo('shiji-notes/'+name,date_time=(2020,1,1,0,0,0));entry.create_system=3;entry.compress_type=zipfile.ZIP_STORED;entry.external_attr=0o100644<<16
            z.writestr(entry,data)
    files['shiji-notes.zip']=archive.getvalue()
    files['install-client.py']=(ROOT/'install-client.py').read_bytes().replace(b'\r\n',b'\n')
    files['manifest.json']=(json.dumps({'version':json.loads((ROOT/'notes-web/package.json').read_text())['version'],'sha256':hashlib.sha256(archive.getvalue()).hexdigest(),'files':{name:hashlib.sha256(data).hexdigest() for name,data in sorted(files.items())}},ensure_ascii=False,indent=2)+'\n').encode()
    return files

def main():
    p=argparse.ArgumentParser();p.add_argument('--check',action='store_true');args=p.parse_args()
    files=resources()
    if args.check:
        actual={x.relative_to(DEST).as_posix() for x in DEST.rglob('*') if x.is_file()}
        if actual!=set(files) or any(not (DEST/name).is_file() or (DEST/name).read_bytes()!=data for name,data in files.items()):
            print('Public Agent resources are stale. Run python scripts/build_agent_assets.py.',file=sys.stderr);return 1
    else:
        for name,data in files.items():
            file=DEST/name;file.parent.mkdir(parents=True,exist_ok=True);file.write_bytes(data)
    print('Public Agent resources verified.' if args.check else 'Public Agent resources generated.')
    return 0
if __name__=='__main__':raise SystemExit(main())
