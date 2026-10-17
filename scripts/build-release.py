"""Build all deployable components from one exact local Git tag."""
import argparse
import hashlib
import io
import json
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--tag', required=True)
parser.add_argument('--repo', type=pathlib.Path, default=pathlib.Path(__file__).resolve().parents[1])
parser.add_argument('--output', type=pathlib.Path)
parser.add_argument('--inspect', action='store_true', help='Validate tag and report commit without building')
args = parser.parse_args()
repo = args.repo.resolve()
def git(*command):
    return subprocess.check_output(['git', '-C', str(repo), *command])
try:
    if args.tag not in git('tag', '--list').decode().splitlines():
        raise ValueError('An exact existing Git tag is required')
    commit = git('rev-parse', '--verify', 'refs/tags/' + args.tag + '^{commit}').decode().strip()
except (ValueError, subprocess.CalledProcessError) as error:
    parser.exit(2, str(error) + '\n')
metadata = {'tag': args.tag, 'commit': commit, 'platform': os.name}
if args.inspect:
    print(json.dumps(metadata))
    raise SystemExit(0)
output = (args.output or repo / 'work/releases' / args.tag.replace('/', '_')).resolve()
if output.exists():
    parser.exit(2, 'Output already exists; choose a new directory to preserve rollback artifacts\n')
go = os.environ.get('GO_EXE') or shutil.which('go')
local_go = repo / 'work/toolchain/go/bin/go.exe'
if not go and local_go.is_file():
    go = str(local_go)
if not go:
    parser.exit(2, 'Go executable required (GO_EXE or PATH)\n')
npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm')
if not npm:
    parser.exit(2, 'npm executable required\n')
with tempfile.TemporaryDirectory(prefix='edu-tag-') as directory:
    source = pathlib.Path(directory) / 'source'
    source.mkdir()
    with tarfile.open(fileobj=io.BytesIO(git('archive', '--format=tar', commit))) as archive:
        archive.extractall(source, filter='data')
    output.mkdir(parents=True)
    try:
        subprocess.run([npm, 'ci', '--no-audit', '--no-fund'], cwd=source / 'frontend-vue', check=True)
        subprocess.run([npm, 'run', 'build'], cwd=source / 'frontend-vue', check=True)
        binary = output / 'backend-go' / ('edu-server.exe' if os.name == 'nt' else 'edu-server')
        binary.parent.mkdir()
        subprocess.run([go, 'build', '-trimpath', '-buildvcs=false', '-ldflags',
                        '-X main.releaseTag=' + args.tag + ' -X main.releaseCommit=' + commit,
                        '-o', str(binary), '.'], cwd=source / 'backend-go', check=True)
        shutil.copytree(source / 'frontend-vue/dist', output / 'frontend-vue/dist')
        shutil.copytree(source / 'render-service-py', output / 'render-service-py')
        shutil.copytree(source / 'scripts', output / 'scripts')
        shutil.copytree(source / 'animation-video', output / 'animation-video')
        shutil.copy2(source / 'compose.yaml', output / 'compose.yaml')
        identity = json.loads(subprocess.check_output([str(binary), '--version'], text=True))
        if identity != {'release_tag': args.tag, 'release_commit': commit}:
            raise ValueError('Built binary does not report the archived tag/commit')
        metadata['files'] = {str(p.relative_to(output)).replace(os.sep, '/'): hashlib.sha256(p.read_bytes()).hexdigest()
                             for p in sorted(output.rglob('*')) if p.is_file()}
        metadata['tools'] = {'go': subprocess.check_output([go, 'version'], text=True).strip(),
                             'node': subprocess.check_output(['node', '--version'], text=True).strip()}
        (output / 'manifest.json').write_text(json.dumps(metadata, indent=2), encoding='utf-8')
        subprocess.run([os.sys.executable, str(output / 'scripts/verify-release.py'),
                        '--release', str(output), '--expect-tag', args.tag], check=True)
    except BaseException:
        # Keep partial artifacts for diagnosis; never label a failed build complete.
        (output / 'BUILD_FAILED').write_text('Build did not finish', encoding='utf-8')
        raise
print(json.dumps({'status': 'built', 'tag': args.tag, 'commit': commit, 'output': str(output)}))
