"""Trusted postprocessing in an isolated container; no model code is loaded."""
import base64
import io
import json
import subprocess
import zipfile
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont

settings=json.loads(Path('/input/settings.json').read_text())
source='/media/Lesson.mp4'
if settings['cloud']:
    overlay=Image.new('RGBA',(1280,720),(0,0,0,0))
    draw=ImageDraw.Draw(overlay)
    draw.rectangle((0,568,1280,720),fill='#0B1220')
    fontpath='/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc'
    for line,y,size in [(settings['caption'],585,25),(settings['formula'],628,28),('情境素材 · 非精确仿真 · 请核验运动关系',680,18)]:
        if line:
            font=ImageFont.truetype(fontpath,size)
            if draw.textlength(line,font=font)>1216:
                raise ValueError('标注过长，无法清晰呈现')
            draw.text((32,y),line,font=font,fill='#F4F7FC')
    overlay.save('/tmp/overlay.png')
    subprocess.run(['ffmpeg','-v','error','-y','-filter_threads','1','-filter_complex_threads','1','-i',source,'-i','/tmp/overlay.png','-filter_complex',
        '[0:v]scale=1280:568:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(568-ih)/2:color=0x0B1220[v];[v][1:v]overlay=0:0',
        '-an','-r','30','-c:v','libx264','-threads','1','-pix_fmt','yuv420p','-t',str(settings['duration']),'/tmp/annotated.mp4'],check=True)
    source='/tmp/annotated.mp4'
subprocess.run(['ffmpeg','-v','error','-xerror','-i',source,'-map','0:v:0','-f','null','-'],check=True)
metadata=json.loads(subprocess.check_output(['ffprobe','-v','error','-show_entries','stream=codec_name,codec_type,width,height,avg_frame_rate:format=duration','-of','json',source]))
duration=float(metadata['format']['duration'])
if abs(duration-settings['duration'])>1 or len(metadata['streams'])!=1:
    raise ValueError('成片时长与分镜不一致或含未批准音轨')
stream=metadata['streams'][0]
if stream['codec_name']!='h264' or stream['height']!=720 or stream['width']!=1280 or stream['avg_frame_rate']!='30/1':
    raise ValueError('成片不是1280x720/30fps H.264')
archive=io.BytesIO()
with zipfile.ZipFile(archive,'w',compression=zipfile.ZIP_STORED) as output:
    for i,second in enumerate(settings['times']):
        frame=subprocess.check_output(['ffmpeg','-v','error','-ss',str(min(second,duration-.1)),'-i',source,'-frames:v','1','-vf','scale=960:540','-f','image2pipe','-vcodec','mjpeg','-threads','1','-'])
        if len(frame)<1000: raise ValueError('关键帧不可读取')
        output.writestr(f'frame-{i}.jpg',frame)
    output.writestr('metadata.json',json.dumps(metadata))
    if settings['cloud']: output.write(source,'Lesson.mp4')
print(base64.b64encode(archive.getvalue()).decode())
