"""Packaged pre-install executes without a checkout and preserves config safely."""
import os
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[1]
subprocess.run([sys.executable,str(ROOT/'tests/team_config_test.py')],
    env={**os.environ,'NOTES_TEST_HOOK':'1'},check=True)
subprocess.run([sys.executable,str(ROOT/'deploy/build-hooks.py'),'--check'],check=True)
print('PASS: self-contained pre-install and reproducible hook generation')
