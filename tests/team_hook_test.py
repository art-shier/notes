"""Packaged pre-install executes without a checkout and preserves config safely."""
import os
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[1]
subprocess.run([sys.executable,str(ROOT/'tests/team_config_test.py')],
    env={**os.environ,'NOTES_TEST_HOOK':'1'},check=True)
subprocess.run([sys.executable,str(ROOT/'deploy/build-hooks.py'),'--check'],check=True)
retired=subprocess.run(['bash',str(ROOT/'deploy/prepare.sh')],capture_output=True,text=True)
assert retired.returncode!=0 and 'ctl' in retired.stderr, 'Obsolete generator must explain the supported ctl path'
print('PASS: self-contained pre-install and reproducible hook generation')
