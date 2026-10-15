package main

const animationRebuildMessage = "教学动画功能重构中，暂时停止新建和继续生成；未创建新作品或预留新积分，历史作品仍可查看和下载。"

func animationUnavailable() error {
	return fail(503, animationRebuildMessage)
}
