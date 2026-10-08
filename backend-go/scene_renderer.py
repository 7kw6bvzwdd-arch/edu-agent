"""Trusted scene interpreter. Model JSON supplies data only; no generated Python."""
import json
from manim import *

PALETTE = {'blue':'#66C7FF','green':'#75DEB1','orange':'#FFC179','purple':'#BFA5FF'}


def text(value, size=28, width=12):
    obj = Text(value, font='Noto Sans CJK SC', font_size=size, color='#F4F7FC')
    if obj.width > width:
        scale = width / obj.width
        if size * scale < 18:
            raise ValueError('ANIMATION_LAYOUT: 文字过密，请缩短文字后重新确认')
        obj.scale(scale)
    return obj


def element(el):
    coords = el['points']
    kind = el['kind']
    if kind in ('right_triangle', 'rectangle'):
        x,y=coords[0];w,h=el['size'];angle=el.get('angle',0)*PI/180
        local=[(0,0),(w,0)]+([(w,h)] if kind=='rectangle' else [])+[(0,h)]
        coords=[(x+u*np.cos(angle)-v*np.sin(angle),y+u*np.sin(angle)+v*np.cos(angle)) for u,v in local]
        kind='polygon'
    points = [np.array([p[0]-1.8, p[1]+0.4, 0]) for p in coords]
    color = PALETTE[el['color']]
    if kind == 'polygon':
        obj = Polygon(*points, color=color, fill_opacity=.16, stroke_width=3)
    elif kind == 'circle':
        obj = Circle(radius=el['radius'], color=color, fill_opacity=.16, stroke_width=3).move_to(points[0])
    elif kind in ('line', 'arrow'):
        obj = (Arrow if kind == 'arrow' else Line)(points[0], points[1], buff=0, color=color, stroke_width=4)
    else:
        obj = VMobject(color=color, stroke_width=4).set_points_as_corners(points)
    return obj


class Lesson(Scene):
    def construct(self):
        self.camera.background_color = '#0B1220'
        slides = PLAN['slides']
        # One scale for the whole lesson: scene fitting must not erase a change in force/size.
        scale=3.0
        for slide in slides:
            elements=slide['scene']['elements']
            objects=[element(el) for el in elements]
            bounds=VGroup(*[obj.copy() for obj in objects],*[obj.copy().shift(np.array([*el['move'],0])) for obj,el in zip(objects,elements)])
            scale=min(scale,7.4/max(bounds.width,.1),3.6/max(bounds.height,.1))
        for index, slide in enumerate(slides):
            scene = slide['scene']
            title = text(slide['title'], 32).move_to([0, 3.35, 0])
            badge = text(f'{index+1:02d} / {len(slides):02d}  ·  '+{'proof':'拼图几何校验通过 · 推理仍需核验','numeric':'数值验证 · 不等同一般性证明','illustration':'教学示意 · 条件需核验'}[scene['evidence']], 20).move_to([0,2.7,0])
            board = RoundedRectangle(width=8.1,height=4.2,corner_radius=.15,color='#2D405B',fill_color='#101D30',fill_opacity=1).move_to([-1.8,.4,0])
            panel = RoundedRectangle(width=3.8,height=4.2,corner_radius=.15,color='#2D405B',fill_color='#101D30',fill_opacity=1).move_to([4.35,.4,0])
            objects = [element(el) for el in scene['elements']]
            # Fit the union of start/end bounds with one scale: preserve relative sizes and motion.
            bounds=VGroup(*[obj.copy() for obj in objects],*[obj.copy().shift(np.array([*el['move'],0])) for obj,el in zip(objects,scene['elements'])])
            center=bounds.get_center()
            for obj in objects:
                obj.shift(-center).scale(scale,about_point=ORIGIN).shift(np.array([-1.8,.4,0]))
            legends = []
            for i,el in enumerate(scene['elements']):
                dot = Dot(radius=.055,color=PALETTE[el['color']]).move_to([2.75,1.95-i*.61,0])
                label = text(el['label'] or el['id'],24,3.0).next_to(dot,RIGHT,buff=.12)
                legends.append(VGroup(dot,label))
            caption = scene['caption']
            if len(caption)>24:
                caption=caption[:24]+'\n'+caption[24:]
            caption_obj=text(caption,27,12).move_to([0,-2.45,0])
            formula=text(scene['formula'],30,12).move_to([0,-3.3,0]) if scene['formula'] else VGroup()
            duration = scene['duration_sec']
            intro = min(1.5, duration*.34)
            action = min(1.2, duration*.3)
            outro = .2
            caption_time = .2
            formula_time = .2 if scene['formula'] else 0
            self.add(title,badge,board,panel)
            reveals = [AnimationGroup(FadeIn(obj),FadeIn(legend)) for obj,legend in zip(objects,legends)]
            self.play(LaggedStart(*reveals,lag_ratio=.4),run_time=intro)
            self.play(FadeIn(caption_obj),run_time=caption_time)
            moves=[obj.animate.shift(np.array([*el['move'],0])*scale) for obj,el in zip(objects,scene['elements']) if any(el['move'])]
            if moves:
                self.play(*moves,run_time=action,rate_func=smooth)
            else:
                self.play(Indicate(objects[-1],scale_factor=1.04,color=PALETTE[scene['elements'][-1]['color']]),run_time=action)
            if scene['formula']:
                self.play(FadeIn(formula),run_time=formula_time)
            self.wait(duration-intro-caption_time-action-formula_time-outro)
            self.play(FadeOut(VGroup(title,badge,board,panel,*legends,caption_obj,formula,*objects)),run_time=outro)
