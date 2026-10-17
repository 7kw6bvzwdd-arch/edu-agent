"""Project-only database status or explicit credential rotation; secrets never printed."""
import argparse
import json
import os
from pathlib import Path
import re
import secrets
import shlex
import socket
import subprocess
from urllib.parse import quote, urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--rotate', action='store_true')
    args = parser.parse_args()
    path = ROOT / '.env'
    lines = path.read_text(encoding='utf-8-sig').splitlines()
    settings = dict(line.split('=', 1) for line in lines if '=' in line and not line.lstrip().startswith('#'))
    host, user = settings['CLOUD_SSH_HOST'], settings.get('CLOUD_SSH_USER', 'root')
    database = urlsplit(settings['DATABASE_URL']).path.lstrip('/')
    if not re.fullmatch(r'[a-zA-Z0-9._:-]+', host) or not re.fullmatch(r'[a-zA-Z0-9_-]+', user) or not re.fullmatch(r'[a-zA-Z0-9_]+', database):
        raise ValueError('Invalid project configuration')
    ssh = ['ssh', '-T', '-i', str(ROOT / 'work/cloud/id_ed25519'), '-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=' + str(ROOT / 'work/cloud/known_hosts'), '-o', 'ConnectTimeout=10', user + '@' + host]
    def query(sql):
        command = ['docker', 'exec', '-i', 'edu-agent-cloud-postgres', 'psql', '-U', 'edu', '-d', database, '-XAt', '-v', 'ON_ERROR_STOP=1']
        result = subprocess.run(ssh + [shlex.join(command)], input=sql.encode(), capture_output=True, timeout=45)
        if result.returncode:
            raise RuntimeError('Project database maintenance command failed')
        return result.stdout.decode().strip()
    status = json.loads(query("SELECT json_build_object('tasks',(SELECT count(*) FROM tasks),'active_workers',(SELECT count(*) FROM tasks WHERE deleted_at IS NULL AND (status IN ('researching','rendering','expanding') OR status LIKE 'anim_%_running' OR animation_export->>'status'='running')),'queued_tasks',(SELECT count(*) FROM tasks WHERE deleted_at IS NULL AND (status IN ('pending','confirmed','expand_pending') OR status LIKE 'anim_%_pending' OR animation_export->>'status'='pending')),'waiting_teacher',(SELECT count(*) FROM tasks WHERE deleted_at IS NULL AND status IN ('awaiting_preview','awaiting_confirmation','anim_plan','anim_representative','anim_acceptance')),'invalid_reservations',(SELECT count(*) FROM members WHERE reserved<0 OR reserved>balance));"))
    if not args.rotate:
        print(json.dumps(status))
        return
    if status['active_workers'] or status['queued_tasks']:
        raise ValueError('Wait for active workers and queue to drain before rotating credentials')
    with socket.socket() as probe:
        if probe.connect_ex(('127.0.0.1', 8080)) == 0:
            raise ValueError('Stop the project backend before rotating credentials')
    checkpoint = ROOT / 'work/private-rotation' / secrets.token_hex(8)
    checkpoint.mkdir(parents=True, exist_ok=False)
    if os.name == 'nt':
        identity = subprocess.check_output(['whoami'], text=True).strip()
        subprocess.run(['icacls', str(checkpoint), '/inheritance:r', '/grant:r', identity + ':(OI)(CI)F', 'SYSTEM:(OI)(CI)F'], check=True, capture_output=True)
    else:
        checkpoint.chmod(0o700)
    admin_password, runtime_password = secrets.token_hex(32), secrets.token_hex(32)
    def replace_password(url, password):
        parts = urlsplit(url)
        if parts.username not in ('edu', 'edu_app') or not parts.hostname:
            raise ValueError('Unexpected database role')
        return urlunsplit(parts._replace(netloc=parts.username + ':' + quote(password, safe='') + '@' + parts.hostname + (':' + str(parts.port) if parts.port else '')))
    updates = {key: replace_password(settings[key], password) for key, password in [('DATABASE_URL', admin_password), ('RUNTIME_DATABASE_URL', runtime_password)]}
    config_path = ROOT / 'work/cloud/config.json'
    config = json.loads(config_path.read_text(encoding='utf-8-sig'))
    config.update(updates)
    # Stage both configurations privately before changing either database role.
    (checkpoint / 'previous.env').write_bytes(path.read_bytes())
    (checkpoint / 'previous-config.json').write_bytes(config_path.read_bytes())
    new_env = chr(10).join(key + '=' + updates[key] if (key := line.split('=', 1)[0]) in updates else line for line in lines) + chr(10)
    (checkpoint / 'next.env').write_text(new_env, encoding='utf-8')
    (checkpoint / 'next-config.json').write_text(json.dumps(config, indent=2), encoding='utf-8')
    query("BEGIN; ALTER ROLE edu PASSWORD '" + admin_password + "'; ALTER ROLE edu_app NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '" + runtime_password + "'; COMMIT;")
    for target, staged in [(path, checkpoint / 'next.env'), (config_path, checkpoint / 'next-config.json')]:
        pending = target.with_name(target.name + '.rotation-pending')
        pending.write_bytes(staged.read_bytes())
        os.replace(pending, target)
    print(json.dumps({'status': 'rotated', 'roles': ['edu', 'edu_app'], 'private_recovery': str(checkpoint), 'restart_required': True}))

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('Database maintenance failed: ' + type(error).__name__ + '; secrets suppressed', file=__import__('sys').stderr)
        raise SystemExit(1)
