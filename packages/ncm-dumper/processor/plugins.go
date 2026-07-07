package processor

import (
	"fmt"
	"os"
)

// DeleteSourceProcessor 转换成功后删除 .ncm 加密源文件的后处理插件
type DeleteSourceProcessor struct{}

// Process 执行删除源文件逻辑，在删除前校验目标生成的音频文件大小完整且未损坏
func (p *DeleteSourceProcessor) Process(src, dst string) error {
	// 1. 首先检查最终生成的音频文件大小元数据
	info, err := os.Stat(dst)
	if err != nil {
		// 若 dst 尚未移动完成，尝试检查临时输出文件 (.tmp) 是否完整
		info, err = os.Stat(dst + ".tmp")
		if err != nil {
			return fmt.Errorf("target file not found: %w", err)
		}
	}

	// 2. 校验文件完整性，防止写入 0 字节的空音频文件
	if info.Size() == 0 {
		return fmt.Errorf("target file is empty, skipping source deletion")
	}

	// 3. 安全地删除原始加密的 .ncm 源文件
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove source file %q: %w", src, err)
	}

	return nil
}

// SizeComparisonProcessor 目标文件夹中存在同名音频文件时，仅保留体积（Size）更大那个的后处理插件
type SizeComparisonProcessor struct{}

// Process 对比临时解密文件 (.tmp) 与已存在的音频文件体积，只保留更大的文件
func (p *SizeComparisonProcessor) Process(src, dst string) error {
	tempDst := dst + ".tmp"

	// 1. 获取本次解密临时生成的音频文件属性
	tempInfo, err := os.Stat(tempDst)
	if err != nil {
		if os.IsNotExist(err) {
			// 若临时文件不存在，表明已经被其他前置后处理链重命名或消费完毕，直接跳过
			return nil
		}
		return fmt.Errorf("failed to stat temporary file: %w", err)
	}

	// 2. 检查目标位置是否已存在同名的音乐文件
	destInfo, err := os.Stat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			// 若同名音乐文件不存在，则直接原子重命名将临时文件移动为最终音频文件
			if err := os.Rename(tempDst, dst); err != nil {
				return fmt.Errorf("failed to rename temporary file to destination: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to stat destination file: %w", err)
	}

	// 3. 同名文件均存在，对比两者体积并只保留大体积文件
	if tempInfo.Size() > destInfo.Size() {
		// 本次转换生成的文件更大，强行覆盖已有的较小文件
		// 在 Windows 系统下，重命名前必须先显式 Remove 删除已有目标，否则会引发 "file already exists" 错误
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("failed to remove smaller destination file: %w", err)
		}
		if err := os.Rename(tempDst, dst); err != nil {
			return fmt.Errorf("failed to replace destination with larger temporary file: %w", err)
		}
	} else {
		// 已存在的同名文件更大，舍弃本次转换生成的较小临时文件，直接将其删除
		if err := os.Remove(tempDst); err != nil {
			return fmt.Errorf("failed to remove smaller temporary file: %w", err)
		}
	}

	return nil
}
