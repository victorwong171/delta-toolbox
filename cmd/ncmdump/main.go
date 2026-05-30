package main

import (
	"os"

	"github.com/FatWang1/fatwang-go-utils/config"
	"github.com/victorwang171/ncmdump/cmd/ncmdump/conf"
)

// 主程序引导入口，设计为极简的依赖注入 (DI) 壳
func main() {
	// 1. 动态自适应定位并加载 conf.yaml 配置文件
	cfg, err := config.NewLoader[*conf.Config]("conf", locateConfigDir(), "yaml").Load()
	if err != nil {
		panic(err)
	}

	// 2. 组装并初始化 CLI 应用程序实体
	app := BuildApp(cfg)

	// 3. 传入命令行参数并拉起应用程序主行动流
	_ = app.Run(os.Args)
}
