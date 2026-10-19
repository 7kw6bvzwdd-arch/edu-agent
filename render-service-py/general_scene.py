"""Declarative 2D scene: bounded shapes and actions, no code, URLs or SVG input."""
import math
from typing import Literal

from PIL import Image, ImageDraw
from pydantic import BaseModel, ConfigDict, Field, FiniteFloat, model_validator


class SceneObject(BaseModel):
    model_config=ConfigDict(extra='forbid')
    id:str=Field(pattern=r'^[a-z][a-z0-9_-]{0,31}$')
    type:Literal['rect','ellipse','text','path','polygon','arrow']
    x:FiniteFloat=Field(default=.5,ge=0,le=1)
    y:FiniteFloat=Field(default=.5,ge=0,le=1)
    width:FiniteFloat=Field(default=.12,ge=.005,le=1)
    height:FiniteFloat=Field(default=.12,ge=.005,le=1)
    color:str=Field(default='#1767bf',pattern=r'^#[0-9a-fA-F]{6}$')
    fill:str=Field(default='',pattern=r'^(#[0-9a-fA-F]{6})?$')
    text:str=Field(default='',max_length=60)
    font_size:int=Field(default=26,ge=22,le=60)
    stroke_width:FiniteFloat=Field(default=3,ge=1,le=8)
    points:list[tuple[FiniteFloat,FiniteFloat]]=Field(default_factory=list,max_length=32)

    @model_validator(mode='after')
    def check(self):
        if self.type=='text' and (not self.text.strip() or '\n' in self.text):raise ValueError('Label must be a short single line')
        if self.type in ('path','polygon','arrow'):
            if len(self.points)<(3 if self.type=='polygon' else 2) or any(not 0<=v<=1 for point in self.points for v in point):raise ValueError('Invalid point path')
            if sum(math.dist(a,b) for a,b in zip(self.points,self.points[1:]))<.001:raise ValueError('Empty path')
        elif self.points:raise ValueError('Only paths accept points')
        return self


class SceneAction(BaseModel):
    model_config=ConfigDict(extra='forbid')
    object:str=Field(pattern=r'^[a-z][a-z0-9_-]{0,31}$')
    op:Literal['move','follow_path','rotate','scale','show','hide','emphasize']
    start_ms:int=Field(default=0,ge=0,le=8000)
    duration_ms:int=Field(default=1000,ge=100,le=8000)
    x:FiniteFloat|None=Field(default=None,ge=0,le=1)
    y:FiniteFloat|None=Field(default=None,ge=0,le=1)
    to:FiniteFloat|None=None
    path:str=''

    @model_validator(mode='after')
    def check(self):
        if self.op=='move':
            if self.x is None or self.y is None:raise ValueError('Move needs a destination')
        elif self.x is not None or self.y is not None:raise ValueError('Destination only belongs to move')
        if self.op in ('rotate','scale'):
            if self.to is None or not(-360<=self.to<=360 if self.op=='rotate' else .2<=self.to<=3):raise ValueError('Invalid transform')
        elif self.to is not None:raise ValueError('Target only belongs to rotation or scale')
        if (self.op=='follow_path')!=bool(self.path):raise ValueError('Path reference belongs to follow_path')
        return self


class DrawingScene(BaseModel):
    model_config=ConfigDict(extra='forbid')
    duration_ms:int=Field(default=3000,ge=1000,le=8000)
    objects:list[SceneObject]=Field(min_length=1,max_length=24)
    actions:list[SceneAction]=Field(default_factory=list,max_length=48)

    @model_validator(mode='after')
    def check(self):
        objects={o.id:o for o in self.objects}
        if len(objects)!=len(self.objects):raise ValueError('Duplicate object IDs')
        routes={a.path for a in self.actions if a.op=='follow_path'}
        tracks={}
        for a in self.actions:
            if a.object not in objects or a.start_ms+a.duration_ms>self.duration_ms:raise ValueError('Unknown object or action outside scene')
            if a.op=='follow_path' and (a.path not in objects or objects[a.path].type!='path' or a.object==a.path):raise ValueError('Invalid route')
            if a.object in routes and a.op in ('move','follow_path','rotate','scale'):raise ValueError('Followed paths must keep their geometry')
            track=(a.object,'position' if a.op in ('move','follow_path') else 'opacity' if a.op in ('show','hide') else a.op)
            if a.start_ms<tracks.get(track,0):raise ValueError('Overlapping or unordered actions on same property')
            tracks[track]=a.start_ms+a.duration_ms
        return self


def path_at(points,fraction):
    lengths=[math.dist(a,b) for a,b in zip(points,points[1:])];remaining=sum(lengths)*fraction
    for a,b,length in zip(points,points[1:],lengths):
        if remaining<=length and length:
            return (a[0]+(b[0]-a[0])*remaining/length,a[1]+(b[1]-a[1])*remaining/length)
        remaining-=length
    return points[-1]


def draw_scene(image,scene,progress,font):
    w,h=940,470;left,top=30,85;t=progress*scene.duration_ms
    objects={o.id:o for o in scene.objects};states={}
    for o in scene.objects:
        center=(sum(p[0] for p in o.points)/len(o.points),sum(p[1] for p in o.points)/len(o.points)) if o.points else (o.x,o.y)
        states[o.id]=dict(x=center[0],y=center[1],angle=0.,scale=1.,opacity=1.,emphasis=0.,center=center)
    seen=set()
    for a in scene.actions:
        if a.op in ('show','hide') and a.object not in seen:
            states[a.object]['opacity']=0. if a.op=='show' else 1.;seen.add(a.object)
    for a in scene.actions:
        if t<a.start_ms:continue
        state=states[a.object];u=min(1,(t-a.start_ms)/a.duration_ms);u=u*u*(3-2*u)
        if a.op=='move':
            state['x']+=(a.x-state['x'])*u;state['y']+=(a.y-state['y'])*u
        elif a.op=='follow_path':state['x'],state['y']=path_at(objects[a.path].points,u)
        elif a.op in ('rotate','scale'):
            key='angle' if a.op=='rotate' else 'scale';state[key]+=(a.to-state[key])*u
        elif a.op in ('show','hide'):state['opacity']+=((1 if a.op=='show' else 0)-state['opacity'])*u
        elif a.op=='emphasize' and t<a.start_ms+a.duration_ms:state['emphasis']=math.sin(math.pi*u)
    for o in scene.objects:
        state=states[o.id];layer=Image.new('RGBA',image.size);pen=ImageDraw.Draw(layer)
        cx,cy=state['center'];angle=math.radians(state['angle']);c,s=math.cos(angle),math.sin(angle)
        def xy(point):
            # Rotation uses isotropic pixel coordinates, preserving angles.
            x,y=(point[0]-cx)*w*state['scale'],(point[1]-cy)*h*state['scale']
            return (left+state['x']*w+x*c-y*s,top+state['y']*h+x*s+y*c)
        color='#bc582b' if state['emphasis']>.05 else o.color
        stroke=round(o.stroke_width+4*state['emphasis'])
        if o.type=='text':
            face=font(o.font_size);bounds=face.getbbox(o.text);tw,th=bounds[2]-bounds[0]+8,bounds[3]-bounds[1]+8
            label=Image.new('RGBA',(tw,th));ImageDraw.Draw(label).text((4-bounds[0],4-bounds[1]),o.text,fill=color,font=face)
            label=label.resize((max(1,round(tw*state['scale'])),max(1,round(th*state['scale'])))).rotate(-state['angle'],expand=True,resample=Image.Resampling.BICUBIC)
            x,y=xy((cx,cy));box=(x-label.width/2,y-label.height/2,x+label.width/2,y+label.height/2)
            layer.alpha_composite(label,(round(box[0]),round(box[1])))
        else:
            if o.points:points=list(o.points)
            elif o.type=='ellipse':points=[(cx+o.width/2*math.cos(i*math.tau/64),cy+o.height/2*math.sin(i*math.tau/64)) for i in range(64)]
            else:points=[(cx-o.width/2,cy-o.height/2),(cx+o.width/2,cy-o.height/2),(cx+o.width/2,cy+o.height/2),(cx-o.width/2,cy+o.height/2)]
            points=[xy(p) for p in points]
            if o.type in ('rect','ellipse','polygon'):pen.polygon(points,fill=o.fill or None,outline=color,width=stroke)
            else:
                pen.line(points,fill=color,width=stroke,joint='curve')
                if o.type=='arrow':
                    a,b=points[-2:];r=math.atan2(b[1]-a[1],b[0]-a[0]);length=12+stroke
                    tip=[b,(b[0]-length*math.cos(r-.45),b[1]-length*math.sin(r-.45)),(b[0]-length*math.cos(r+.45),b[1]-length*math.sin(r+.45))]
                    pen.polygon(tip,fill=color);points+=tip
            box=(min(p[0] for p in points),min(p[1] for p in points),max(p[0] for p in points),max(p[1] for p in points))
        if box[0]<left-1 or box[1]<top-1 or box[2]>left+w+1 or box[3]>top+h+1:raise ValueError('Object or label leaves drawing area')
        layer.putalpha(layer.getchannel('A').point(lambda a:round(a*state['opacity'])))
        image.paste(layer,(0,0),layer)
