"""Round-trip the real standard packager; fixture digest is never published."""
import sys
import tempfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(Path(sys.argv[1]).resolve()))
from deployctl.contract import read_yaml
from deployctl.release import build_release, unpack_release

with tempfile.TemporaryDirectory() as temporary:
    base=Path(temporary)
    archive=build_release(read_yaml(ROOT/'deploy/deployment.yaml'),
        'ghcr.io/example/notes-fixture@sha256:'+'0'*64,'v0.0.0-test',base/'packages',project_root=ROOT)
    target=base/'unpacked';release=unpack_release(archive,target)
    assert release['schema_version']==2 and release['minimum_deployctl_version']=='1.6.0'
    assert release['deployment']['required_config']==[]
    assert release['hooks']['pre_install']['refresh_config'] is False
    assert 'DATABASE_URL' not in (target/'.env.example').read_text()
    for phase in ('pre','post'):
        assert (target/f'hooks/{phase}-install.sh').read_bytes()==(ROOT/f'deploy/hooks/{phase}-install.sh').read_bytes()
    assert not (target/'notes-server-go').exists()
print('PASS: v2 package round-trip, no database prerequisite, immutable hooks and ctl>=1.6 requirement')
