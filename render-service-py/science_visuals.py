"""Trusted scientific drawings. Author input is data, never executable code."""
import base64
import io
import json
import math
from functools import lru_cache

from PIL import Image, ImageDraw, ImageFont
from matplotlib.font_manager import findfont, FontProperties
from pptx.util import Inches
from rdkit import Chem, rdBase
from rdkit.Chem import rdMolDescriptors
from rdkit.Chem.Draw import rdMolDraw2D
from general_scene import draw_scene
from carbon_visuals import draw_carbon, metadata as carbon_metadata

KINDS = ('thin_lens', 'molecule', 'cell', 'scene', 'carbon')
GROUPS = {'none':None, 'hydroxyl':'[O;H1;!$(O-C=O)]',
          'carboxyl':'[CX3](=[OX1])[OX2H1]', 'amino':'[NX3;H2,H1;!$(N-C=O)]',
          'carbonyl':'[CX3]=[OX1]', 'double_bond':'[CX3]=[CX3]'}
GROUP_NAMES = dict(none='无',hydroxyl='羟基',carboxyl='羧基',amino='氨基',carbonyl='羰基',double_bond='碳碳双键')
CELL_PARTS = dict(cell_wall='细胞壁',cell_membrane='细胞膜',cytoplasm='细胞质',nucleus='细胞核',
                  mitochondrion='线粒体',chloroplast='叶绿体',vacuole='中央液泡')


def molecule(d):
    # Parser diagnostics include user input; keep it out of service logs.
    with rdBase.BlockLogs():mol=Chem.MolFromSmiles(d.smiles)
    if mol is None or not 1<=mol.GetNumAtoms()<=80 or len(Chem.GetMolFrags(mol))!=1:raise ValueError('Invalid molecule')
    matches=mol.GetSubstructMatches(Chem.MolFromSmarts(GROUPS[d.highlight_group])) if GROUPS[d.highlight_group] else ()
    if d.highlight_group!='none' and not matches:raise ValueError('Functional group absent')
    atoms=sorted({i for match in matches for i in match})
    bonds=sorted({b.GetIdx() for match in matches for b in mol.GetBonds() if b.GetBeginAtomIdx() in match and b.GetEndAtomIdx() in match})
    return mol,atoms,bonds


@lru_cache(maxsize=16)
def font(size, family):
    return ImageFont.truetype(findfont(FontProperties(family=family)), size)


def lens_state(d):
    f, u = d.focal_length_cm, d.object_distance_cm
    v = None if math.isclose(f, u, rel_tol=1e-9) else f*u/(u-f)
    m = None if v is None else -v/u
    nature = '不成有限远像（出射光平行）' if v is None else ('倒立实像' if v > 0 else '正立虚像')
    return dict(image_distance_cm=v, magnification=m, nature=nature)


def draw_frame(d, family, progress=1):
    composed=d.kind=='scene' and any(o.type=='carbon' for o in d.scene.objects)
    image = Image.new('RGB', (1000, 400 if composed else 620), 'white')
    pen = ImageDraw.Draw(image)
    ink, blue, orange, grey = '#18324c', '#1767bf', '#bc582b', '#60758a'
    def text(x, y, value, color=ink, size=25, anchor=None):
        pen.text((x,y), str(value), font=font(size,family), fill=color, anchor=anchor)
    text(30, 16, d.title, blue, 30)
    if d.kind=='carbon':
        draw_carbon(pen,d.allotrope,progress,text)
    elif d.kind=='scene':
        draw_scene(image,d.scene,progress,lambda size:font(size,family))
        text(30,370 if composed else 590,'教学示意 · 非真实比例；结构变化与观察视角按旁白区分',grey,18)
    elif d.kind == 'thin_lens':
        if d.animation:
            d = d.model_copy(update={'object_distance_cm':d.object_distance_cm+(d.animation.to-d.object_distance_cm)*progress})
        state = lens_state(d);f,u,h = d.focal_length_cm,d.object_distance_cm,d.object_height_cm
        v,m=state['image_distance_cm'],state['magnification']
        # One fixed scale across the animation, with explicit offscreen notice.
        extent=max(3*f,u,d.animation.to if d.animation else 0)
        scale=390/extent; ox,oy=500,310
        def xy(x,y):return (ox+x*scale,oy-y*scale)
        pen.line((35,oy,965,oy),fill=grey,width=2)
        pen.ellipse((ox-15,95,ox+15,490),outline=blue,width=3)
        for value,label in [(-2*f,'2F'),(-f,'F'),(f,"F′"),(2*f,"2F′")]:
            x,y=xy(value,0);pen.line((x,y-5,x,y+5),fill=grey,width=2);text(x,y+10,label,grey,22,'mt')
        def ray(a,b,color,dashed=False):
            # Clip every segment to the drawing area, including near focal infinity.
            x1,y1=xy(*a);x2,y2=xy(*b);dx,dy=x2-x1,y2-y1;t0,t1=0.,1.
            for p,q in [(-dx,x1-35),(dx,965-x1),(-dy,y1-90),(dy,490-y1)]:
                if p==0:
                    if q<0:return
                elif p<0:t0=max(t0,q/p)
                else:t1=min(t1,q/p)
            if t0>t1:return
            a=(x1+dx*t0,y1+dy*t0);b=(x1+dx*t1,y1+dy*t1)
            if dashed:
                length=math.dist(a,b);n=max(1,int(length/10))
                for i in range(0,n,2):pen.line((a[0]+(b[0]-a[0])*i/n,a[1]+(b[1]-a[1])*i/n,a[0]+(b[0]-a[0])*min(i+1,n)/n,a[1]+(b[1]-a[1])*min(i+1,n)/n),fill=color,width=2)
            else:pen.line((*a,*b),fill=color,width=3)
        def arrow(x,height,color,label):
            base,tip=xy(x,0),xy(x,height)
            if not(90<=tip[1]<=490 and 35<=tip[0]<=965):return False
            pen.line((*base,*tip),fill=color,width=5)
            direction=1 if height>0 else -1
            pen.polygon([tip,(tip[0]-7,tip[1]+direction*12),(tip[0]+7,tip[1]+direction*12)],fill=color)
            text(tip[0],tip[1]-30 if height>0 else tip[1]+12,label,color,23,'mt');return True
        arrow(-u,h,blue,'物')
        ray((-u,h),(0,h),orange);ray((0,h),(extent,h*(1-extent/f)),orange)
        ray((-u,h),(extent,-h*extent/u), '#278567')
        if v is not None:
            if v<0:
                ray((0,h),(v,m*h),orange,True);ray((0,0),(v,m*h),'#278567',True)
            visible=arrow(v,m*h,orange,'像（虚）' if v<0 else '像（实）')
            text(30,520,f'u = {u:.3g} cm    v = {v:.3g} cm    m = {m:.3g}   {state["nature"]}',size=25)
            if not visible:text(30,557,'像超出当前绘图区；像距与放大率见数值。',orange,23)
        else:text(30,520,'u = f：不成有限远像；两条出射光平行。',orange,25)
        text(30,590,'理想薄透镜 · 近轴光线 · 示意，非实测；虚线为光线反向延长线',grey,20)
    elif d.kind == 'molecule':
        mol,atoms,bonds=molecule(d)
        drawer=rdMolDraw2D.MolDraw2DCairo(920,430)
        options=drawer.drawOptions();options.addAtomIndices=True;options.explicitMethyl=True
        options.minFontSize=20;options.maxFontSize=34
        strength=progress if d.animation else 1
        color=(1.,1.-.35*strength,1.-.8*strength)
        drawer.DrawMolecule(mol,highlightAtoms=atoms,highlightBonds=bonds,
                            highlightAtomColors={i:color for i in atoms},highlightBondColors={i:color for i in bonds})
        drawer.FinishDrawing()
        with Image.open(io.BytesIO(drawer.GetDrawingText())) as structure:image.paste(structure,(40,70))
        text(30,510,'分子式：'+rdMolDescriptors.CalcMolFormula(mol)+'    高亮：'+GROUP_NAMES[d.highlight_group],size=26)
        text(30,552,'小号数字为原子编号；键线式省略的氢按价态补足。',grey,23)
        text(30,590,'RDKit二维结构示意 · 不表示真实空间构型或反应已验证',grey,20)
    elif d.kind == 'cell':
        visible=set(d.visible_parts);highlight=set(d.highlight_parts)
        strength=progress if d.animation else 1
        def outline(part):return orange if part in highlight and strength>.02 else blue
        def width(part):return 3+int(strength*6) if part in highlight else 3
        def fill(part,color):
            if part not in highlight:return color
            rgb=Image.new('RGB',(1,1),color).getpixel((0,0));target=(255,211,122)
            return tuple(int(a+(b-a)*strength) for a,b in zip(rgb,target))
        pen.rounded_rectangle((120,110,660,510),radius=95 if d.cell_type=='plant' else 190,fill='#f8fafc')
        if 'cell_wall' in visible:pen.rounded_rectangle((115,105,665,515),radius=70,outline=outline('cell_wall'),width=width('cell_wall'))
        if 'cytoplasm' in visible:pen.rounded_rectangle((135,125,645,495),radius=80 if d.cell_type=='plant' else 175,fill=fill('cytoplasm','#f7ecdb'))
        if 'cell_membrane' in visible:pen.rounded_rectangle((135,125,645,495),radius=80 if d.cell_type=='plant' else 175,outline=outline('cell_membrane'),width=width('cell_membrane'))
        if 'vacuole' in visible:pen.rounded_rectangle((330,185,565,410),radius=75,fill=fill('vacuole','#ddf2fd'),outline=outline('vacuole'),width=width('vacuole'))
        if 'nucleus' in visible:
            pen.ellipse((205,235,310,350),fill=fill('nucleus','#e7d9f6'),outline=outline('nucleus'),width=width('nucleus'))
            pen.ellipse((240,275,265,300),fill='#9a78b8')
        if 'chloroplast' in visible:
            for x,y in [(215,170),(570,290),(360,440)]:
                pen.ellipse((x-36,y-20,x+36,y+20),fill=fill('chloroplast','#b6dfaf'),outline=outline('chloroplast'),width=width('chloroplast'))
                for dx in [-18,0,18]:pen.line((x+dx,y-10,x+dx,y+10),fill='#3c8153',width=3)
        if 'mitochondrion' in visible:
            for x,y in [(230,405),(545,445)]:
                pen.ellipse((x-32,y-18,x+32,y+18),fill=fill('mitochondrion','#f3c7aa'),outline=outline('mitochondrion'),width=width('mitochondrion'))
                pen.line((x-20,y,x-10,y-9,x,y+9,x+10,y-9,x+20,y),fill='#a26444',width=2)
        anchors=dict(cell_wall=(118,240),cell_membrane=(140,220),cytoplasm=(390,150),nucleus=(260,290),
                     vacuole=(450,270),chloroplast=(570,290),mitochondrion=(545,445))
        for i,part in enumerate(d.visible_parts):
            y=115+i*57;x0,y0=anchors[part];color=outline(part)
            # Numbered keys keep labels legible without a web of crossing leaders.
            pen.ellipse((x0-13,y0-13,x0+13,y0+13),fill='white',outline=color,width=2)
            text(x0,y0-1,str(i+1),color,19,'mm')
            text(720,y,f'{i+1}  {CELL_PARTS[part]}',color,26)
        text(30,555,('植物' if d.cell_type=='plant' else '动物')+'细胞教学示意；仅展示选定结构。',grey,23)
        text(30,590,'程序绘制的结构示意，不代表真实比例、显微照片或全部细胞类型',grey,20)
    else:
        raise ValueError('Unsupported scientific drawing')
    output=io.BytesIO();image.save(output,format='PNG');return output.getvalue()


def summary(d):
    if d.kind=='carbon':return '碳单质结构教学示意；局部网络/层间滑动/理想C60笼，非真实比例或转化仿真。',carbon_metadata(d.allotrope)
    if d.kind=='scene':
        from carbon_visuals import native_model
        return '结构化教学示意；图形与动作按描述绘制，非学科仿真或实测。',dict(object_count=len(d.scene.objects),action_count=len(d.scene.actions),duration_ms=d.scene.duration_ms,models={o.id:native_model(o.allotrope) for o in d.scene.objects if o.type=='carbon'})
    if d.kind=='cell':return '程序绘制的细胞教学示意；不代表真实比例或显微照片。',dict(cell_type=d.cell_type,visible_parts=d.visible_parts,highlight_parts=d.highlight_parts)
    if d.kind=='molecule':
        mol,atoms,_=molecule(d)
        return 'RDKit二维结构示意；不表示反应已验证；名称与结构对应关系需教师核对。',dict(formula=rdMolDescriptors.CalcMolFormula(mol),canonical_smiles=Chem.MolToSmiles(mol),highlight_atoms=atoms)
    state=lens_state(d)
    return '理想薄透镜；'+state['nature']+'；示意，非实测。',state


def encoded(raw):return 'data:image/png;base64,'+base64.b64encode(raw).decode()


def gif_bytes(images,duration,poster=None):
    output=io.BytesIO()
    # GIF delays have 10ms precision. Include both endpoints with equal delays.
    delay=max(10,round(duration/len(images)/10)*10)
    if poster is not None:
        # PowerPoint thumbnails show the first GIF frame. Give teachers a useful
        # overview, then retain the original empty-to-complete construction.
        images=[poster,*images]
        delay=[1000,*([delay]*(len(images)-1))]
    images[0].save(output,format='GIF',save_all=True,append_images=images[1:],duration=delay,loop=0)
    return output.getvalue()


def render_bundle(d, family, frames=False, native=False):
    note,metadata=summary(d)
    dynamic=bool(d.animation or d.kind=='scene' and d.scene.actions)
    result=dict(image=encoded(draw_frame(d,family,0 if dynamic else 1)),summary=note,metadata=metadata,version='science-visuals-v1')
    native_scene=frames and native and d.kind=='scene'
    if native_scene:
        result['native']=True;result['frames']=[result['image']]
        if not dynamic:return result
    if dynamic:
        count=min(121,max(26,round(d.scene.duration_ms/1000*15)+1)) if d.kind=='scene' and (not frames or native_scene) else 26
        raw_frames=[draw_frame(d,family,i/(count-1)) for i in range(count)]
        duration=d.scene.duration_ms if d.kind=='scene' else d.animation.duration_ms
        images=[Image.open(io.BytesIO(raw)).convert('RGB') for raw in raw_frames]
        result['animation']='data:image/gif;base64,'+base64.b64encode(gif_bytes(images,duration)).decode()
        if frames and not native_scene:result['frames']=[encoded(raw) for raw in raw_frames]
    elif frames:result['frames']=[result['image']]
    return result


def add_science(slide,d,family,x,y,w,h):
    if d.animation or d.kind=='scene' and d.scene.actions:
        duration=d.scene.duration_ms if d.kind=='scene' else d.animation.duration_ms
        count=min(121,max(26,round(duration/1000*15)+1)) if d.kind=='scene' else 26
        images=[Image.open(io.BytesIO(draw_frame(d,family,i/(count-1)))).convert('RGB') for i in range(count)]
        poster=images[-1].copy() if d.kind=='scene' and any(a.op=='reveal' for a in d.scene.actions) else None
        raw=gif_bytes(images,duration,poster=poster)
    else:raw=draw_frame(d,family)
    # Keep the drawing aspect ratio; never stretch ray angles or cell structures.
    aspect=400 if d.kind=='scene' and any(o.type=='carbon' for o in d.scene.objects) else 620
    height=min(h,w*aspect/1000);width=height*1000/aspect
    shape=slide.shapes.add_picture(io.BytesIO(raw),Inches(x+(w-width)/2),Inches(y+(h-height)/2),width=Inches(width),height=Inches(height))
    shape.name='scientific-'+d.kind
    shape._element.nvPicPr.cNvPr.set('descr',json.dumps(d.model_dump(),ensure_ascii=False))
