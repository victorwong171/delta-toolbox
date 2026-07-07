package tag

import (
	"fmt"
	"strings"
)

const (
	audioFormatMp3  = "mp3"
	audioFormatFlac = "flac"
)

// Tagger 是针对 MP3 和 FLAC 音频文件元数据（标签与封面）进行注入操作的底层统一包装接口
type Tagger interface {
	SetCover(buf []byte, mime string) error // 设置专辑封面的二进制数据缓冲区
	SetCoverUrl(coverUrl string) error      // 设置专辑封面的远程 URL 地址链接
	SetTitle(string) error                  // 设置歌曲标题名
	SetAlbum(string) error                  // 设置专辑名称
	SetArtist([]string) error               // 设置多艺术家/歌手列表
	SetComment(string) error                // 设置注释信息
	Save() error                            // 将所有内存修改强制落盘持久化保存，必须最终被调用
}

// NewTagger 根据目标音频的格式（MP3 或 FLAC），动态构造并返回对应的 Tagger 实体实现
func NewTagger(input, format string) (Tagger, error) {
	var tagger Tagger
	var err error
	switch strings.ToLower(format) {
	case audioFormatMp3:
		tagger, err = NewMp3Tagger(input)
	case audioFormatFlac:
		tagger, err = NewFlacTagger(input)
	default:
		err = fmt.Errorf("format: %s is not supported", format)
	}

	return tagger, err
}

