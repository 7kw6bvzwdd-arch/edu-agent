"""Local real-file smoke test; never invokes paid providers in sample mode."""
import json
import time
import urllib.request
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
cfg=dict(line.split('=',1) for line in (ROOT/'.env').read_text(encoding='utf-8').splitlines() if '=' in line and not line.startswith('#'))
BASE='http://127.0.0.1:8080/api'
def request(path,body=None,token=''):
    req=urllib.request.Request(BASE+path,data=json.dumps(body).encode() if body is not None else None,headers={'Content-Type':'application/json','Authorization':'Bearer '+token})
    with urllib.request.urlopen(req,timeout=30) as response:return json.load(response)

if __name__=='__main__':
    assert request('/health')['mode']=='sample','This smoke check is for explicit sample mode only'
    token=request('/auth/login',{'username':'admin','password':cfg['SEED_PASSWORD']})['token']
    existing={x['name'] for x in request('/documents',token=token)}
    for path in (ROOT/'fixtures').glob('*.md'):
        if path.name in existing:continue
        boundary='----Edu'+str(time.time_ns())
        body=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{path.name}"\r\nContent-Type: text/markdown\r\n\r\n').encode()+path.read_bytes()+f'\r\n--{boundary}--\r\n'.encode()
        req=urllib.request.Request(BASE+'/documents',data=body,headers={'Content-Type':'multipart/form-data; boundary='+boundary,'Authorization':'Bearer '+token})
        with urllib.request.urlopen(req) as response:assert response.status==202
    for _ in range(100):
        docs=request('/documents',token=token)
        if all(x['status']=='ready' for x in docs):break
        time.sleep(.3)
    evaluation=request('/evaluations',{},token)
    (ROOT/'work/retrieval-evaluation.json').write_text(json.dumps(evaluation,ensure_ascii=False,indent=2),encoding='utf-8')
    print('Retrieval metrics:',json.dumps(evaluation['metrics']),flush=True)
    ident=request('/tasks',{'type':'animation','topic':'浮力','pages':8,'theme':'forest','use_kb':True,'idempotency_key':'smoke-'+str(time.time_ns())},token)['id']
    for _ in range(100):
        task=request('/tasks/'+ident,token=token)
        if task['status']=='awaiting_confirmation':break
        time.sleep(.3)
    assert task['status']=='awaiting_confirmation',task
    request('/tasks/'+ident+'/confirm',{'result':task['result'],'revision':task['revision'],'accept_review':True},token)
    for _ in range(150):
        task=request('/tasks/'+ident,token=token)
        if task['status'] in ('succeeded','degraded','failed'):break
        time.sleep(1)
    (ROOT/'work/animation-smoke.json').write_text(json.dumps(task,ensure_ascii=False,indent=2),encoding='utf-8')
    print('Animation:',task['status'],ident,flush=True)
    assert task['status'] in ('succeeded','degraded'),task.get('error')
    with urllib.request.urlopen(urllib.request.Request(BASE+'/tasks/'+ident+'/download/mp4',headers={'Authorization':'Bearer '+token})) as r:
        media=r.read();assert len(media)>1000
        (ROOT/'work/animation-sample.mp4').write_bytes(media)
    print('Real MP4 bytes:',len(media),flush=True)
