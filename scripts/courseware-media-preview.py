"""Offline PPTX visual fixtures. Uses no API keys, models, or billing."""
import json
import sys
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'render-service-py'))
from app import Deck, THEMES, render

out=ROOT/'work/courseware-teaching-media'
out.mkdir(parents=True,exist_ok=True)
data=json.loads((ROOT/'fixtures/courseware-teaching-media.json').read_text(encoding='utf-8'))
for theme in THEMES:
    data['theme']=theme
    (out/f'{theme}.pptx').write_bytes(render(Deck(**data)))
print(json.dumps({'directory':str(out),'decks':len(THEMES),'slides_each':len(data['slides']),'mode':'offline-fixture'}))
