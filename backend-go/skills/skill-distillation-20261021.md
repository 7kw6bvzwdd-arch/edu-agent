# manim-video / frontend-slides 项目适配来源

2026-10-21安装。后端编译嵌入项目化原则，不读取开发者个人目录、不执行下载包脚本。

| 上游 | 下载归档SHA256 | 应用 |
| --- | --- | --- |
| NousResearch/hermes-agent，skills/creative/manim-video | 5081db40b5f15e32fe43b8ce4f43c06536bb21500bbcc97ce4dae1ecb3d6945a | 学习问题、先图后式、分部揭示、语音节拍、关键状态验收 |
| affaan-m/ECC，docs/zh-CN/skills/frontend-slides | 0abbe9e3b504eed17974aa5da163d217909e1dff7ee5a853b631358003953cf8 | 视口边界、内容容量、可读性、实物预览 |

原下载端点：
- https://ai.trusttools.cn/api/skills/NousResearch/hermes-agent/manim-video/download?repo_path=skills%2Fcreative%2Fmanim-video%2FSKILL.md
- https://ai.trusttools.cn/api/skills/affaan-m/ECC/frontend-slides/download?repo_path=docs%2Fzh-CN%2Fskills%2Ffrontend-slides%2FSKILL.md

安装目录为C:/Users/YIZE/.codex/skills/manim-video和frontend-slides，各有SOURCE.json。使用自定义UA下载；归档经过路径/文件类型检查后解压。归档hash固定此次内容，未将浮动下载地址冒称固定Git提交。Manim包声明MIT；frontend-slides归档未提供独立LICENSE，保留来源，仅概括方法，未复制其模板代码。

后端入口：animAuthorInstructions → edu-animation-v2；coursewareTeachingSkill / 整课规划共用 composition → edu-courseware-v5。初次、修复和改写沿用既有共用入口。渲染器lesson-visual-v7连续绘制scene，并将carbon的程序几何、局部reveal/slide与逐句时间同步。课件同源scene采用全宽画布及真实GIF/PPTX输出。

舍弃上游强制暗底/等宽字体/抽象装饰、固定三套风格确认和机械换色换版；不安装Manim运行栈，不执行模型代码，不更换语音供应商或PPTX交付格式。原审核、鉴权、计费和历史文件协议保持。
