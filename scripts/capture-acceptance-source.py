"""Capture the actual tracked/untracked candidate source without ignored secrets."""
import argparse
import datetime
import hashlib
import json
import pathlib
import subprocess
import zipfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--repo', type=pathlib.Path, required=True)
parser.add_argument('--output', type=pathlib.Path, required=True)
args = parser.parse_args()
root, out = args.repo.resolve(), args.output.resolve()
out.mkdir(parents=True, exist_ok=True)

def git(*command):
    return subprocess.run(['git', '-C', str(root), *command], check=True, capture_output=True).stdout

paths = sorted(set(git('ls-files', '-z', '--cached', '--others', '--exclude-standard').decode('utf-8').split('\0')) - {''})
files, excluded = {}, []
with zipfile.ZipFile(out / 'source.zip', 'w', zipfile.ZIP_DEFLATED) as archive:
    for name in paths:
        path = root / name
        if path.is_symlink() or not path.is_file():
            continue
        if path.resolve().is_relative_to(out):
            continue
        if (path.name.startswith('.env') and path.name not in {'.env.example', '.env.sample'}) or path.suffix.lower() in {'.key', '.pem', '.pfx', '.p12'}:
            excluded.append(name)
            continue
        data = path.read_bytes()
        files[name] = hashlib.sha256(data).hexdigest()
        archive.writestr(name, data)
status = git('status', '--porcelain=v1', '-uall').decode('utf-8')
(out / 'status.txt').write_text(status, encoding='utf-8')
# Restrict patch paths to the same source allowlist; never capture excluded files.
patch = git('diff', '--binary', 'HEAD', '--', *files)
(out / 'diff.patch').write_bytes(patch)
manifest = {'captured_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'head': git('rev-parse', 'HEAD').decode().strip(),
            'dirty': bool(status), 'files': files, 'excluded': excluded,
            'source_zip_sha256': hashlib.sha256((out / 'source.zip').read_bytes()).hexdigest()}
(out / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding='utf-8')
print(json.dumps({'snapshot': str(out), 'head': manifest['head'], 'files': len(files), 'dirty': bool(status)}))
