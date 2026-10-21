# 技能来源与适配边界

适配版本：`edu-animation-v2`，2026-10-21。以下为设计方法的项目化改写，运行时仅嵌入 `references/teaching-composition.md`；不在线下载、不动态执行上游脚本。

v2 新增 manim-video 的分部构建与语音节拍、frontend-slides 的视口容量与实物预览原则。下载指纹、安装目录和采用/舍弃清单见 [本轮蒸馏记录](../skill-distillation-20261021.md)。

| 来源 | 固定版本 | 采用内容 |
| --- | --- | --- |
| [hyperframes-creative](https://github.com/heygen-com/hyperframes/tree/00d55a7f1dab0765296793b4da2f3e791b5ce42e/skills/hyperframes-creative) | `00d55a7f1dab0765296793b4da2f3e791b5ce42e` | house-style、video-composition、beat-direction：内容先于版式、明确主体、逐幕观察任务、实际语音节奏 |
| [hyperframes-animation](https://github.com/heygen-com/hyperframes/tree/8be76fc86cda5b2a77d0ee23b65f582e85376042/skills/hyperframes-animation) | `8be76fc86cda5b2a77d0ee23b65f582e85376042` | rules-index 与动作原则：动作表达关系，状态由时间决定，往返 seek 一致 |
| [faceless-explainer](https://github.com/heygen-com/hyperframes/tree/8be76fc86cda5b2a77d0ee23b65f582e85376042/skills/faceless-explainer) | `8be76fc86cda5b2a77d0ee23b65f582e85376042` | story-design、visual-design：围绕问题重组内容，图形解释机制，旁白不是整段原文搬运 |

上游 HyperFrames 为 Apache-2.0。本文件记录方法来源；未复制其运行时代码进后端。两个动作/讲解技能使用已有本地检出的固定版本，更新下载中断后未冒称已更新。creative 的安装文件逐项比对 GitHub tree blob SHA 后采用上述新版本。

教学适配优先：不采用营销视频的固定双焦点、装饰数量、全画面持续运动、强制转场与营销 CTA；不要求所有课题用同一个分镜模板。GSAP、任意 HTML、外链素材、上游语音供应商和自动付费服务不进入模型输出。

本项目的 AnimScript 校验、句子锚点、技术时长、受控科学图、代表幕/最终确认、离线 CSP、账户隔离与计费机制继续为约束。组件数量上限来自实际渲染能力；审美建议不能作为无依据的结构拒绝理由。

[playwright-interactive](https://github.com/openai/skills/tree/49f948faa9258a0c61caceaf225e179651397431/skills/.curated/playwright-interactive) 安装用于开发者浏览器验收，不注入内容生成提示词；本轮复用项目现有离线 Edge/CDP 检查。其固定来源为 `49f948faa9258a0c61caceaf225e179651397431`，安装文件按 GitHub tree blob SHA 核验。
