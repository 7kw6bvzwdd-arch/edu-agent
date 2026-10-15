import io
import os
from pathlib import Path

import pymupdf
import pytest

from fastapi.testclient import TestClient
from pypdf import PdfWriter
from pypdf.generic import DictionaryObject, NameObject, DecodedStreamObject

from app import app


def test_pdf_fraction_exponent_and_table_blanks():
    # Real PDF positions: a fraction is not the adjacent string '6x'.
    with pymupdf.open() as doc:
        page = doc.new_page(width=400, height=400)
        page.insert_text((30, 50), 'y =', fontsize=12)
        page.insert_text((58, 43), '6', fontsize=12)
        page.insert_text((58, 60), 'x', fontsize=12)
        page.draw_line((56, 47), (67, 47), width=.4)
        page.insert_text((100, 50), 'x', fontsize=12)
        page.insert_text((106, 45), '2', fontsize=7)
        for x in (30, 110, 190, 270):
            page.draw_line((x, 100), (x, 190), width=.5)
        for y in (100, 130, 160, 190):
            page.draw_line((30, y), (270, y), width=.5)
        for x, y, text in ((40,120,'x'),(120,120,'1'),(200,120,'2'),
                           (40,150,'y'),(120,150,'6'),(40,180,'z'),(200,180,'3')):
            page.insert_text((x,y),text,fontsize=12)
        data=doc.tobytes()
    with TestClient(app) as client:
        result=client.post('/extract',files={'file':('math.pdf',data,'application/pdf')})
    assert result.status_code == 200, result.text
    text=result.json()['pages'][0]['text']
    assert '6/x' in text
    assert 'x^2' in text
    assert 'y | 6 | [空白]' in text
    assert 'z | [空白] | 3' in text


def test_actual_uploaded_textbook_math():
    path=os.environ.get('EDU_TEXTBOOK_PDF')
    if not path:
        pytest.skip('Set EDU_TEXTBOOK_PDF to the original uploaded textbook')
    with TestClient(app) as client:
        result=client.post('/extract',files={'file':('textbook.pdf',Path(path).read_bytes(),'application/pdf')})
    assert result.status_code == 200, result.text
    pages=result.json()['pages']
    assert len(pages)==122
    text=pages[10]['text']
    assert 'y=6/x' in text
    assert 'y=12/x' in text
    assert 'y=ax^2+bx+c' in text
    assert 'x | … | -12 | -6 | -4 | -3 | -2 | -1 | 1 | 2 | 3 | 4 | 6 | 12 | …' in text
    assert 'y=6/x | … | [空白] | [空白] | -1.5 | -2 | [空白] | [空白] | 6' in text
    assert not any('\x00' in p['text'] for p in pages)
    assert '6=k/2' in pages[9]['text']
    assert 'y=1/(x^2)' in ''.join(pages[9]['text'].split())
    assert '∠A=30°' in pages[67]['text']
    assert 'BC/(AB)=1/2' in ''.join(pages[67]['text'].split())
    assert '61' in pages[67]['text'].splitlines()[-1]
    assert '78°' in pages[32]['text'] and '118°' in pages[32]['text']
    assert 'unresolved_math_layout' in pages[32]['extraction_warnings']
    assert 'unreadable_body_glyph' in pages[8]['extraction_warnings']


def test_pdf_does_not_invent_division_and_multidigit_exponents():
    with pymupdf.open() as doc:
        page=doc.new_page()
        page.insert_text((30,50),'6x',fontsize=12)
        page.draw_line((30,54),(45,54),width=.4)  # underline, no denominator
        page.insert_text((100,50),'ax',fontsize=12)
        page.insert_text((113,45),'12',fontsize=7)
        data=doc.tobytes()
    with TestClient(app) as client:
        result=client.post('/extract',files={'file':('math.pdf',data,'application/pdf')})
    assert result.status_code==200,result.text
    text=result.json()['pages'][0]['text']
    assert '6x' in text and '6/x' not in text
    assert 'ax^(12)' in text


@pytest.mark.parametrize('numerator,denominator,script,marker,expected',[
    ('1','x','2','^','1/(x^2)'),
    ('AB','A','1','_','AB/(A_1)'),
    ('x','y','12','^','x/(y^(12))'),
])
def test_pdf_fraction_keeps_scripts_on_their_actual_term(numerator,denominator,script,marker,expected):
    with pymupdf.open() as doc:
        page=doc.new_page()
        page.insert_text((40,50),numerator,fontsize=12)
        page.draw_line((38,54),(66,54),width=.4)
        page.insert_text((40,68),denominator,fontsize=12)
        page.insert_text((48,63 if marker=='^' else 73),script,fontsize=7)
        data=doc.tobytes()
    with TestClient(app) as client:
        result=client.post('/extract',files={'file':('math.pdf',data,'application/pdf')})
    assert result.status_code==200,result.text
    assert expected in result.json()['pages'][0]['text']


def test_original_pdf_page_image_is_bounded_and_page_specific():
    from PIL import Image
    import base64
    with pymupdf.open() as doc:
        p=doc.new_page(width=200,height=300)
        p.insert_text((30,60),'First page')
        p=doc.new_page(width=300,height=200)
        p.insert_text((30,60),'Second page')
        pdf=doc.tobytes()
    with TestClient(app) as client:
        result=client.post('/document/page-image?page=2',files={'file':('book.pdf',pdf,'application/pdf')})
        assert result.status_code==200,result.text
        body=result.json();assert body['page']==2
        im=Image.open(io.BytesIO(base64.b64decode(body['data_url'].split(',')[1])))
        assert im.width>im.height and max(im.size)<=1600
        for n in (0,3,301):
            assert client.post('/document/page-image?page='+str(n),files={'file':('book.pdf',pdf,'application/pdf')}).status_code==422


def test_pdf_unknown_glyph_is_flagged_encryption_and_scans_still_rejected():
    # Unmapped text must not quietly turn into a verified teaching source.
    from pdf_extract import _chars
    class UnknownPage:
        rect=pymupdf.Rect(0,0,400,400)
        def get_text(self,kind):
            return {'blocks':[{'type':0,'lines':[{'spans':[{
                'font':'Unmapped','size':12,'chars':[{'c':'\ue033','origin':(30,50),'bbox':(30,38,42,50)}]
            }]}]}]}
    warnings=set()
    _chars(UnknownPage(),warnings)
    assert 'unreadable_body_glyph' in warnings
    with pymupdf.open() as doc:
        doc.new_page()
        scanned=doc.tobytes()
        encrypted=doc.tobytes(encryption=pymupdf.PDF_ENCRYPT_AES_256,owner_pw='test',user_pw='test')
    with TestClient(app) as client:
        for data in (scanned,encrypted):
            assert client.post('/extract',files={'file':('math.pdf',data,'application/pdf')}).status_code==422


def test_document_nul_separators_preserve_math_and_page_text():
    text = '锐角三角函数\nA\x00\nB\x00C\x00图28.1-2\n∠C=90°\tBC/AB=1/2，ρ液=1000kg/m³'
    with TestClient(app) as client:
        response = client.post('/extract', files={'file': ('text.txt', text.encode('utf-8'), 'text/plain')})
    assert response.status_code == 200, response.text
    assert response.json()['pages'] == [{'page': 1, 'text': text.replace('\x00', ' ')}]


def test_document_upload_100mb_boundary():
    writer = PdfWriter()
    page = writer.add_blank_page(width=300, height=300)
    font = DictionaryObject({NameObject('/Type'): NameObject('/Font'),
                             NameObject('/Subtype'): NameObject('/Type1'),
                             NameObject('/BaseFont'): NameObject('/Helvetica')})
    page[NameObject('/Resources')] = DictionaryObject({
        NameObject('/Font'): DictionaryObject({NameObject('/F1'): writer._add_object(font)})})
    stream = DecodedStreamObject()
    stream.set_data(b'BT /F1 12 Tf 20 200 Td (Large document boundary) Tj ET')
    page[NameObject('/Contents')] = writer._add_object(stream)
    output = io.BytesIO()
    writer.write(output)
    pdf = output.getvalue()
    split = pdf.rindex(b'startxref')
    # An ignored PDF comment keeps all object/xref offsets valid and text small.
    padding = 100 * 1024 * 1024 - len(pdf)
    pdf = pdf[:split] + b'%' + b' ' * (padding - 2) + b'\n' + pdf[split:]
    with TestClient(app) as client:
        accepted = client.post('/extract', files={'file': ('large.pdf', pdf, 'application/pdf')})
        assert accepted.status_code == 200, accepted.text
        assert 'Large document boundary' in accepted.json()['pages'][0]['text']
        rejected = client.post('/extract', files={'file': ('large.pdf', pdf + b' ', 'application/pdf')})
        assert rejected.status_code == 413, rejected.text
        assert rejected.json()['detail'] == '文件超过100MB'
