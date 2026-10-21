from fastapi.testclient import TestClient
import pytest
from app import app


def check(source):
    frames=[{'time':t,'duration':1000,'progress':t/1000,'cues':{}} for t in [0,500,1000]]
    return TestClient(app).post('/render/authored-animation-check',json={'representative':'s1','scenes':[{'id':'s1','html':source,'frames':frames}]})


def test_real_authored_motion_has_repeatable_reverse_seek():
    result=check('<svg width="1280" height="608"><circle id="c" cy="200" r="40" fill="blue"/></svg><script>window.renderFrame=f=>document.getElementById("c").setAttribute("cx",100+f.progress*700)</script>')
    assert result.status_code==200,result.text
    assert result.json()['scenes'][0]['reverse_seek'] is True
    assert result.json()['scenes'][0]['changed'] is True


@pytest.mark.parametrize('source',[
    '<h1>Only static text</h1><script>window.renderFrame=f=>{}</script>',
    '<svg width="1280" height="608"><circle id="c" cy="200" r="40"/></svg><script>var x=20;window.renderFrame=f=>document.getElementById("c").setAttribute("cx",x+=50)</script>',
    '<script>fetch("http://127.0.0.1:1/private");window.renderFrame=f=>{}</script>',
    '<script>window.renderFrame=f=>{while(true){}}</script>',
])
def test_invalid_authored_motion_cannot_pass_preflight(source):
    result=check(source)
    assert result.status_code==422
    assert result.json()['detail']['code']=='authored_animation_invalid'


def test_invalid_svg_coordinates_return_actionable_safe_reason():
    result=check('<svg><polygon id="p"/></svg><script>window.renderFrame=f=>document.getElementById("p").setAttribute("points","0,0 NaN,5 50,50")</script>')
    assert result.status_code==422
    assert result.json()['detail']=={'code':'authored_animation_invalid','field':'html','reason':'invalid_geometry'}
