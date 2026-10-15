# 首页需求对话改版

范围：HomeWorkspace.vue、首页外壳样式；Operate 模式。
用户于2026-10-15确认优先突出新课需求对话，保留现有生成流程。

## Direction contract

THESIS: 打开首页即可沟通教学需求；输入与确认优先，作品和工具降为辅助入口。

OWN-WORLD: 用户指定既有紫水晶主题。直接继承 amethyst.css 的深紫侧栏、浅紫底色、白色工作区、紫色强调与中文系统字体，不另建配色。圆角8–16px。

STORY: 打开即描述课题，流式交流并查看分块需求表格；可继续补充，也可直接制作。资料与教学设置默认折叠，之后进入已有的三页预览和教师审核流程。

FIRST VIEWPORT: 首页保留深紫侧栏，参考用户提供页面，以居中问候与宽大输入框作为起点；白色主对话使用整个内容区宽度，删除重复标题、步骤条及右侧提示/灵感栏，真实最近作品横排在下方。有对话后问候标题自动收紧，为消息记录腾出空间，发送和直接制作按钮在桌面及手机测试尺寸内可见。审核页保留同一外壳，左侧缩略图、中间单页大幅真实PPT、右侧审核/助手选项卡，底部保留明确的交付门槛。移动端缩略图横排，预览先于辅助区。

FORM: 用户明确指定紫水晶主题与常见PPT编辑布局。首页保留对话主区，摘要可按需展开修改，直接制作无需额外摘要确认；审核页的缩略图切换同步大幅预览和当前页审核，单页修改不混淆全套交付。seed: none (unavailable)

2026-10-15补充：最新有效回复的需求表格使用原生输入框、下拉框与多行文本直接修改，覆盖课题、学科、年级、课时、教材、学生基础、目标、详略八项。修改课题保留聊天记录；旧表格只读，回复中锁定编辑；10–180整数课时等校验失败时阻止发送/制作。手动摘要保留，实际请求总是附带当前教学上下文。验证见 scripts/browser-conversation.cjs 与 scripts/browser-home-review.cjs，截图位于 work/conversation-first/browser。

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
