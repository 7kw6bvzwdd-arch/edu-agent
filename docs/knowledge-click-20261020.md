# 知识库提图与PPT原生点击动画

2026-10-20（Asia/Shanghai）。按用户最新指示取消成员长期教学偏好；本轮实施知识库PDF/PPTX自动提图、复用与PPT函数原生点击动画。当前为本地源码及隔离候选验收，未切换正式8080/8000。

## 可操作功能

- 上传PDF/PPTX随原文字解析提取内嵌位图（含PPT组合中的图片）；按文档、文件页码和上下文保存，使用已有私有素材与本地/MinIO存储接口，不向课件JSON写入图片二进制。知识库显示图片数量、鉴权预览、页码与提取警告；保留原文件的旧资料可点击重新解析。
- 企业知识库继续为企业内共享资料，跨企业拒绝。共享文档图片复用到课件或预览时生成/复用当前成员私有素材，其他成员不能直接读取该私有引用。已交付作品的素材不随原文删除而删除。
- 生成和手动换图共用入口，启用知识库或选中教材后先匹配文档图片，使用现有视觉模型筛选用途相关性；UseWeb=false时不访问公开网页。允许联网后才依次尝试网页、备用Commons；所有来源共用最多3次视觉审核。没有合适图片保留原内容。相关性不是事实或使用许可认证，未知作者/许可如实记录。
- 本轮不增加OCR、任意矢量对象自动组合或从纯文字生成图片；仅提取原文实际内嵌的位图。单文档最多24图，每页PDF最多检查12个图片对象，单图8MB、1600万源像素，提取图最长边2048，总量12MB；达到上限给出警告。GIF来源按首帧作静态资料图处理。
- graph.reveal_on_click=true使用原生可编辑曲线组，一次、二次和反比例沿用现有函数定义；坐标轴始终显示，最多4条曲线按series顺序每次点击出现一条，标记与图例一同出现。编辑区可开关，与参数GIF和Matplotlib互斥；模型提示词可选择该方式，不执行模型代码。
- 网页、截图和静态PPT导图显示所有曲线；PowerPoint放映使用点击时间线。旧渲染服务缺少native-click-v1时后端拒绝静默导出，参数保留。

## 验收与复验

- 先记录原生导出缺少分组/时间线、错误组合未拒绝及PDF/PPTX无提图的失败用例，再实现；真实隔离HTTP首轮记录图片入口404。
- Python渲染回归：236通过、3项环境相关跳过；覆盖实际PDF/PPTX提图、组合图片、图片页、容量警告、真实PPTX曲线组/点击时间线、GIF与原有出处导出。Go本地可运行测试通过（依赖数据库的用例在无TEST_DATABASE_URL时跳过）。
- 隔离PostgreSQL与18003候选渲染通过PDF/PPTX上传→解析→成员私有引用→关闭联网的视觉筛选→实际课件保存/下载→备注文档及页码链路；检查401、跨企业404、跨成员私有素材404、删除来源保留交付素材及额度不变。视觉服务是显式固定fixture，不冒充真实模型试跑。
- 自动生成完整链路通过：未指定教材、启用企业知识库且关闭网页联网，上传PDF后创建任务→3页预览→扩展→逐页确认→交付结算→下载真实PPTX，核对自动采用知识库配图及备注中的原文档ID和第1页出处。模型与视觉均为显式固定fixture；此结果验证工程链路，不代表真实模型教学质量。旧资料重新解析亦通过排队/重复请求校验及原素材ID复用。
- 原生动画隔离HTTP通过保存设置、错误模式400、任务所有权隔离、实际PPTX下载内两条clickEffect及额度不变；旧渲染能力检查通过。旧知识库上传/检索/失败重试隔离回归通过。
- Vue浏览器固定API验收通过资料图片预览与页码、旧资料重新解析入口、关闭联网仍匹配知识库且allow_web=false、自动保存引用，以及点击/GIF/高清模式互斥和设置保存。没有访问或修改正式任务。
- 本机桌面PowerPoint已打开固定样例，识别两个原生Appear效果及OnPageClick触发（COM的EffectType=1、TriggerType=1），成功导出截图；未冒充教师实际课堂放映验收。

证据在work/knowledge-click-20261020、work/conversation-first/knowledge-images-red.log、knowledge-images-green.log、knowledge-click-http.log、knowledge-click-final.log及knowledge-auto-generation.log。最后两组隔离验收均通过并清理测试数据库；最新Go本地可运行测试与浏览器验收再次通过。固定样例构造脚本为scripts/knowledge-visual-fixtures.py；浏览器入口scripts/browser-knowledge-click.cjs。

复验：根目录 `.venv/Scripts/python.exe -m pytest render-service-py -q`；frontend-vue中 `npm run build`；backend-go中 `go test .`。隔离HTTP使用scripts/test-workflow.ps1的RenderURL参数，指向新版候选渲染，设置KNOWLEDGE_IMAGE_FIXTURES、KNOWLEDGE_IMAGE_RENDER_URL、NATIVE_CLICK_RENDER_URL后运行TestKnowledgeFiguresOfflineHTTP、TestNativeClickCoursewareEditHTTP、TestNativeClickRejectsOldRenderer及TestKnowledgeHTTPIsolationAndRetry。

自动生成复验：设置KNOWLEDGE_IMAGE_FIXTURES与COMMONS_TEST_RENDER_URL，运行 `scripts/test-workflow.ps1 -Pattern '^TestCoursewareImageNeedGenerationHTTP/offline-knowledge$' -Label knowledge-auto-generation -RenderURL http://127.0.0.1:18003`。

本文件最初这组知识库/点击工程验收未调用收费真实模型；后续授权的验收闭环已经运行真实模型、122页实际教材及本机PowerPoint放映取证，结果见docs/acceptance-closure-20261020.md。不能沿用本句声称后续没有模型费用。未部署、推送或新增tag；MinIO本轮独立读写、其他Office及真实课堂仍需后续验收。
