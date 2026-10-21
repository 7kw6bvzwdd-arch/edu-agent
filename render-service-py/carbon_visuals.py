"""Bounded ball-and-stick teaching models; coordinates and bonds are program-owned."""
import math
from functools import lru_cache


@lru_cache(maxsize=3)
def structure(kind):
    if kind == 'diamond':
        tetra = [(1,1,1),(1,-1,-1),(-1,1,-1),(-1,-1,1)]
        points = [(0,0,0)] + tetra
        for p in tetra:
            points.extend(tuple(a-b for a,b in zip(p,q)) for q in tetra if q != p)
        length = math.sqrt(3)
    elif kind == 'graphite':
        # Seven fused hexagons, repeated as three separate layers.
        sheet = set()
        for q,r in [(q,r) for q in range(-1,2) for r in range(-1,2) if abs(q+r)<=1]:
            cx,cy = 1.5*q, math.sqrt(3)*(r+q/2)
            sheet.update((round(cx+math.cos(i*math.pi/3),6),round(cy+math.sin(i*math.pi/3),6)) for i in range(6))
        points = [(x,y,z) for z in (-1,0,1) for x,y in sorted(sheet)]
        length = 1
    elif kind == 'fullerene':
        # Truncate each directed edge of a regular icosahedron at one third.
        phi = (1+math.sqrt(5))/2
        ico = [(0,a,b*phi) for a in (-1,1) for b in (-1,1)]
        ico += [(a,b*phi,0) for a in (-1,1) for b in (-1,1)]
        ico += [(b*phi,0,a) for a in (-1,1) for b in (-1,1)]
        points = [tuple((2*a+b)/3 for a,b in zip(p,q)) for p in ico for q in ico if math.isclose(math.dist(p,q),2)]
        length = 2/3
    else:
        raise ValueError('Unsupported allotrope')
    bonds = [(i,j) for i,p in enumerate(points) for j,q in enumerate(points[:i])
             if (kind != 'graphite' or p[2] == q[2]) and math.isclose(math.dist(p,q),length,abs_tol=1e-5)]
    return points,bonds


def metadata(kind):
    points,bonds = structure(kind)
    degree = [sum(i in edge for edge in bonds) for i in range(len(points))]
    data = dict(allotrope=kind,atoms=len(points),bonds=len(bonds),degree=degree,model='bounded teaching model; not to scale')
    if kind == 'diamond':data.update(central_coordination=degree[0],boundary='incomplete outer coordination; local network, not a molecule')
    elif kind == 'graphite':data.update(layers=3,interlayer_bonds=0,layer_motion='rigid translation; intralayer bonds retained')
    else:data.update(formula='C60',pentagons=12,hexagons=20)
    return data


def native_model(kind):
    points,bonds=structure(kind)
    return dict(metadata(kind),points=points,edges=bonds)


def draw_composed_carbon(pen,kind,cx,cy,width,height,state):
    points,bonds=structure(kind);angle=.52+state['angle']*math.pi/180
    projected=[]
    for x,y,z in points:
        a,b=x*math.cos(angle)+y*math.sin(angle),-x*math.sin(angle)+y*math.cos(angle)
        projected.append((a+z*state['slide']*.9,b*.34-z*1.65,z+b*.01) if kind=='graphite' else (a,b*.45-z*.89,b*.89+z*.45))
    # Fixed fit across every view; camera motion never changes bond lengths.
    ex=max(math.hypot(x,y)+(.9*abs(z) if kind=='graphite' else 0) for x,y,z in points)
    ey=max(math.hypot(x,y)*(.34 if kind=='graphite' else .45)+abs(z)*(1.65 if kind=='graphite' else .89) for x,y,z in points)
    scale=min(width/(2*ex+.25),height/(2*ey+.25))
    projected=[(cx+x*scale,cy+y*scale,z) for x,y,z in projected]
    reveal=state['reveal'];order=sorted(range(len(points)),key=lambda i:(math.dist(points[i],points[0]),i))
    rank={i:j for j,i in enumerate(order)}
    def alpha(i):return min(1,max(0,reveal*len(points)-rank[i]))
    for i,j in sorted(bonds,key=lambda e:sum(projected[k][2] for k in e)):
        p,q=projected[i],projected[j];opacity=min(alpha(i),alpha(j))
        if opacity:pen.line((*p[:2],*q[:2]),fill=('#bc582b' if state['emphasis']>.05 else '#dc8b29' if kind=='diamond' and 0 in (i,j) else '#6885a0')+format(round(opacity*255),'02x'),width=max(2,round(scale*.045))+round(3*state['emphasis']))
    for i,(x,y,z) in sorted(enumerate(projected),key=lambda item:item[1][2]):
        opacity=alpha(i)
        if opacity:
            r=max(3,min(13,scale*(.075 if kind!='fullerene' else .055)))
            pen.ellipse((x-r,y-r,x+r,y+r),fill=('#de932e' if kind=='diamond' and i==0 else '#197bad')+format(round(opacity*255),'02x'),outline='#ffffff',width=1)


def draw_carbon(pen, kind, progress, text):
    points,bonds = structure(kind)
    projected = []
    angle = .25 + (1.1*progress if kind != 'graphite' else .18)
    for x,y,z in points:
        a,b = x*math.cos(angle)+y*math.sin(angle),-x*math.sin(angle)+y*math.cos(angle)
        if kind == 'graphite':
            projected.append((370+a*64+z*65*progress,315+b*22-z*94,z+b*.01))
        else:
            projected.append((370+a*90,300+(b*.45-z*.89)*90,b*.89+z*.45))
    # Draw far bonds/atoms first; all connectivity stays unchanged while viewing.
    for i,j in sorted(bonds,key=lambda e:sum(projected[k][2] for k in e)):
        p,q = projected[i],projected[j]
        color = '#ed9d36' if kind == 'diamond' and 0 in (i,j) else '#7c9ab4'
        pen.line((*p[:2],*q[:2]),fill=color,width=5)
    for i,(x,y,z) in sorted(enumerate(projected),key=lambda item:item[1][2]):
        color = '#e89d32' if kind == 'diamond' and i == 0 else '#2385bd'
        radius = 11 if kind != 'fullerene' else 8
        pen.ellipse((x-radius,y-radius,x+radius,y+radius),fill=color,outline='white',width=2)
    lines = {
        'diamond':['金刚石 · 局部网络','中心碳连接4个碳','空间共价网络','与高硬度联系','转动用于观察结构'],
        'graphite':['石墨 · 局部层状','层内六元环网络','观察层间相对滑动','层内共价键保持','仅画三层 / 实际多层'],
        'fullerene':['C60 · 闭合碳笼','60个碳 / 90条棱','每个碳连接3个碳','12个五元环','20个六元环']}[kind]
    for i,line in enumerate(lines):text(700,135+i*62,line,'#18324c' if i == 0 else '#60758a',25)
    text(30,545,{'diamond':'局部截取：边缘原子未画出完整配位；不是独立金刚石分子。',
                 'graphite':'层间滑动是性质关联示意；不表示层内共价键断裂或实时力学仿真。',
                 'fullerene':'理想化球棍模型；棱线用于表达连接关系，不标定真实键长。'}[kind],'#60758a',22)
    text(30,590,'程序绘制 · 教学结构示意，非真实原子外观或比例；不是不同单质间的转化','#60758a',20)
