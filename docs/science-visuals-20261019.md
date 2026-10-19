# 通用动画与科学绘图：本地候选验收

2026-10-19，Asia/Shanghai。延续已批准的函数动画、PPT GIF及图片引用基础设施，加入用户最新要求的结构化通用动画与透镜、分子、细胞工具。源码和本地产物尚未提交、推送或部署。

## 可操作流程

课件规划、逐页生成和改写提示词包含同一份绘图契约。模型返回`scientific`数据，程序校验并绘图；有动作时生成GIF，导出时直接嵌入PPTX。教师在原课件工作台点击“手动编辑内容”，可添加通用动画或三类专业图示，修改参数、图形和动作，查看真实PNG/GIF预览，再免费保存并重新导出。参数留在课件JSON和PPT图片描述中；PPT里的动态图是图片，不能在PowerPoint内逐个编辑内部结构。

新增鉴权接口`POST /api/courseware/scientific`，要求`courseware.generate`权限，预览不持久化、不扣积分，响应`no-store`。原课件任务和文件接口继续验证企业、成员与修订。渲染服务内部使用`POST /render/scientific`，不接收模型脚本、SVG、HTML或远程素材URL；通用scene及其对象、动作的未知字段会在Go和Python入口拒绝。

视频的`lesson.explain@1`新增`scientific`组件，复用相同数据和绘图模块。每幕一个科学组件占据绘图区，内部可组合多个图形；`progress`从0至1选择同源PNG帧，暂停、跳转、倒退和MP4导出都按同一时间轴定位。沿用原方案、代表幕、全片确认、每幕一次免费修改和40积分一次结算流程。动画工作台仍通过已有AI局部修改或结构化分镜JSON编辑；课件的图形参数表单用于PPT编辑。

## 数据与范围

通用工具为`scientific.kind="scene"`，对象支持`rect/ellipse/text/path/polygon/arrow`，动作支持`move/follow_path/rotate/scale/show/hide/emphasize`。最多24个对象、48个动作，每段1至8秒；坐标为0至1，x/y表示中心，y向下。路径使用2至32个点，多边形至少3点；沿路径运动按折线弧长加缓动计算。旋转采用像素等比例坐标，正角为顺时针。文字最多60字、22至60号，颜色只接受十六进制色值。

同对象同属性的动作必须按时间声明且不重叠，移动和沿路径共享位置属性。被跟随的路径不允许再移动、旋转或缩放，可显示、隐藏或强调。每帧检查图形和文字边界，越界直接报错，不能悄悄丢图。有动作时生成26帧循环GIF，无动作时为静态图；视频复用26帧，因此长时间动画的运动细腻程度有限。图形为概念示意，不是通用物理仿真或学科正确性证明。

专业工具包括：

- `thin_lens`：理想凸薄透镜近轴模型。输入焦距、物距、物高，由程序计算像距、放大率、实虚像与光路；物距等于焦距时无有限像距，超出画面范围明确提示。可让物距在1至300cm间变化，经过焦点也可绘制。
- `molecule`：用RDKit解析SMILES并绘制二维结构，最多80原子、单一分子。支持羟基、羧基、氨基、羰基和碳碳双键匹配、高亮；损坏结构或不存在的官能团拒绝。名称与结构对应、空间构型和化学反应未自动认证。
- `cell`：典型植物/动物细胞结构，可选择显示、隐藏和高亮结构，并生成渐进高亮GIF。动物细胞不能选择细胞壁、叶绿体和中央液泡。不代表显微照片、真实比例或所有植物细胞。

原有三角形、受力和电阻电路仍是静态专业工具，附加未实现的动画会报错。函数参数动画继续复用上一切片。场景示意不能冒充实测；固定脚本、AI生成内容及测试音分别标注。

## 验证证据

各切片先观察失败，再实现：渲染输入不支持新kind、真实PPT缺少GIF、科学组件离线包不支持、路由把通用矩形描述误判为几何、未知脚本字段被Go丢弃、静态专业工具忽略动画等均已针对根因修正。

- Go全量测试（含真实绘图离线包）和vet通过；Python全量220项通过、3项环境相关跳过，前端生产构建与现有format:check通过。渲染测试覆盖真实PNG、26帧GIF及PPTX，检查薄透镜焦点状态、分子真实官能团、细胞层选择、通用路径动作及备注。缺失引用、重叠动作、越界、脚本字段和不支持的类型拒绝。
- 隔离Postgres的`TestScientificVisualsHTTP`验证401、跨成员/企业编辑404、免费预览/编辑额度不变、未知scene字段400、静态scene以及包含四种图示的真实PPTX下载。
- `TestHTMLScienceWorkflowHTTP`使用固定模型响应和测试音接口，但真实调用绘图服务。覆盖科学组件方案、代表幕、全片交付、免费修改、取消/隔离和并发只结算一次；这不是教师真实审批。
- `TestScienceOfflinePackage`使用四幕固定脚本生成真实离线HTML；浏览器检查四种组件的起终点帧不同、倒退恢复完全一致。实际Vue工作台可增加图形/动作、修改坐标、预览真实GIF并保存；透镜、分子、细胞均可开启动态预览。记录在`work/science-visuals/browser.json`，16次真实绘图预览，无页面异常。
- `work/science-visuals/science.pptx`包含四页、四个GIF，用本机PowerPoint成功打开并导出截图，已人工检查四页布局。`science.html`和`science.mp4`为固定脚本与测试音，47.82秒，1280×720，H264/AAC、1148帧、8条测试音轨，首中末与随机倒退定位、完整编码检查通过。
- 真实`deepseek-flash`试跑“简化传送带分拣”，最终2次模型请求生成三幕58.1秒的结构化作者数据，其中一幕采用通用scene。数据通过Go/Python校验并产出真实GIF、PPTX、离线HTML和MP4。证据为`live-general.html.script.json`、`live-general.html.generation.json`、`live-general.gif`、`live-general.pptx`、`live-general.mp4.json`。此前三次独立试跑失败，分别暴露路由和对象ID提示问题；本轮共8次模型请求。全部使用明确标注的测试音，没有真实TTS或教师课堂质量验收。

真实模型试跑只证明一份模型数据可以进入共同绘图和导出链路，不保证动作时机、教学内容或其他课题都正确，也不等同于完整课件逐页生成的真实模型质量验收。PPT静态截图只显示GIF首帧，桌面PowerPoint放映播放与教师课堂效果仍需人工验收。外部图库、生图模型与费用未接入。

## 部署依赖与复验

需要同时更新Go后端、Vue前端和Python渲染服务；Python增加固定版本RDKit，Docker镜像复制`teaching_plots.py`、`science_visuals.py`和`general_scene.py`。健康检查包含`science_visuals_version=science-visuals-v1`，后端拒绝旧绘图服务。CLI使用`RENDER_SERVICE_URL`，不会读写业务数据库。

```powershell
# backend-go
../work/toolchain/go/bin/go.exe test ./...
../work/toolchain/go/bin/go.exe vet ./...

# render-service-py
../.venv/Scripts/python.exe -m pytest -q

# frontend-vue
npm run build
npm run format:check
```

已启动新本地绘图服务时，设置`SCIENCE_RENDER_URL`后运行`TestScienceOfflinePackage`。隔离数据库设置使用已有脚本，凭据仅注入环境：`scripts/test-workflow.ps1 -Pattern 'TestScientificVisualsHTTP|TestHTMLScienceWorkflowHTTP' -Label science-final`。浏览器脚本为`scripts/browser-science-visuals.cjs`，默认连接15173的候选前端和18001的候选绘图服务，任务API全部mock，只把绘图请求发到本地服务，不操作正式账户。

固定包入口为`--anim-package fixtures/animation-v2-science.json <out.html> tone`，随后运行`node animation-video/export.mjs <out.html> <out.mp4>`。真实模型探针显式使用`--anim-generate <brief.txt> <out.html> tone`，会产生真实模型费用；已有产物可直接检查，无需重复调用。
