package pipeline

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/victorwang171/ncmdump/parser"
	"github.com/victorwang171/ncmdump/processor"
	"github.com/victorwang171/ncmdump/tag"
)

// ConversionPipeline 核心接口：编排整个 NCM 加密文件的解密、音频标签注入以及转换后处理流程
type ConversionPipeline interface {
	Convert(inputPath, outputDir string, isTag bool) error
}

type ncmPipeline struct {
	parser     parser.NCMParser
	tagger     tag.MetadataManager
	processors []processor.FileProcessor
}

// NewConversionPipeline 返回 ConversionPipeline 实例，组装解析器、标签管理器和后处理插件链
func NewConversionPipeline(
	p parser.NCMParser,
	t tag.MetadataManager,
	procs []processor.FileProcessor,
) ConversionPipeline {
	return &ncmPipeline{
		parser:     p,
		tagger:     t,
		processors: procs,
	}
}

// Convert 实现了单趟 (Single-Pass) 顺序流式解密处理，极大优化了内存分配并杜绝文件占用
func (n *ncmPipeline) Convert(inputPath, outputDir string, isTag bool) error {
	var meta *parser.Meta
	var format string
	var cover []byte
	var outputFile string
	var tempOutputFile string

	// 1. 读取元数据、顺序流式解密写盘并提取封面（使用匿名函数确保所有文件句柄在后续后处理前被正确关闭和释放）
	err := func() error {
		fp, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("failed to open source NCM: %w", err)
		}
		defer fp.Close()

		// 顺序线性单趟解析
		parsed, err := n.parser.Parse(fp)
		if err != nil {
			return fmt.Errorf("failed to parse NCM: %w", err)
		}

		meta = parsed.Metadata()
		format = parsed.AudioFormat()
		cover = parsed.Cover()

		outputFile = getOutputFullPath(inputPath, outputDir, format)
		tempOutputFile = outputFile + ".tmp"

		// 创建临时输出文件
		outFile, err := os.Create(tempOutputFile)
		if err != nil {
			return fmt.Errorf("failed to create temporary output: %w", err)
		}
		defer outFile.Close()

		// 直接将解密流拷贝至临时文件，低内存高吞吐
		_, err = io.Copy(outFile, parsed.DecryptedStream())
		return err
	}()
	if err != nil {
		if tempOutputFile != "" {
			_ = os.Remove(tempOutputFile)
		}
		return err
	}

	// 2. 如果需要添加标签，则在临时文件上打标签
	if isTag && meta != nil {
		if err := n.tagger.Inject(tempOutputFile, format, cover, meta); err != nil {
			log.Printf("Warning: failed to inject metadata tags to %s: %v\n", tempOutputFile, err)
		}
	}

	// 3. 链式执行后处理插件链
	for _, p := range n.processors {
		if err := p.Process(inputPath, outputFile); err != nil {
			log.Printf("Warning: post-processor chain failed: %v\n", err)
			break
		}
	}

	// 4. 最终隐式文件落盘：若临时文件仍然存在（如没有执行 SizeComparisonProcessor），则原子移动到目标路径
	if _, err := os.Stat(tempOutputFile); err == nil {
		_ = os.Remove(outputFile) // 兼容 Windows 重命名覆盖限制
		if err := os.Rename(tempOutputFile, outputFile); err != nil {
			return fmt.Errorf("failed to finalize audio output: %w", err)
		}
	}

	return nil
}

// getOutputFullPath 计算并返回输出音频文件的绝对路径
func getOutputFullPath(input string, outputDir string, format string) string {
	if outputDir == "" {
		outputDir = filepath.Dir(input)
	} else {
		outputDir = filepath.Clean(outputDir)
		_ = os.MkdirAll(outputDir, 0755)
	}

	name := filepath.Base(input)
	if strings.ToLower(filepath.Ext(name)) == ".ncm" {
		name = name[:len(name)-4]
	}
	return filepath.Join(outputDir, name+"."+format)
}
