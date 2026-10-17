"""Back up this project's database, local storage and MinIO prefix; optionally restore into isolation."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tarfile
import tempfile
import uuid
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
CONTAINER = 'edu-agent-cloud-postgres'

def settings():
    values = dict(os.environ)
    for line in (ROOT / '.env').read_text(encoding='utf-8-sig').splitlines():
        if line.strip() and not line.lstrip().startswith('#') and '=' in line:
            key, value = line.split('=', 1)
            values[key.strip()] = value.strip()
    return values

def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()

def private(path):
    if os.name == 'nt':
        identity = subprocess.check_output(['whoami'], text=True).strip()
        subprocess.run(['icacls', str(path), '/inheritance:r', '/grant:r', identity + ':(OI)(CI)F', 'SYSTEM:(OI)(CI)F'], check=True, capture_output=True)
    else:
        path.chmod(0o700)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--drill', action='store_true')
    parser.add_argument('--restore-drill', type=Path, help='Verify and restore an existing backup into a random isolated database/prefix')
    args = parser.parse_args()
    env = settings()
    host, user = env.get('CLOUD_SSH_HOST'), env.get('CLOUD_SSH_USER', 'root')
    if not host or not re.fullmatch(r'[a-zA-Z0-9._:-]+', host) or not re.fullmatch(r'[a-zA-Z0-9_-]+', user):
        raise ValueError('Project SSH configuration required')
    database = unquote(urlsplit(env['DATABASE_URL']).path.lstrip('/'))
    if not re.fullmatch(r'[a-zA-Z0-9_]+', database):
        raise ValueError('Invalid project database name')
    ssh = ['ssh', '-T', '-i', str(ROOT / 'work/cloud/id_ed25519'), '-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=' + str(ROOT / 'work/cloud/known_hosts'), '-o', 'ConnectTimeout=10', user + '@' + host]
    def remote(*command, stdin=None, stdout=subprocess.PIPE):
        result = subprocess.run(ssh + [shlex.join(command)], stdin=stdin, stdout=stdout, stderr=subprocess.PIPE, timeout=1800)
        if result.returncode:
            raise RuntimeError('Project database backup/restore command failed; credentials and remote details suppressed')
        return result.stdout
    go = shutil.which('go') or str(ROOT / 'work/toolchain/go/bin/go.exe')
    def objects(mode, folder):
        subprocess.run([go, 'run', str(ROOT / 'scripts/backup-objects.go'), mode, str(folder)], cwd=ROOT / 'backend-go', env=env, check=True, timeout=1800)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    if args.restore_drill:
        output = args.restore_drill.resolve(strict=True)
        manifest = json.loads((output / 'manifest.json').read_text(encoding='utf-8'))
        if manifest.get('status') != 'complete':
            raise ValueError('Incomplete backup cannot be restored')
    else:
        output = (args.output or ROOT / 'work/backups' / (stamp + '-' + uuid.uuid4().hex[:6])).resolve()
        output.mkdir(parents=True, exist_ok=False)
        private(output)
        (output / 'INCOMPLETE').write_text('Backup did not complete', encoding='utf-8')
        with (output / 'database.dump').open('xb') as target:
            remote('docker', 'exec', CONTAINER, 'pg_dump', '-U', 'edu', '-d', database, '--format=custom', '--no-owner', '--no-privileges', stdout=target)
        storage = Path(env.get('STORAGE_DIR', '../storage'))
        if not storage.is_absolute():
            storage = (ROOT / 'backend-go' / storage).resolve()
        local_files = {}
        with tarfile.open(output / 'storage.tar.gz', 'w:gz') as archive:
            if storage.exists():
                for file in sorted(storage.rglob('*')):
                    if file.is_symlink():
                        raise ValueError('Storage symlink requires operator review')
                    if file.is_file():
                        name = file.relative_to(storage).as_posix()
                        before = digest(file)
                        archive.add(file, arcname=name, recursive=False)
                        if before != digest(file):
                            raise ValueError('Storage changed during backup; retry during quiet period')
                        local_files[name] = before
        if env.get('MINIO_ENDPOINT'):
            objects('backup', output / 'objects')
        manifest = {'version': 1, 'status': 'complete', 'created_at': stamp, 'database': database, 'storage_files': local_files, 'consistency': 'PostgreSQL snapshot; local/object copies are individually verified. Quiesce writes for cross-store point-in-time consistency.', 'files': {file.relative_to(output).as_posix(): digest(file) for file in sorted(output.rglob('*')) if file.is_file() and file.name != 'INCOMPLETE'}}
        (output / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding='utf-8')
        (output / 'INCOMPLETE').unlink()
    for name, expected in manifest['files'].items():
        path = (output / name).resolve()
        if not path.is_relative_to(output) or not path.is_file() or digest(path) != expected:
            raise ValueError('Backup file checksum/path verification failed')
    if args.drill or args.restore_drill:
        isolated = 'edu_restore_' + uuid.uuid4().hex
        remote('docker', 'exec', CONTAINER, 'createdb', '-U', 'edu', isolated)
        try:
            with (output / 'database.dump').open('rb') as source:
                remote('docker', 'exec', '-i', CONTAINER, 'pg_restore', '-U', 'edu', '--dbname=' + isolated, '--exit-on-error', '--no-owner', '--no-privileges', stdin=source)
            counts = remote('docker', 'exec', CONTAINER, 'psql', '-U', 'edu', '-d', isolated, '-At', '-c', "SELECT json_build_object('users',(SELECT count(*) FROM users),'members',(SELECT count(*) FROM members),'tasks',(SELECT count(*) FROM tasks),'ledger',(SELECT count(*) FROM ledger),'invalid_reservations',(SELECT count(*) FROM members WHERE reserved<0 OR reserved>balance));")
            verified_counts = json.loads(counts)
            if verified_counts['invalid_reservations'] != 0:
                raise ValueError('Restored quota invariants failed')
            with tempfile.TemporaryDirectory(prefix='edu-restore-storage-') as temporary:
                with tarfile.open(output / 'storage.tar.gz') as archive:
                    archive.extractall(temporary, filter='data')
                for name, expected in manifest['storage_files'].items():
                    if digest(Path(temporary) / name) != expected:
                        raise ValueError('Restored storage checksum mismatch')
            if (output / 'objects/objects.json').exists():
                objects('restore-drill', output / 'objects')
            (output / 'restore-drill.json').write_text(json.dumps({'status': 'passed', 'at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'counts': verified_counts, 'scope': 'isolated DB restore, extracted local files, temporary MinIO prefix; live data unchanged'}, indent=2), encoding='utf-8')
        finally:
            remote('docker', 'exec', CONTAINER, 'dropdb', '-U', 'edu', '--force', isolated)
    print(json.dumps({'status': 'complete', 'backup': str(output), 'restore_drill': bool(args.drill or args.restore_drill)}))

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('Backup/restore failed: ' + type(error).__name__ + '; details suppressed to protect credentials', file=__import__('sys').stderr)
        raise SystemExit(1)
