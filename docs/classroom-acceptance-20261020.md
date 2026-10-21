# 课堂设备与教学质量验收记录

本机放映自动取证已完成，下面两项仍待实际教师与目标课堂设备验收。没有签名、日期和证据的空表不算通过。打包文件夹：work/acceptance-closure-20261020/classroom-package。

## 文件与预期行为

| 文件 | 内容来源 | 放映时应看到 |
| --- | --- | --- |
| science.pptx | 固定渲染样例，非实时AI生成 | 第1页透镜物距与光路变化；第2页羟基渐进高亮；第3页叶绿体高亮；第4页圆点沿路径运动 |
| functions.pptx | 固定渲染样例，非实时AI生成 | 第1页参数a变化，经过0时标明非二次函数；第2页本来就是静态可编辑曲线；第3页反比例两支分离 |
| native-click.pptx | 固定渲染样例，非实时AI生成 | 初始只有坐标轴，第一次点击出现第一条曲线及标记/图例，第二次出现第二条；翻页、返回、重开再验 |
| real-long-conversation.pptx | 13轮真实需求沟通后生成的9页历史课 | 检查全文、目标、练习和备注；此课没有图片，不能用于证明自动配图或图片史实正确 |

manifest.json记录实际复制文件的SHA256。slideshow-*.gif是本机PowerPoint放映窗口截图拼接的回放，供查看动态证据，不替代目标电脑上的原PPTX放映。固定样例的播放通过也不代表真实模型已学会为每课正确选择动画。

## 目标设备检查

在实际课堂电脑上复制整个文件夹，填写device-review.csv的设备、系统、软件完整版本、检查人和日期。打开原PPTX检查全部页面，再从头放映。

1. 字体、上下标、分式、符号和中文换行可读；记录替换字体及具体页码，不仅记录“能打开”。
2. 正文、图例、图注、表格不遮挡、不裁切，后排投影能辨认。
3. GIF连续播放并循环，翻页和返回后行为可接受；静态对照页保持静态。
4. 点击动画初始隐藏正确，一击一步、顺序正确；教师能控制停留、回看和揭示。
5. 关闭重开后再次检查；保存截图或短录屏路径到evidence，问题写入notes，status填passed或failed。

可选自动取证（需要Windows桌面PowerPoint；脚本会操作放映窗口，已有放映时拒绝打断）：

```powershell
pwsh -File scripts/check-ppt-slideshow.ps1 -Path work/acceptance-closure-20261020/classroom-package/science.pptx -OutputDirectory work/classroom-device/science -Mode gif -Frames 8
pwsh -File scripts/check-ppt-slideshow.ps1 -Path work/acceptance-closure-20261020/classroom-package/native-click.pptx -OutputDirectory work/classroom-device/click -Mode click -Frames 3
```

脚本结果只证明该设备实际放映行为，不自动填写教师质量结论。其他放映软件需使用该软件实际检查，不能沿用PowerPoint结果。

## 学科教师评价

teaching-review.csv每科一行，填写实际课件及版本。按同一需求、同一材料和范围提供旧/新课件，打乱顺序，由对应学科教师分别评分并写具体理由；缺少可比旧稿时先记录单稿评价，不能伪称完成对照。

五维0–4分：事实与答案；目标和学生考查一致性；投影表达与可读性；图像相关性及图注依据；课堂节奏。每项至少3分且没有关键事实、公式、题设或答案错误，才可记passed。记录评分分歧与裁定，禁止用总平均掩盖关键错误。建议数值是验收门槛，不是已经取得的评分。

## 教材逐页核对

work/acceptance-closure-20261020/textbook/formula-review.csv列出实际122页教材中41个正文/公式警告页，初始状态均pending。对照原PDF文件页序填写原式、提取式、修正和依据；页边警告另见report.json，不能混作41页全部是确定公式错误。复杂分式、根式、矩阵、扫描页、双栏和跨页表格仍需要分别补实际校对材料。

如果关键条件或公式读不出，保留未核验，补充原文或可核对页面后再生成。不由模型猜测缺失公式，也不把当前math-layout-v2称为通用OCR。
