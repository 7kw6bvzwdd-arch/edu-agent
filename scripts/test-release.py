"""Exercise the release CLI against a disposable Git repository."""
import pathlib
import subprocess
import sys
import tempfile

script = pathlib.Path(__file__).with_name('build-release.py')
with tempfile.TemporaryDirectory() as directory:
    root = pathlib.Path(directory)
    def git(*args):
        return subprocess.run(['git', '-C', str(root), *args], check=True, capture_output=True, text=True)
    git('init')
    git('config', 'user.name', 'Release test')
    git('config', 'user.email', 'release-test@example.invalid')
    (root / 'version.txt').write_text('published baseline', encoding='utf-8')
    git('add', '.')
    git('commit', '-m', 'baseline')
    git('tag', 'release-test')
    (root / 'version.txt').write_text('unpublished change', encoding='utf-8')
    base = [sys.executable, str(script), '--repo', str(root)]
    result = subprocess.run(base + ['--tag', 'HEAD', '--inspect'], capture_output=True, text=True)
    assert result.returncode != 0, 'A branch or commit must not masquerade as a release tag'
    result = subprocess.run(base + ['--tag', 'release-test', '--inspect'], capture_output=True, text=True)
    assert result.returncode == 0, result.stderr
    assert git('rev-parse', 'HEAD').stdout.strip() in result.stdout
    assert 'published baseline' not in result.stdout, 'Inspection must print metadata only'
    commit = git('rev-parse', 'HEAD').stdout.strip()
    result = subprocess.run(base + ['--commit', commit, '--inspect'], capture_output=True, text=True)
    assert result.returncode == 0, result.stderr
    assert commit in result.stdout, 'Commit releases must preserve the exact source identity'
    result = subprocess.run(base + ['--commit', 'HEAD', '--inspect'], capture_output=True, text=True)
    assert result.returncode != 0, 'Commit releases require a full immutable hash'
    assert (root / 'version.txt').read_text() == 'unpublished change', 'Release must not modify the working tree'
print('PASS: exact tag/commit resolution, dirty-tree isolation and metadata-only inspection')
