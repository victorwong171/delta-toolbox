package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FatWang1/fatwang-go-utils/config"
	cli "github.com/urfave/cli/v2"
	"github.com/yoki123/ncmdump"
	"github.com/yoki123/ncmdump/cmd/ncmdump/conf"
	"github.com/yoki123/ncmdump/processor"
	"github.com/yoki123/ncmdump/tag"
)

var VERSION = "VERSION"

// expand tilde `~` to the user's home directory
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

func mkdirIfNotExist(path string) error {
	info, err := os.Stat(path)

	if os.IsNotExist(err) {
		err = os.MkdirAll(path, 0755)
	}

	if err != nil {
		return err
	}

	if !info.IsDir() {
		return errors.New(fmt.Sprintf("output path is not a directory"))
	}
	return nil
}

func getOutputFullPath(input string, outputDir string, format string) string {
	if outputDir == "" {
		outputDir = filepath.Dir(input)
	} else {
		outputDir = filepath.Clean(outputDir)

		var err error
		if outputDir, err = expandTilde(outputDir); err != nil {
			outputDir = filepath.Dir(input)
			log.Printf("get user's home directory error: %s, write to input path instead\n", err)
		}

		// auto mkdir if not exist
		if err := mkdirIfNotExist(outputDir); err != nil {
			outputDir = filepath.Dir(input)
			log.Printf("stat output path error: %s, write to input path instead\n", err)
		}
	}

	name := filepath.Base(input)
	newName := strings.Replace(name, ".ncm", "."+format, -1)
	return outputDir + string(filepath.Separator) + newName
}

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

// processFile 处理单个 NCM 文件，将其转换为指定格式并添加标签（如果需要）。
// input 是输入的 NCM 文件路径，outputDir 是输出目录路径，isTag 表示是否为输出文件添加标签。
func processFile(input string, outputDir string, isTag bool, processors []processor.FileProcessor) {
	// 使用 defer 和 recover 捕获并记录处理文件过程中可能出现的 panic
	defer func() {
		err := recover()
		if err != nil {
			// 记录处理文件时出现的错误
			log.Printf("Error processing file:\t%s\n", input)
			// 记录错误的详细信息
			log.Printf("Error information:\t\t%v\n", err)
		}
	}()

	var meta ncmdump.Meta
	var cover []byte

	// 1. 读取元数据、解密写入临时文件并提取封面（使用匿名函数确保源文件句柄在后续的后处理操作前已被正确关闭和释放）
	err := func() error {
		fp, err := os.Open(input)
		if err != nil {
			return err
		}
		defer fp.Close()

		// 从文件中提取元数据
		meta, err = ncmdump.DumpMeta(fp)
		if err != nil {
			return err
		}

		// 获取最终和临时输出路径并解密写盘
		output := getOutputFullPath(input, outputDir, meta.Format)
		tempOutput := output + ".tmp"
		
		outFile, err := os.Create(tempOutput)
		if err != nil {
			return err
		}
		defer outFile.Close()

		if _, err := fp.Seek(0, io.SeekStart); err != nil {
			return err
		}

		if err := ncmdump.DumpToWriter(fp, outFile); err != nil {
			return err
		}

		// 提取封面
		if _, err := fp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		cover, err = ncmdump.DumpCover(fp)
		if err != nil {
			return err
		}

		return nil
	}()
	if err != nil {
		panic(err)
	}

	// 获取最终输出和临时输出文件的完整路径
	output := getOutputFullPath(input, outputDir, meta.Format)
	tempOutput := output + ".tmp"

	// 记录文件处理成功的信息
	log.Printf("Successfully processed:\t%s\n", input)
	log.Printf("Successfully saved temp file:\t%s\n", tempOutput)

	// 2. 如果需要添加标签，则在临时文件上打标签
	if isTag {
		// 创建一个标签器实例
		tagger, err := tag.NewTagger(tempOutput, meta.Format)
		if err != nil {
			// 若创建标签器失败，触发 panic
			panic(err)
		}

		// 为输出文件添加标签
		err = tag.TagAudioFileFromMeta(tagger, cover, &meta)
		if err != nil {
			// 若添加标签失败，记录错误信息
			log.Printf("Error tagging file:\t%s\n", tempOutput)
		} else {
			// 若添加标签成功，记录成功信息
			log.Printf("Successfully tagged file:\t%s\n", tempOutput)
		}
	}

	// 3. 链式执行配置的后处理插件
	for _, p := range processors {
		if err := p.Process(input, output); err != nil {
			log.Printf("Processor failed for %s: %v\n", input, err)
			break
		}
	}

	// 4. 最终隐式文件落盘：若临时文件仍然存在（如没有执行 SizeComparisonProcessor），则原子移动到目标路径
	if _, err := os.Stat(tempOutput); err == nil {
		_ = os.Remove(output) // 兼容 Windows 重命名限制
		if err := os.Rename(tempOutput, output); err != nil {
			log.Printf("Failed to rename temporary file %s to %s: %v\n", tempOutput, output, err)
		} else {
			log.Printf("Successfully saved final file:\t%s\n", output)
		}
	}
}

var registry = map[string]processor.FileProcessor{
	"delete_source":   &processor.DeleteSourceProcessor{},
	"size_comparison": &processor.SizeComparisonProcessor{},
}

// 定义任务结构体
type fileTask struct {
	filename  string
	outputDir string
	isTag     bool
}

func main() {
	cfg, err := config.NewLoader[*conf.Config]("conf", "D:\\software\\buffer\\projects\\ncm-dumper\\cmd\\ncmdump\\conf", "yaml").Load()
	if err != nil {
		panic(err)
	}
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

		// 获取所有要处理的路径：如果有命令行参数，只使用命令行参数；否则使用配置文件中的source路径
		allPaths := make([]string, 0)
		if len(args) > 0 {
			allPaths = append(allPaths, args...)
		} else {
			allPaths = append(allPaths, cfg.GetSourcePath()...)
		}

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

		// 过滤出NCM文件
		var ncmFiles []string
		for _, filename := range files {
			if strings.ToLower(filepath.Ext(filename)) == ".ncm" {
				ncmFiles = append(ncmFiles, filename)
			}
		}

		// 创建任务通道和结果通道
		// 使用带有背压机制的较小缓冲区通道，防止因文件数量巨大导致无谓的内存分配
		bufferSize := workerCount * 2
		if bufferSize < 100 {
			bufferSize = 100
		}
		if bufferSize > len(ncmFiles) {
			bufferSize = len(ncmFiles)
		}
		taskChan := make(chan fileTask, bufferSize)
		wg := sync.WaitGroup{}

		// 解析并动态加载配置驱动的后处理插件
		var activeProcessors []processor.FileProcessor
		for _, name := range cfg.GetProcessors() {
			if p, ok := registry[name]; ok {
				activeProcessors = append(activeProcessors, p)
			} else {
				log.Printf("Warning: unknown processor %q\n", name)
			}
		}

		// 启动worker池
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for task := range taskChan {
					processFile(task.filename, task.outputDir, task.isTag, activeProcessors)
				}
			}()
		}

		// 发送任务
		for _, filename := range ncmFiles {
			taskChan <- fileTask{
				filename:  filename,
				outputDir: outputDir,
				isTag:     isTag,
			}
		}

		// 关闭任务通道并等待所有任务完成
		close(taskChan)
		wg.Wait()

		return nil
	}
	app.Run(os.Args)
}
