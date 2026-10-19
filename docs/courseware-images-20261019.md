# 课件真实配图 C：本地候选

2026-10-19，Asia/Shanghai。按用户“先实现 C”授权，首期只接入 Wikimedia Commons。复用现有视觉模型、素材存储及课件审核流程，不接入生图模型，不发布正式服务。

## 教师可以操作的流程

- live制作且打开联网检索时，规划/页面模型用 image_need 描述具体教学用途、1–2组检索词及人物、地点、年代或视角限制。后端实际查询Commons，不接受模型提供的图片URL或素材ID。
- 后端筛选明确的CC BY、CC BY-SA、CC0、公有领域元数据，再把候选图片真实字节连同页面用途与来源说明交给已配置的视觉模型。不确定、不相关、许可不明、下载/审核失败的图片均不自动采用。每页最多自动采用一张。
- 成功图片按asset_id进入当前成员私有素材，真实PNG/JPEG/GIF字节复用既有本地/MinIO存储。工作台“手动编辑内容 → 专业图示、配图、课堂图解与表格 → 检索真实配图”可以改用途和检索词，查看候选及作者许可、选图后保存。可移除或上传替换。替换文件清空旧图库出处；教师删除决定在后续自动修复及单页改写中保留。
- 采用图片后继续走原三页预览、剩余页制作、逐页审核及完整交付。修改图片使页面重新待审。PPT备注保留来源页、作者、许可链接、原图和采用图URL、说明、日期、抓取时间与SHA256。画布引用同一素材也保留出处。
- 未找到可用图片或图库/视觉服务不可用时，不伪造图片，不中断有效文字课件，页面保留原因；可以以后重试或自行上传。样例不自动联网；手动按钮明确发起真实外部检索。

## 数据与边界

- POST /api/tasks/{id}/images/search：revision、slide_id、need；仅本人awaiting_preview/awaiting_confirmation/succeeded课件可用。搜索完成后复查版本及状态，候选只能通过原保存接口应用。无认证401，跨企业/成员404，过期409。
- GET /api/media/{asset}/metadata 返回本人素材及来源快照。图片与快照均受企业RLS和成员校验，响应private/no-store。
- 只请求commons.wikimedia.org的API/文件页，图片只从upload.wikimedia.org/wikipedia/commons/下载。HTTPS、既有公网DNS检查、重定向逐次域名校验；不接受浏览器或模型下载地址。
- 每次最多2组检索，每组6条，最多审核3张，教师候选最多2张；总90秒，GET最多20秒，视觉请求最多45秒；图片8MB、边长320–4096、1600万像素，GIF与总导出容量沿既有边界。
- 图片检索不另扣制作积分；视觉调用记入courseware.image.review，live任务复用持久化供应商调用上限。外部视觉费用仍取决于现有供应商。
- 原始出处以media_assets.metadata中的provenance为准；渲染时清除客户端来源快照并从已鉴权素材记录装载，教师不能伪造导出作者/许可。来源记录和相关性审核不构成授权或史实认证。

## 验证记录

先观察无检索路由、真实PPTX缺少来源备注、画布引用丢失出处的失败，再实现对应切片。HTTP集成使用随机隔离数据库，不创建正式收费任务。Commons响应、文字模型和视觉回复fixture均明确为测试替身，图片为程序生成的测试PNG，不冒充真实历史照片。

- 隔离HTTP：认证、企业/成员隔离、源图片字节/来源快照、免费编辑积分不变、无许可/私网地址/私网跳转/不相关/服务不可用拒绝、无视觉配置、坏输入、过期版本，以及真实PPTX中拒绝伪造来源。日志：work/conversation-first/commons-safety-live.log中的TestCoursewareImagesSearchHTTP通过记录（同日志真实联网探针失败，不能合并为全绿）。commons-final.log曾暴露测试间复用同hash素材但临时目录已清理的问题；生成fixture采用不同内容后，commons-delivery.log复测通过。
- 自动生成与关闭联网/图库故障的三页预览均通过：work/conversation-first/commons-generation-fixed.log。
- Vue真实浏览器：检索、鉴权图片预览、选择、查看来源、保存、删除、停止等待后拒绝迟到候选、上传替换清空旧出处。业务API为明确fixture。可运行node scripts/browser-courseware-images.cjs；证据work/commons-images/browser.json、commons-editor.png。
- Python渲染回归222 passed、3 skipped；新增配图和画布来源备注2项通过。Go test ./...、go vet ./...及Vue build/format检查通过。五页自动配图已通过创建→规划→三页预览→逐页审核→生成剩余两页→全套审核→确认→succeeded/settled→真实PPTX下载；五页备注均包含可信作者。自动成功、关闭联网和图库故障三组通过，见work/conversation-first/commons-delivery.log；产物work/commons-images/commons-automatic.pptx。

最后一次安全回归：work/conversation-first/commons-safety-green.log整组通过，包括公有领域元数据中的不安全许可链接拒绝（先红后绿）、鉴权隔离、免费编辑、配图/画布出处及真实PPTX；隔离库已删除。Go test ./...与go vet ./...最终通过；浏览器4组流程、3次保存通过；Vue build和format检查通过。自动交付日志commons-delivery.log三组均通过，随机隔离库已删除。测试临时服务15173/18001已停止。

## 真实服务与未完成验收

本机对Commons的真实HTTP检索返回unavailable；独立curl直连8秒连接超时。work/commons-images/live-probe.json及TestCoursewareCommonsLiveHTTP保留失败，不算外部链路通过。没有拿到真实候选，因此没有完成真实视觉相关性或真实历史课件教学质量验收；部署环境必须能访问commons.wikimedia.org和upload.wikimedia.org，之后用RUN_COMMONS_LIVE=1在隔离库重跑。

本轮复用已有MinIO接口，未单独验收线上MinIO写入。未实现企业PDF/PPTX自动提图、D生图、动画图片镜头动作、孤立素材清理。未提交、推送、部署；正式8080/8000和.env未更改。PowerPoint实际放映及教师课堂仍需人工验收。
