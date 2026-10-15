"""PDF text with bounded recovery of visible mathematical structure.

This is not OCR or a general formula recognizer. Unknown mappings remain flagged.
"""
import re
import pymupdf

VERSION = 'math-layout-v2'
# Verified against the uploaded textbook's rendered E-BX9-PK7481a5 glyphs.
# Never replace these Chinese characters in ordinary prose or unrelated fonts.
BOOK_GLYPHS = str.maketrans({'犪':'a', '犫':'b', '犮':'c', '犽':'k',
                           '狓':'x', '狔':'y', '犃':'A', '犅':'B', '犆':'C',
                           '犇':'D', '犈':'E', '犉':'F', '犌':'G', '犎':'H',
                           '犛':'S', '犱':'d', '犺':'h', '狆':'p', '狊':'s',
                           '狋':'t', '狏':'v', '犿':'m', '狀':'n', '狉':'r',
                           '狕':'z', '犾':'l', '犻':'i', '狑':'w',
                           '犐':'I', '犑':'J', '犓':'K', '犔':'L', '犕':'M',
                           '犖':'N', '犗':'O', '犘':'P', '犙':'Q', '犚':'R',
                           '犜':'T', '犝':'U', '犞':'V', '\ue011':'-'})
BOOK_FONTS={'E-BX9-PK748162','E-BX9-PK748163','E-BX9-PK7481a2',
            'E-BX9-PK7481a4','E-BX9-PK7481a5'}
FULLWIDTH = str.maketrans({**{chr(n):chr(n-0xfee0) for n in range(0xff01,0xff5f)}, '\u3000':' '})
ATOM = re.compile(r'[A-Za-z0-9.+−\-()^_]+\Z')


def _chars(page, warnings):
    chars=[]
    for block in page.get_text('rawdict')['blocks']:
        if block['type'] != 0:
            continue
        for line in block['lines']:
            line_id=len(chars)
            for span in line['spans']:
                font=span['font']
                for char in span['chars']:
                    text=char['c'].translate(FULLWIDTH).replace('\x00',' ')
                    if font in BOOK_FONTS:
                        text=text.translate(BOOK_GLYPHS)
                    if font == 'HYa6gj-GBK-EUC-H200005e9' and text == 'e':
                        text='°'  # Verified chart angle labels on original file page 33.
                    if re.match(r'FzBookMaker\d+DlFont',font):
                        # No Unicode map: its decoded ASCII is not its visible text.
                        margin=char['origin'][1] > page.rect.height*.9
                        warnings.add('unreadable_margin_text' if margin else 'unreadable_body_glyph')
                        text='[页边文字未提取]' if margin else '[未识别字形]'
                    elif re.search(r'[\ue000-\uf8ff\ufffd]',text) or (font.startswith('E-BX9-') and re.search('[\u4e00-\u9fff]',text)):
                        warnings.add('unreadable_body_glyph')
                    chars.append({'text':text,'size':span['size'],'origin':char['origin'],
                                  'bbox':char['bbox'],'line':line_id,'used':False})
    return chars


def _join(chars):
    lines={}
    for char in chars:
        if char['used']:
            continue
        lines.setdefault(char['line'],[]).append(char)
    texts=[''.join(c['text'] for c in sorted(line,key=lambda c:c['origin'][0])) for line in lines.values()]
    text='\n'.join(s.strip() for s in texts if s.strip())
    return re.sub(r'(\[页边文字未提取\]){2,}',r'\1',text)


def _fractions(page, chars, warnings):
    # Require an actual short horizontal stroke and simple terms above/below it.
    # ponytail: complex/nested fractions stay unrecognized; add a math OCR path only
    # after a verified corpus justifies it, never infer division from proximity alone.
    for path in page.get_drawings():
        if path['type'] != 's' or len(path['items']) != 1:
            continue
        item=path['items'][0]
        if item[0] != 'l':
            continue
        a,b=item[1:]
        x0,x1=sorted((a.x,b.x))
        if abs(a.y-b.y)>.2 or not 3<x1-x0<80:
            continue
        nearby=[c for c in chars if not c['used'] and c['text'].strip()
                and x0-1 <= c['origin'][0]+min(c['bbox'][2]-c['bbox'][0],c['size']*.6)/2 <= x1+1
                and abs(c['origin'][1]-a.y) <= c['size']*1.25]
        top=[c for c in nearby if .5<a.y-c['origin'][1]<c['size']*.65]
        bottom=[c for c in nearby if .5<c['origin'][1]-a.y<c['size']*1.25]
        if not top or not bottom:
            continue
        # The next body line can also fall inside a legacy font's inflated size.
        top_y=max(c['origin'][1] for c in top)
        bottom_y=min(c['origin'][1] for c in bottom)
        top=[c for c in top if abs(c['origin'][1]-top_y)<2]
        bottom=[c for c in bottom if abs(c['origin'][1]-bottom_y)<2]
        num=''.join(c['text'] for c in sorted(top,key=lambda c:c['origin'][0]))
        den=''.join(c['text'] for c in sorted(bottom,key=lambda c:c['origin'][0]))
        if re.search(r'[A-Za-z][0-9]',num+den):
            warnings.add('unresolved_math_layout')
        if not ATOM.fullmatch(num) or not ATOM.fullmatch(den):
            warnings.add('unresolved_math_layout')
            continue
        if not .75<max(c['size'] for c in top)/max(c['size'] for c in bottom)<1.33:
            warnings.add('unresolved_math_layout')
            continue
        group=top+bottom
        first=min(group,key=lambda c:chars.index(c))
        first['text']=(f'({num})' if len(num)>1 and not num.isalnum() else num)+'/'+(f'({den})' if len(den)>1 else den)
        first['origin']=(x0,(min(c['origin'][1] for c in top)+max(c['origin'][1] for c in bottom))/2+2)
        for c in group:
            if c is not first:
                c['used']=True


def _scripts(chars):
    scripts={}
    for i,c in enumerate(chars):
        if c['used'] or not re.fullmatch('[A-Za-z0-9+-]',c['text']):
            continue
        candidates=[p for p in chars[max(0,i-6):i] if not p['used']
                    and p['line']==c['line']
                    and re.search(r'[A-Za-z0-9)]$',p['text'])
                    and .3 < c['size']/p['size'] < .8
                    and (0 < c['origin'][0]-p['origin'][0] < p['size']*.9
                         or id(p) in scripts and 0 <= c['origin'][0]-p['bbox'][2] < c['size']*.4)
                    and p['size']*.12 < abs(c['origin'][1]-p['origin'][1]) < p['size']*.6]
        if candidates:
            base=min(candidates,key=lambda p:(abs(c['origin'][1]-p['origin'][1]),-p['origin'][0]))
            marker='^' if c['origin'][1]<base['origin'][1] else '_'
            entry=scripts.setdefault(id(base),(base,marker,[]))
            if entry[1] != marker:
                continue
            entry[2].append(c['text'])
            base['bbox']=(base['bbox'][0],base['bbox'][1],c['bbox'][2],base['bbox'][3])
            c['used']=True
    for base,marker,values in scripts.values():
        value=''.join(values)
        base['text']+=marker+(value if len(value)==1 else f'({value})')


def _tables(page, chars):
    def point(c):
        # Legacy fonts have inflated advance boxes; origins near a ruling can
        # belong to the cell on its right. Use the glyph's horizontal centre.
        x=c['origin'][0]+min(c['bbox'][2]-c['bbox'][0],c['size']*.6)/2
        return pymupdf.Point(x,c['origin'][1])

    for table in page.find_tables().tables:
        # Charts/axes and note boxes can be falsely detected as tables. Only
        # fully ruled rectangular grids with >=3 columns are reconstructed.
        if table.row_count<2 or table.col_count<3 or any(cell is None for row in table.rows for cell in row.cells):
            continue
        inside=[c for c in chars if not c['used'] and point(c) in pymupdf.Rect(table.bbox)]
        if not inside:
            continue
        rows=[]
        for row in table.rows:
            cells=[]
            for bbox in row.cells:
                cell=[c for c in inside if point(c) in pymupdf.Rect(bbox)]
                cells.append(_join(cell).replace('\n',' ').strip() or '[空白]')
            rows.append(' | '.join(cells))
        first=min(inside,key=lambda c:chars.index(c))
        first['text']='[表格；空白为原文留空]\n'+'\n'.join(rows)
        for c in inside:
            if c is not first:
                c['used']=True


def extract_pdf(data, max_pages=300):
    with pymupdf.open(stream=data,filetype='pdf') as doc:
        if doc.is_encrypted:
            raise ValueError('暂不支持加密PDF')
        if len(doc)>max_pages:
            raise ValueError(f'文档超过{max_pages}页')
        pages=[]
        for n,page in enumerate(doc):
            warnings=set()
            chars=_chars(page,warnings)
            _scripts(chars)
            _fractions(page,chars,warnings)
            _tables(page,chars)
            text=_join(chars)
            if warnings & {'unreadable_body_glyph','unresolved_math_layout'}:
                text='[PDF文字待核对：部分字形或公式位置未识别，不能作为已核验的公式依据]\n'+text
            result={'page':n+1,'text':text,'extraction_version':VERSION}
            if warnings:
                result['extraction_warnings']=sorted(warnings)
            pages.append(result)
        return pages
