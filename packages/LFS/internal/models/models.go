package models

import "time"

// FileChunkInfo 表示文件分片的元数据信息。
type FileChunkInfo struct {
	FileName   string `json:"file_name"`   // 文件名
	TotalSize  int64  `json:"total_size"`  // 文件总大小（字节）
	ChunkIndex int    `json:"chunk_index"` // 分片索引（从0开始）
	ChunkSize  int64  `json:"chunk_size"`  // 分片大小（字节）
	TotalChunk int    `json:"total_chunk"` // 总分片数
	MD5        string `json:"md5"`         // 分片的MD5值
	ModTime    int64  `json:"mod_time"`    // 原始文件修改时间（Unix时间戳，秒）
}

// FileMetadata 表示文件或目录的元数据信息。
type FileMetadata struct {
	Name     string         `json:"name"`               // 文件或目录名
	Path     string         `json:"path"`               // 完整路径
	Size     int64          `json:"size"`               // 文件大小（字节），目录为0
	ModTime  time.Time      `json:"mod_time"`           // 修改时间
	MD5      string         `json:"md5,omitempty"`      // MD5值（仅文件）
	IsDir    bool           `json:"is_dir"`             // 是否为目录
	Children []FileMetadata `json:"children,omitempty"` // 子项列表（仅目录）
}
