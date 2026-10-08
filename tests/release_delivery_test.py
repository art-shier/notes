"""A failed GitHub publication resumes the exact ctl package, without a rebuild."""
import importlib.util
import json
from pathlib import Path
import tempfile
import sys
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(Path(sys.argv[1]).resolve()))
spec=importlib.util.spec_from_file_location('release_delivery',ROOT/'deploy/release_delivery.py')
delivery=importlib.util.module_from_spec(spec);spec.loader.exec_module(delivery)

with tempfile.TemporaryDirectory() as temporary:
    folder=Path(temporary);package=folder/'notes-v1.0.0.tar.gz';package.write_bytes(b'immutable fixture')
    sidecar=package.with_name(package.name+'.sha256');sidecar.write_text('fixture checksum')
    remote={};uploads=[]
    def gh(argv):
        if argv[:2]==['release','view']:
            if not remote:return 1,''
            return 0,json.dumps({'tagName':'v1.0.0','assets':[{'name':name} for name in remote]})
        if argv[:2]==['release','create']:
            remote[package.name]=package.read_bytes()
            return 1,''  # Release exists but upload/response failed after the first artifact.
        if argv[:2]==['release','download']:
            name=argv[argv.index('--pattern')+1];dest=Path(argv[argv.index('--dir')+1]);(dest/name).write_bytes(remote[name]);return 0,''
        if argv[:2]==['release','upload']:
            filename=Path(argv[-1]);uploads.append(filename.name);remote[filename.name]=filename.read_bytes();return 0,''
        raise AssertionError(argv)
    with patch.object(delivery,'gh',side_effect=gh):
        try:delivery.publish_github('art-shier/notes','v1.0.0',package)
        except RuntimeError:pass
        else:raise AssertionError('partial publication was reported successful')
        delivery.publish_github('art-shier/notes','v1.0.0',package)
        assert uploads==[sidecar.name], 'existing immutable artifact must be reused, only missing asset uploaded'
        remote[package.name]=b'conflicting artifact'
        try:delivery.publish_github('art-shier/notes','v1.0.0',package)
        except ValueError:pass
        else:raise AssertionError('different published package was overwritten')
print('PASS: partial GitHub publication resumes exact registered bytes and rejects conflicts')
