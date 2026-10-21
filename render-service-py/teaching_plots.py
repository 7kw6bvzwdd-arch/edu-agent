"""Fixed scientific plots from validated numbers, never generated Python code.

Plotting guidance: K-Dense-AI/scientific-agent-skills, skills/matplotlib,
commit a1043ef544e3a100c28e70a736b6cf32554d0677. See docs/matplotlib-web.md.
"""
import io
import json
import threading
import textwrap
import numpy as np
from matplotlib.figure import Figure
from matplotlib.backends.backend_agg import FigureCanvasAgg
from matplotlib.font_manager import FontProperties
from pptx.util import Inches
from PIL import Image

# Matplotlib/font caches are process-global; each request still owns its figure.
_plot_lock=threading.Lock()


def function_expression(series):
    if series.kind=='inverse':return f'y = {series.k:g}/x'
    terms=[(series.a,'x²'),(series.b,'x'),(series.c,'')] if series.kind=='quadratic' else [(series.k,'x'),(series.b,'')]
    expression=''
    for coefficient,symbol in terms:
        if not coefficient:continue
        term=('' if symbol and abs(coefficient)==1 else f'{abs(coefficient):g}')+symbol
        expression+=(' - ' if coefficient<0 else ' + ') + term if expression else ('-' if coefficient<0 else '')+term
    return 'y = '+(expression or '0')


def plot_png(data, width, height, colors, font, function=False, show_coordinates=True, dpi=220):
    background,ink,accent,_,soft,muted=colors
    with _plot_lock:
        fig=Figure(figsize=(width,height),dpi=dpi,facecolor='#'+background,layout='constrained')
        FigureCanvasAgg(fig)
        ax=fig.add_subplot(facecolor='#'+background)
        labels=FontProperties(family=font,size=12)
        ax.grid(True,color='#'+soft,linewidth=.7)
        ax.set_axisbelow(True)
        for spine in ax.spines.values():spine.set_color('#'+muted)
        ax.tick_params(colors='#'+ink,labelsize=11)
        if function:
            ax.set_xlim(data.x_min,data.x_max);ax.set_ylim(data.y_min,data.y_max)
            ax.set_aspect('equal',adjustable='box')
            ax.axhline(0,color='#'+muted,linewidth=1)
            ax.axvline(0,color='#'+muted,linewidth=1)
            ax.set_xlabel('x',fontproperties=labels,color='#'+ink)
            ax.set_ylabel('y',fontproperties=labels,color='#'+ink,rotation=0)
            palette=[accent,'D75454','168979','8059B0']
            styles=['-','--','-.',':']
            for i,series in enumerate(data.series):
                xs=np.linspace(data.x_min,data.x_max,1201)
                if series.kind=='quadratic':
                    ys=series.a*xs**2+series.b*xs+series.c
                elif series.kind=='inverse':
                    with np.errstate(divide='ignore',invalid='ignore'):ys=series.k/xs
                    # Keep the two branches disconnected even when the grid misses zero.
                    xs=np.insert(xs,np.searchsorted(xs,0),np.nan)
                    ys=np.insert(ys,np.searchsorted(np.linspace(data.x_min,data.x_max,1201),0),np.nan)
                else:
                    ys=series.k*xs+series.b
                expression=function_expression(series)+('（非二次函数）' if series.kind=='quadratic' and series.a==0 else '')
                ys=np.where((ys>=data.y_min)&(ys<=data.y_max),ys,np.nan)
                ax.plot(xs,ys,color='#'+palette[i],linestyle=styles[i],linewidth=2,label=expression)
                for value in series.mark_x:
                    result=series.value(value)
                    ax.scatter([value],[result],color='#'+palette[i],s=24,zorder=4)
                    if show_coordinates:
                        ax.annotate(f'({value:g}, {result:g})',(value,result),xytext=(6,6),
                                    textcoords='offset points',fontproperties=labels,color='#'+ink,
                                    annotation_clip=True,clip_on=True)
            ax.legend(prop=FontProperties(family=font,size=10),loc='best',labelcolor='#'+ink,
                      facecolor='#'+background,edgecolor='#'+soft)
        else:
            ax.set_xlabel('\n'.join(textwrap.wrap(data.x_label,24)),fontproperties=labels,color='#'+ink)
            ax.set_ylabel('\n'.join(textwrap.wrap(data.y_label,20)),fontproperties=labels,color='#'+ink)
            if data.style=='errorbar':
                ax.errorbar(data.x_values,data.values,yerr=data.y_errors,fmt='o',linestyle='none',
                            color='#'+accent,ecolor='#'+muted,capsize=4,markersize=5)
            else:ax.scatter(data.x_values,data.values,color='#'+accent,s=36)
            ax.margins(.12)
            for axis in (ax.xaxis,ax.yaxis):axis.get_offset_text().set_color('#'+ink)
        for tick in ax.get_xticklabels()+ax.get_yticklabels():tick.set_fontproperties(labels)
        output=io.BytesIO()
        fig.savefig(output,format='png',dpi=dpi,facecolor='#'+background)
        return output.getvalue()


def add_plot(slide,data,colors,font,x,y,w,h,function=False,show_coordinates=True):
    if function and data.animation:
        motion=data.animation;frames=[]
        start=getattr(data.series[motion.series],motion.param)
        for value in np.linspace(start,motion.to,25):
            frame=data.model_copy(deep=True);frame.animation=None
            setattr(frame.series[motion.series],motion.param,float(value))
            png=plot_png(frame,w,h,colors,font,True,show_coordinates,dpi=110)
            with Image.open(io.BytesIO(png)) as im:frames.append(im.convert('RGB'))
        output=io.BytesIO()
        frames[0].save(output,format='GIF',save_all=True,append_images=frames[1:],duration=motion.duration_ms//25,loop=0)
        png=output.getvalue()
    else:png=plot_png(data,w,h,colors,font,function,show_coordinates)
    picture=slide.shapes.add_picture(io.BytesIO(png),Inches(x),Inches(y),width=Inches(w),height=Inches(h))
    picture.name='matplotlib-function-animation' if function and data.animation else 'matplotlib-function-plot' if function else 'matplotlib-data-plot'
    picture._element.nvPicPr.cNvPr.set('descr',json.dumps(data.model_dump(),ensure_ascii=False))
    return picture
