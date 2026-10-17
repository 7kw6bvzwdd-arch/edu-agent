"""Verify one complete tag-built release without starting services."""
import argparse
import hashlib
import json
import os
import pathlib
import re

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--release', required=True, type=pathlib.Path)
parser.add_argument('--expect-tag', required=True)
args = parser.parse_args()
root = args.release.resolve()
try:
    if (root / 'BUILD_FAILED').exists():
        raise ValueError('Incomplete build')
    manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8'))
    if manifest['tag'] != args.expect_tag or not re.fullmatch(r'[0-9a-f]{40}', manifest['commit']):
        raise ValueError('Release tag/commit mismatch')
    if manifest['platform'] != os.name:
        raise ValueError('Release was built for another operating system')
    files = manifest['files']
    binary = 'backend-go/edu-server.exe' if os.name == 'nt' else 'backend-go/edu-server'
    required = {binary, 'frontend-vue/dist/index.html', 'render-service-py/app.py',
                'render-service-py/requirements.txt', 'compose.yaml',
                'scripts/start.ps1', 'scripts/start-cloud.ps1', 'scripts/watch-cloud.ps1',
                'scripts/verify-release.py', 'scripts/render-courseware-preview.ps1',
                'animation-video/export.mjs', 'animation-video/package-lock.json'}
    if not required.issubset(files):
        raise ValueError('Release is missing a required component')
    for name, digest in files.items():
        path = (root / name).resolve()
        if not path.is_relative_to(root) or pathlib.PurePosixPath(name).is_absolute():
            raise ValueError('Artifact path escapes release directory')
        if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise ValueError('Artifact hash mismatch: ' + name)
except (ValueError, KeyError, TypeError, OSError) as error:
    parser.exit(2, 'Release verification failed: ' + str(error) + chr(10))
print(json.dumps({'status': 'verified', 'tag': manifest['tag'],
                  'commit': manifest['commit'], 'files': len(files)}))
