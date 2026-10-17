"""Check release admission before starting any process or reading credentials."""
import hashlib
import json
import os
import pathlib
import subprocess
import sys
import tempfile

script = pathlib.Path(__file__).with_name('verify-release.py')
with tempfile.TemporaryDirectory() as directory:
    root = pathlib.Path(directory)
    files = ['backend-go/edu-server.exe' if os.name == 'nt' else 'backend-go/edu-server', 'frontend-vue/dist/index.html',
             'render-service-py/app.py', 'render-service-py/requirements.txt', 'compose.yaml',
             'scripts/start.ps1', 'scripts/start-cloud.ps1', 'scripts/watch-cloud.ps1',
             'scripts/verify-release.py', 'scripts/render-courseware-preview.ps1',
             'animation-video/export.mjs', 'animation-video/package-lock.json']
    for name in files:
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text('explicit test fixture', encoding='utf-8')
    manifest = {'tag': 'test-v1', 'commit': 'a' * 40, 'platform': os.name,
                'files': {name: hashlib.sha256((root / name).read_bytes()).hexdigest() for name in files}}
    def verify(expected=0):
        (root / 'manifest.json').write_text(json.dumps(manifest), encoding='utf-8')
        result = subprocess.run([sys.executable, str(script), '--release', str(root),
                                 '--expect-tag', 'test-v1'], capture_output=True, text=True)
        assert (result.returncode == 0) == (expected == 0), result.stdout + result.stderr
    verify()
    (root / files[0]).write_text('tampered executable')
    verify(2)
    (root / files[0]).write_text('explicit test fixture')
    manifest['files']['../outside'] = '0' * 64
    verify(2)
    del manifest['files']['../outside']
    manifest['tag'] = 'wrong-tag'
    verify(2)
    manifest['tag'] = 'test-v1'
    del manifest['files']['animation-video/export.mjs']
    verify(2)
print('PASS: valid release, tampering, path escape, wrong tag and missing component')
