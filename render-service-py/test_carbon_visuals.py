import base64
import io
import zipfile
from fastapi.testclient import TestClient
from PIL import Image, ImageChops
from app import app


def test_carbon_connectivity_motion_frames_and_real_pptx():
    client = TestClient(app)
    for kind in ('diamond','graphite','fullerene'):
        diagram = dict(kind='carbon',allotrope=kind,title='碳结构教学示意',
                       animation=dict(param='progress',to=1,duration_ms=3000))
        response = client.post('/render/scientific',json=dict(diagram=diagram,frames=True))
        assert response.status_code == 200,response.text
        result = response.json();m = result['metadata']
        assert len(result['frames']) == 26
        if kind == 'diamond':assert m['central_coordination'] == 4
        if kind == 'graphite':assert m['layers'] == 3 and m['interlayer_bonds'] == 0
        if kind == 'fullerene':assert m['atoms'] == 60 and m['bonds'] == 90 and set(m['degree']) == {3}
        images = [Image.open(io.BytesIO(base64.b64decode(result['frames'][i].split(',')[1]))).convert('RGB') for i in (0,25)]
        # Pixels inside the model region change, independent of the static labels.
        assert ImageChops.difference(*[im.crop((40,80,680,510)) for im in images]).getbbox()
        response = client.post('/render/pptx',json=dict(title='碳单质',slides=[dict(title='观察结构与性质',scientific=diagram)]))
        assert response.status_code == 200,response.text
        with zipfile.ZipFile(io.BytesIO(response.content)) as archive:
            gifs = [n for n in archive.namelist() if n.startswith('ppt/media/') and n.endswith('.gif')]
            assert len(gifs) == 1
            with Image.open(io.BytesIO(archive.read(gifs[0]))) as im:assert im.n_frames >= 20
    for patch in (dict(allotrope='unknown'),dict(animation=dict(param='highlight',to=1,duration_ms=1000))):
        assert client.post('/render/scientific',json=dict(diagram={**diagram,**patch})).status_code == 422
