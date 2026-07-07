package processor

// FileProcessor 定义了转换后处理插件的统一标准接口
type FileProcessor interface {
	// Process 执行对原始加密输入文件 (src) 以及所生成的解密目标文件 (dst) 的后处理逻辑。
	Process(src, dst string) error
}

