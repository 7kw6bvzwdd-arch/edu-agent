"""Collect existing acceptance evidence; never start models or approve lessons."""
import argparse
import csv
import datetime
import hashlib
import json
import pathlib
import shutil

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--root',type=pathlib.Path,default=pathlib.Path(__file__).resolve().parents[1])
parser.add_argument('--output',type=pathlib.Path)
args=parser.parse_args()
root=args.root.resolve()
out=args.output or root/'work/acceptance-closure-20261020'
out.mkdir(parents=True,exist_ok=True)

def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))

def save(name,value):
    (out/name).write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

findings_path=root/'docs/acceptance-findings-20261020.json'
findings=read(findings_path) if findings_path.exists() else {}
runs=[]
for path in sorted((root/'work/courseware-full-quality').glob('*/20261020-*/report.json')):
    report=read(path)
    task=report.get('task',[{}])[0]
    deck=task.get('result') or {}
    checkpoint=path.parent/'checkpoint.json'
    saved=read(checkpoint) if checkpoint.exists() else {}
    usage=report.get('usage',[])
    slides=deck.get('slides',[])
    goal_check=deck.get('deck_check') or {}
    run=path.parent.name
    finding=findings.get(path.parent.parent.name+'/'+run)
    if finding and hashlib.sha256(path.read_bytes()).hexdigest()!=finding['report_sha256']:
        raise ValueError('Acceptance findings do not match original report: '+str(path))
    engineering_status='passed' if report.get('passed') else 'failed'
    quality_status=finding['quality_status'] if finding else ('pending' if report.get('passed') else 'failed')
    failure_evidence=[]
    log=root/'work/quality-gate'/run/'go.jsonl'
    if log.exists():
        for line in log.read_text(encoding='utf-8-sig').splitlines():
            event=json.loads(line)
            if event.get('OutputType')=='error':
                failure_evidence.append(event.get('Output','').strip())
    fail_fast=report.get('fail_fast')
    if fail_fast is None:
        log=root/'work/quality-gate'/run/'go.jsonl'
        if log.exists():
            fail_fast='first failed live attempt retained' in log.read_text(encoding='utf-8-sig')
    runs.append(dict(topic=path.parent.parent.name,run=run,status=quality_status,
        engineering_status=engineering_status,quality_status=quality_status,acceptance_findings=finding,
        report=str(path.relative_to(root)),mode=report.get('mode'),model=report.get('model'),
        review_model=report.get('review_model'),vision_model=report.get('vision_model'),seconds=report.get('seconds'),
        pages_saved=len(slides),pages_planned=len((deck.get('plan') or {}).get('pages',[])),
        goals=len((deck.get('plan') or {}).get('learning_goals',[])),
        stage_before_cleanup=saved.get('stage'),charge_after_cleanup=task.get('charge_state'),
        issue=(deck.get('generation') or {}).get('issue') or (failure_evidence[0] if failure_evidence else task.get('error')),
        failure_evidence=failure_evidence,
        full_deck_status=goal_check.get('status'),full_deck_check=goal_check,
        image_pages=[dict(order=s.get('order'),asset_id=(s.get('picture') or {}).get('asset_id'),source_label=(s.get('picture') or {}).get('source'),
            provider=((s.get('picture') or {}).get('provenance') or {}).get('provider'),
            search=(s.get('image_search') or {}).get('status')) for s in slides if s.get('picture') or s.get('image_search')],
        supplier_calls=len(usage),input_tokens=sum(u.get('input_tokens',0) for u in usage),
        output_tokens=sum(u.get('output_tokens',0) for u in usage),failed_calls=sum(not u.get('success') for u in usage),
        fail_fast=fail_fast,test_teacher_approval=report.get('test_teacher_approval'),human_quality_accepted=False,
        image_source_snapshots=read(path.parent/'image-sources.json') if (path.parent/'image-sources.json').exists() else None))
topics=['quadratic','lens','chemistry','biology','history','geography','chinese','english']
save('full-lessons.json',dict(generated_at=datetime.datetime.now().astimezone().isoformat(),
    note='Engineering success is not teaching quality acceptance. Original reports are preserved; independently recorded findings are hash-bound. Unreviewed engineering successes remain pending. Fail-fast stops before business retries are exhausted. Tokens are not a price.',
    topics={t:dict(completed_runs=sum(r['topic']==t for r in runs),
        engineering_passed_runs=sum(r['topic']==t and r['engineering_status']=='passed' for r in runs),
        quality_accepted_runs=sum(r['topic']==t and r['quality_status']=='passed' for r in runs)) for t in topics},runs=runs))

package=out/'classroom-package'
package.mkdir(exist_ok=True)
files=[]
for name,source in [('science.pptx','work/science-visuals/science.pptx'),
                    ('functions.pptx','work/function-families/functions.pptx'),
                    ('native-click.pptx','work/knowledge-click-20261020/native-click.pptx'),
                    ('real-long-conversation.pptx','work/acceptance-closure-20261020/long-20261020-155937/long-conversation.pptx')]:
    path=root/source
    if path.exists():
        shutil.copy2(path,package/name)
        files.append(dict(file=name,source=source,sha256=hashlib.sha256(path.read_bytes()).hexdigest(),
                          mode='live' if name.startswith('real-') else 'fixed_renderer_sample'))
(package/'manifest.json').write_text(json.dumps(files,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

def pending_csv(path,fields,rows):
    if path.exists():return  # Preserve any actual teacher/device review already entered.
    with path.open('w',encoding='utf-8-sig',newline='') as file:
        writer=csv.DictWriter(file,fieldnames=fields);writer.writeheader();writer.writerows(rows)

pending_csv(package/'device-review.csv',
    ['device','os','software_version','file','pages','fonts','formula','layout','gif','click','reopen','notes','status','reviewer','date','evidence'],
    [dict(file=f['file'],status='pending',pages='all') for f in files])
pending_csv(package/'teaching-review.csv',
    ['subject','topic','file','version','reviewer','facts_0_4','goals_tasks_0_4','readability_0_4','images_captions_0_4','pacing_0_4','critical_error','required_changes','status','date'],
    [dict(topic=t,status='pending') for t in topics])

textbook=out/'textbook'
if (textbook/'pages.json').exists():
    pages=read(textbook/'pages.json')['pages']
    warning_rows=[dict(page=p['page'],warnings='; '.join(p.get('extraction_warnings',[])),status='pending')
                  for p in pages if set(p.get('extraction_warnings',[])) & {'unreadable_body_glyph','unresolved_math_layout'}]
    pending_csv(textbook/'formula-review.csv',
        ['page','warnings','original_expression','extracted_expression','correction','critical','status','reviewer','date'],warning_rows)
print(json.dumps(dict(runs=len(runs),engineering_passed=sum(r['engineering_status']=='passed' for r in runs),
    quality_accepted=sum(r['quality_status']=='passed' for r in runs),classroom_files=len(files),output=str(out)),ensure_ascii=False))
