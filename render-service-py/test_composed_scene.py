import base64
import io
import zipfile
from fastapi.testclient import TestClient
from lxml import etree
from app import app


def composition():
    return dict(kind='scene', title='同为碳元素，连接方式有什么不同？', scene=dict(duration_ms=6000, objects=[
        dict(id='diamond',type='carbon',allotrope='diamond',x=.25,y=.48,width=.4,height=.66),
        dict(id='graphite',type='carbon',allotrope='graphite',x=.75,y=.48,width=.4,height=.66),
        dict(id='label1',type='text',x=.25,y=.91,text='空间网络',font_size=32),
        dict(id='label2',type='text',x=.75,y=.91,text='层状结构',font_size=32)],actions=[
        dict(object='diamond',op='reveal',to=1,start_ms=0,duration_ms=2000,sentence='n1'),
        dict(object='graphite',op='slide',to=.7,start_ms=2000,duration_ms=3000,sentence='n2')]))


def test_composed_carbon_preview_and_real_pptx_share_structure_and_full_canvas():
    client=TestClient(app);diagram=composition()
    response=client.post('/render/scientific',json=dict(diagram=diagram,frames=True,native=True))
    assert response.status_code==200,response.text
    rendered=response.json()
    assert rendered['native'] is True
    assert len(rendered['metadata']['models']['diamond']['points'])==17
    assert rendered['metadata']['models']['diamond']['central_coordination']==4
    assert rendered['metadata']['models']['graphite']['interlayer_bonds']==0
    assert len(rendered['frames'])==1,'native video must not be quantized into 26 frames'
    response=client.post('/render/pptx',json=dict(title='碳单质',slides=[dict(title='比较原子连接方式',scientific=diagram,bullets=[],focus='同一种元素，可以构成不同结构')]))
    assert response.status_code==200,response.text
    with zipfile.ZipFile(io.BytesIO(response.content)) as archive:
        xml=etree.fromstring(archive.read('ppt/slides/slide1.xml'))
        ns={'p':'http://schemas.openxmlformats.org/presentationml/2006/main','a':'http://schemas.openxmlformats.org/drawingml/2006/main'}
        pictures=xml.findall('.//p:pic',ns)
        assert pictures
        extent=pictures[0].find('.//a:ext',ns)
        assert int(extent.get('cx'))>9*914400,'composed evidence needs full-width slide space'
        assert any(p.startswith('ppt/media/') for p in archive.namelist())


def test_invalid_composition_is_rejected_before_render():
    client=TestClient(app)
    for patch in (dict(op='slide',object='diamond',to=.7),dict(op='reveal',object='diamond',to=2)):
        diagram=composition();diagram['scene']['actions'][0].update(patch)
        assert client.post('/render/scientific',json=dict(diagram=diagram)).status_code==422


def test_pptx_reveal_has_complete_poster_before_construction():
    from PIL import Image, ImageChops
    diagram=composition()
    response=TestClient(app).post('/render/pptx',json=dict(title='Structure',slides=[dict(title='Observe then construct',scientific=diagram,bullets=[])]))
    assert response.status_code==200,response.text
    with zipfile.ZipFile(io.BytesIO(response.content)) as archive:
        media=[p for p in archive.namelist() if p.startswith('ppt/media/') and p.endswith('.gif')]
        assert len(media)==1
        gif=Image.open(io.BytesIO(archive.read(media[0])))
        poster=gif.convert('RGB')
        assert gif.info['duration']==1000,'PPT thumbnail must show the complete structure for one second before constructing'
        assert gif.n_frames>26
        gif.seek(1)
        assert ImageChops.difference(poster,gif.convert('RGB')).getbbox(),'construction must still start from the original initial state'
        # The central atom is present on the left half of the completed poster.
        assert sum(1 for r,g,b in poster.crop((30,65,480,320)).getdata() if b>r+20 and b>g)>30
