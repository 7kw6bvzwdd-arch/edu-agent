# 函数动画与图片引用：本地候选

2026-10-19，Asia/Shanghai。实施范围为用户确认的 A、B 和图片引用基础设施；未发布到正式服务，未接入外部图库或生图模型。

## 可操作能力

- `func.params@1` 新增 `function` 对象，`kind` 与课件 `graph.series` 一致：`linear{k,b}`、`quadratic{a,b,c}`、`inverse{k}`。旧 `line` 对象和作品继续支持。函数值校验复用课件计算，课件预览和离线播放器共享 `animplayer/functions.js`。模型提供系数与动作，程序计算曲线、顶点、对称轴和坐标轴交点。二次项经过零时明确提示非二次函数；反比例系数及标注点不得连续跨过零。
- 课件函数编辑区可开启“导出参数变化GIF”，选择曲线、系数、目标值和1–8秒时长。25帧动图由现有 matplotlib/Pillow 生成并嵌入真实 PPTX。动图的编辑和截图显示首帧；本轮没有新增原生点击出现动画，原有静态原生曲线仍可编辑。函数图可与教学表格同页；专业图示仍保留独立绘图区限制。
- 新上传走 `POST /api/media`，课件的 `picture` 和画布图片保存 `asset_id`，通过鉴权的 `GET /api/media/{asset}` 查看。素材不可原地覆盖，SHA-256、格式、大小和尺寸记录在具有企业 RLS 的 `media_assets` 表。文件复用现有本地/MinIO存储；渲染前才解析为内部传输数据，不把解析后的二进制写回课件。
- PNG/JPEG/GIF 单文件8MB、边长32–4096像素且最多1600万像素；GIF最多120帧和8000万总像素；一次导出引用图片总量最多64MB。旧内联图片保持兼容，其旧容量边界保留；新引用不受旧180KB/整套1.2MB内联限制。保留素材原始字节和GIF帧，图片说明、出处写入PPT备注。
- 动画工作台的PNG/JPEG上传也保存引用，生成离线HTML时才解析并内嵌。动画拒绝GIF配图，避免其独立播放时钟影响可重复的时间轴和MP4定位；GIF用于PPT放映。
- 每页规划要求明确视觉方案；提示词允许互补组合，并保留教学用途、来源与练习不泄露答案等要求。文字模型仍不读取图片，不能声称完成识图或相关性核验。

## 验证

先观察到函数对象打包失败、PPT导出缺少GIF、引用图片渲染失败、上传接口404、函数图与表格组合422，再分别实现对应切片。

本地验证包括 Go 测试、渲染服务213项通过（3项环境相关跳过）、前端生产构建、49项动画工作台浏览器检查、画布上传/替换/比例/保存/恢复检查，以及浏览器中曲线、顶点、对称轴、零系数提示、反比例断支和倒退定位检查。

隔离数据库HTTP验收覆盖上传/读取认证、跨成员及跨企业拒绝、损坏素材拒绝、图片引用随课件编辑保存和真实PPTX下载、外来图片引用拒绝、免费编辑额度不变。动画引用配图沿原有方案、代表幕、交付和40积分一次结算流程验证；视频导出恢复边界沿既有接口验证。

`work/function-families/functions.pptx` 为3页固定样例，包含二次函数参数变化GIF与对照表、可编辑的静态二次函数图、反比例函数参数变化GIF。同目录的 `functions.html`、`functions.mp4` 和 `references.pptx` 也可检查。函数视频为31.38秒、1280×720、H264/AAC，保留全部6段测试音轨，明确标注固定样例及测试音；它不代表真实模型教学质量或真实旁白验收。MP4检查报告记录在 `functions.mp4.json`，浏览器检查与画面同目录保存。

本轮复用对象存储接口，未单独对线上MinIO做读写验收。已用本机PowerPoint导出样例页截图并检查函数图与表格组合；桌面PowerPoint实际放映及教师课堂验收仍待完成。素材引用暂保留以支持历史修订，未新增孤立素材清理。C的图库来源、许可快照与相关性审核，D的生图模型和费用，以及动画图片镜头动作均未接入。

## 复验入口

在 `backend-go` 执行 `go test .`。渲染服务目录执行 `python -m pytest -q`，前端执行 `npm run build`。

使用现有隔离库脚本并设置 `MEDIA_TEST_RENDER_URL` 指向新版本地渲染服务，运行 `TestMediaAssetsIsolationHTTP|TestHTMLIllustrationPictureHTTP|TestAnimationExportRecoveryHTTP`。服务凭据通过环境注入，不写入文档或日志。

使用 `--anim-package fixtures/animation-v2-functions.json <out.html> tone` 生成固定测试包；运行 `node scripts/browser-functions.cjs <out.html>` 与 `node animation-video/export.mjs <out.html> <out.mp4>` 验证播放器和真实视频。正式服务更新时需要同时带入后端数据库迁移、前端、渲染服务；旧版渲染服务缺少动图能力时后端明确拒绝，保留参数。
