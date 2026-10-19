# 2026-10-19 课件配图与科学动画本机发布

按用户“启动最新版本”指示，已由统一发布脚本切换本机8080与8000到固定提交 `f4f0aafc1e2066311ea947092aafae64560f79ec`。发布目录为 `work/releases/commit-f4f0aafc1e20`，96个文件SHA256及二进制版本校验通过；本地提交，无推送，无新增Git tag。此前功能文档中的“本地候选、未部署”为本次发布前的历史状态。

发布包含函数参数演示与PPT GIF、私有图片引用、网页优先且Commons备用的配图检索，以及结构化通用图形动画和透镜/分子/细胞专业绘图。前端、Go后端、Python渲染均来自同一提交。AI生图不在本轮。

发布前Go test/vet及Vue格式检查与构建通过；Python渲染224项通过、3项跳过。活动与排队任务均为0；沿用原有正常退出机制。首次启动因FFmpeg下载缓慢中止了该次npm安装；核对新旧package.json和package-lock.json完全一致后复用旧发布的node_modules，npm依赖和FFmpeg/FFprobe执行检查通过，再次按原启动脚本成功启动。

上线检查记录为 `work/local-runtime/latest-deployment.json`：

- health身份与发布一致，live模式；PostgreSQL、Redis、MinIO和Milvus均ok。线上首页及两个前端静态资源hash匹配manifest。
- 图片引用为assets-v1，函数为functions-v1，科学渲染为science-visuals-v1；搜索及视觉服务已配置。部署检查没有重新调用外部搜索或付费模型。
- 经正式鉴权接口实际生成透镜、分子、细胞及通用scene的GIF预览，帧数分别为26、26、26、22；在本机work/local-runtime保存校验产物。
- 教师可见14项作品的状态与切换前一致，balance940、reserved60、available880保持不变；旧89001字节PPTX可下载，SHA256保持f78519520246d191f4fb6c1f82ef2f93f5ed83533a5f265d327de314dfc7a226。
- 匿名任务、积分、素材、SSE和下载均401；本人请求不存在素材404。没有创建业务任务，没有付费模型调用。

这次上线验证不替代真实课题教学质量、图片使用权限或桌面PowerPoint放映的人工验收。网页候选仍需按既有权限状态处理；此前长城试跑候选没有因此自动插入正式课件。
