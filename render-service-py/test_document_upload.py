import io

from fastapi.testclient import TestClient
from pypdf import PdfWriter
from pypdf.generic import DictionaryObject, NameObject, DecodedStreamObject

from app import app


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
