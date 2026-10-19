# 网页配图优先，Commons备用

2026-10-19，Asia/Shanghai。按用户“用其他方式，把Commons作为备用方式”修改本地候选。使用现有搜索服务、Go标准库及已安装的x/net解析器，不新增图库账号或依赖。

## 当前行为

- 统一searchCoursewareImages入口同时服务自动制作和教师手动搜索。复用SEARCH_API_URL/SEARCH_API_KEY，搜索返回网页链接；实际访问来源页，提取img/data-src、图片title/alt、figure说明及og:image。相对地址以最终网页URL解析，支持中文网页编码。模型只描述检索词、用途与限制，不提供下载URL。
- 对明确关联到figure图片的rel=license声明，识别CC BY、CC BY-SA、CC0、公有领域链接；一般网页页脚版权不当成照片授权。保存的是来源声明，不是法律认证。无声明图片标为pending_confirmation。每次最多4页、8次图片下载、3次网页候选视觉筛选。
- 视觉审核读取真实图片字节。明确开放许可且相关的网页图可自动采用；没有可直接采用的网页图时查询备用Commons。自动流程遇到许可待确认图保留文字课件与needs_confirmation状态；手动流程提供相关候选供教师确认。
- 教师填写5–300字使用权限依据，通过POST /api/tasks/{id}/images/confirm生成独立素材引用，保留原始出处、原始hash、教师声明和确认时间。不修改原待确认素材，不伪称已取得平台授权。确认后仍需选择图片、保存并重新审核页面。
- pending_confirmation素材可鉴权预览，不能通过图片或画布引用绕过确认进入导出；渲染边界从素材记录重载出处，拒绝客户端伪造。来源、原图、作者、许可、SHA256、抓取时间和教师声明进入真实PPTX备注。
- 沿用任务认证/成员隔离、任务状态及前后版本复查。图片搜索、选择与确认不另扣制作积分，搜索和视觉服务仍会产生供应商用量；视觉调用沿原日志和调用上限。
- 抓取沿现有SSRF安全公网DNS固定地址拨号；初始URL及每次跳转检查HTTP(S)、80/443、无用户信息、无私网IP。网页2MB、图片8MB及现有尺寸/帧数限制。搜索服务不转发任务认证头。
- 网页部分最多65秒、备用Commons总预算25秒，单次抓取网页/图片7秒，整体90秒；前端95秒超时并支持取消，权限确认20秒超时。没有图片仍保留文字课件。

## 验证与边界

- 先修改真实PPTX渲染测试，观察web_page来源被422拒绝，再接入网页来源导出，4项通过。Python整体224 passed、3 skipped。
- 新增HTTP测试先复现只查Commons、没有网页图的失败。网页优先、许可待确认、认证/隔离、版本过期、权限确认和积分不变的初步测试通过；最终含真实PPTX、Commons成功回退和未确认引用拒绝的结果见work/web-images/web-final.log。
- 既有Commons安全回归见work/web-images/commons-final.log；自动五页网页配图从创建到审核、扩展、确认、结算和真实PPTX见work/web-images/automatic-final.log。外部搜索/页面/模型为显式fixture，不冒充真实模型。
- Vue浏览器5组操作、3次保存、1次权限确认通过；证据work/web-images/browser.json。浏览器业务API为fixture，真实Vue组件与图片预览/交互。
- 真实搜索探针work/web-images/search-live.json返回HTTP200（约2.4秒）。实际HTTP链路work/web-images/live2.log已真实搜索、下载人民网图片并经现有视觉模型筛选，返回web_page、pending_confirmation；结果work/web-images/live-http.json。Commons仍7秒连接超时，不阻断网页候选。候选未确认权限、未应用于真实课件，也不代表真实教学质量完成验收。
- 首次live.log未找到候选，后续修正无效小图计数、网页编码与图片title后live2.log通过；不能把首轮失败描述为全程成功。

本轮不绕过登录、验证码、防盗链；仅解析服务端返回的HTML，纯JS页面、背景图、WebP、未提供图片的网页可能无候选。来源缺作者时如实标为未标明。未知许可不会自动进入课件。企业知识库PDF/PPTX提图、AI生图、对象存储独立验收、孤立候选清理不在本轮。没有修改.env、系统代理或正式8080/8000服务，没有提交/推送/部署。

## 最终结果

web-final.log整组PASS（约81秒）：网页优先、Commons成功备用、未知权限导出拒绝、教师声明后真实PPTX、401/404/409与积分不变。commons-final.log整组PASS（约77秒）：既有Commons安全与来源导出。automatic-final.log的web-primary五页流程PASS（约203秒）：真实任务创建、三页预览审核、扩展、五页审核、确认、succeeded/settled及真实PPTX下载。随机隔离库均删除；web-http.pptx和web-confirmed-http.pptx仅使用程序生成的测试图。

较早的web-render.log混合运行出现not_found/unavailable状态兼容回归及整组8分钟时限超时，不计为通过；修正后按独立隔离库运行上述三组全部通过。最终go test ./...、go vet ./...、Vue build与组件Prettier检查通过；Python224 passed、3 skipped。浏览器截图已检查网页优先与权限声明显示。
