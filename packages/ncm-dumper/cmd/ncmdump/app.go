package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	cli "github.com/urfave/cli/v2"
	"github.com/victorwang171/ncmdump/cmd/ncmdump/conf"
	"github.com/victorwang171/ncmdump/parser"
	"github.com/victorwang171/ncmdump/pipeline"
	"github.com/victorwang171/ncmdump/processor"
	"github.com/victorwang171/ncmdump/tag"
)

var VERSION = "VERSION"

// 注册并定义所有可用的后处理插件映射表
var registry = map[string]processor.FileProcessor{
	"delete_source":   &processor.DeleteSourceProcessor{},   // 转换成功后删除源文件插件
	"size_comparison": &processor.SizeComparisonProcessor{}, // 目标文件夹同名音频对比仅保留大文件插件
}

// 并发通道传递的文件处理任务结构体
type fileTask struct {
	filename  string // 待处理的加密 .ncm 文件绝对路径
	outputDir string // 转换后音频文件的输出目录路径
	isTag     bool   // 是否开启音频标签（元数据/封面）自动注入
}

// expandTilde 兼容处理并将 Unix/macOS 下的波浪号 '~' 展开映射为当前系统的主用户家目录
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home + path[1:], nil
}

// getAllFiles 递归扫描并读取指定目录路径下的所有文件列表
func getAllFiles(dir string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				files = append(files, path)
			}
			return nil
		})
	return files, err
}

// locateConfigDir 依据 Go 社区 CLI 工具标准，动态自适应定位配置文件目录，保障跨平台移植性
func locateConfigDir() string {
	// 1. 尝试当前执行工作目录下的 "conf" 文件夹
	if info, err := os.Stat("conf"); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join("conf", "conf.yaml")); err == nil {
			return "conf"
		}
	}
	// 2. 尝试当前执行工作目录同级位置
	if _, err := os.Stat("conf.yaml"); err == nil {
		return "."
	}
	// 3. 尝试开发环境工作区本地源码目录 "cmd/ncmdump/conf"
	devDir := filepath.Join("cmd", "ncmdump", "conf")
	if info, err := os.Stat(devDir); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(devDir, "conf.yaml")); err == nil {
			return devDir
		}
	}
	// 4. 尝试当前运行的二进制可执行程序所在目录（兼容绿色版便携分发）
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		// 检查可执行程序所在目录下的 conf 文件夹
		dir := filepath.Join(execDir, "conf")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, "conf.yaml")); err == nil {
				return dir
			}
		}
		// 检查可执行程序同级目录下是否存在
		if _, err := os.Stat(filepath.Join(execDir, "conf.yaml")); err == nil {
			return execDir
		}
	}
	// 5. 尝试操作系统标准的用户应用数据配置目录（Linux: ~/.config/ncmdump/，Windows: %APPDATA%/ncmdump/）
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		dir := filepath.Join(userConfigDir, "ncmdump")
		if _, err := os.Stat(filepath.Join(dir, "conf.yaml")); err == nil {
			return dir
		}
	}
	// 兜底回退至 "conf" 默认文件夹
	return "conf"
}

// BuildApp 根据已解析的配置反向拼装并返回一个完整的 cli.App 实例，完全实现入口逻辑高内聚
func BuildApp(cfg *conf.Config) *cli.App {
	app := cli.NewApp()
	app.Version = VERSION
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "output",
			Value: cfg.GetTargetPath(),
			Usage: "output directory path.",
		},
		&cli.BoolFlag{
			Name:  "tag",
			Value: true,
			Usage: "tag the output file from ncm file metadata.",
		},
		&cli.IntFlag{
			Name:  "workers",
			Value: 4,
			Usage: "number of concurrent workers.",
		},
	}

	app.Action = func(c *cli.Context) error {
		args := c.Args().Slice()
		outputDir := c.String("output")
		isTag := c.Bool("tag")
		workerCount := c.Int("workers")

		// 1. 如果路径中包含波浪号，自动展开路径
		if outputDir != "" {
			if expanded, err := expandTilde(outputDir); err == nil {
				outputDir = expanded
			}
		}

		// 2. 解析输入的加密源路径列表
		allPaths := make([]string, 0)
		if len(args) > 0 {
			allPaths = append(allPaths, args...)
		} else {
			allPaths = append(allPaths, cfg.GetSourcePath()...)
		}

		// 3. 递归检索出目录下所有的待处理文件
		files := make([]string, 0)
		for _, path := range allPaths {
			if path == "" {
				continue
			}
			if info, err := os.Stat(path); err != nil {
				log.Printf("Path %s does not exist.", path)
			} else if info.IsDir() {
				dirFiles, err := getAllFiles(path)
				if err != nil {
					log.Printf("Error while reading %s: %s", path, err.Error())
					continue
				}
				files = append(files, dirFiles...)
			} else {
				files = append(files, path)
			}
		}

		// 4. 过滤剔除非 NCM 的多余文件
		var ncmFiles []string
		for _, filename := range files {
			if strings.ToLower(filepath.Ext(filename)) == ".ncm" {
				ncmFiles = append(ncmFiles, filename)
			}
		}

		if len(ncmFiles) == 0 {
			return nil
		}

		// 5. 根据配置文件，解析并动态激活后处理插件列表
		var activeProcessors []processor.FileProcessor
		for _, name := range cfg.GetProcessors() {
			if p, ok := registry[name]; ok {
				activeProcessors = append(activeProcessors, p)
			} else {
				log.Printf("Warning: unknown processor %q\n", name)
			}
		}

		// 6. 构造转化核心管道 ConversionPipeline 及其依赖
		ncmParser := &parser.SequentialNCMParser{}
		metaManager := tag.NewMetadataManager()
		conversionPipeline := pipeline.NewConversionPipeline(ncmParser, metaManager, activeProcessors)

		// 7. 启动并发任务 worker 工作池
		bufferSize := workerCount * 2
		if bufferSize < 100 {
			bufferSize = 100
		}
		if bufferSize > len(ncmFiles) {
			bufferSize = len(ncmFiles)
		}
		taskChan := make(chan fileTask, bufferSize)
		var wg sync.WaitGroup

		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for task := range taskChan {
					// 启动处理保护块，捕获异常崩溃以防止单一损坏文件引发整个多线程转换队列闪退
					err := func() error {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("Panic processing %s: %v\n", task.filename, r)
							}
						}()
						return conversionPipeline.Convert(task.filename, task.outputDir, task.isTag)
					}()
					if err != nil {
						log.Printf("Error processing %s: %v\n", task.filename, err)
					}
				}
			}()
		}

		// 8. 依次向工作通道塞入文件解密处理任务
		for _, filename := range ncmFiles {
			taskChan <- fileTask{
				filename:  filename,
				outputDir: outputDir,
				isTag:     isTag,
			}
		}

		// 9. 关闭通道并等待并发池完美收网
		close(taskChan)
		wg.Wait()

		return nil
	}

	return app
}
