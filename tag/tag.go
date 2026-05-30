package tag

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/victorwang171/ncmdump/parser"
)

// 全局复用共享的带超时控制的 HTTP Client，规避因频繁创建套接字而导致 TCP 端口耗尽的问题
var sharedHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

const (
	mimeJPEG = "image/jpeg"
	mimePNG  = "image/png"
)

// MetadataManager 定义了统一外部标签注入流程控制接口
type MetadataManager interface {
	Inject(path string, format string, cover []byte, meta *parser.Meta) error
}

type tagManagerImpl struct{}

// NewMetadataManager 构造并返回 MetadataManager 的默认实例
func NewMetadataManager() MetadataManager {
	return &tagManagerImpl{}
}

// Inject 执行具体的元数据和封面注入操作，在遇到不支持的格式时返回错误
func (m *tagManagerImpl) Inject(path string, format string, cover []byte, meta *parser.Meta) error {
	tagger, err := NewTagger(path, format)
	if err != nil {
		return err
	}
	return TagAudioFileFromMeta(tagger, cover, meta)
}

// containPNGHeader 校验图片前缀魔数是否符合 PNG 签名（8字节）
func containPNGHeader(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	return string(data[:8]) == string([]byte{137, 80, 78, 71, 13, 10, 26, 10})
}

// fetchUrl 辅助函数：网络拉取远程 HTTP(S) 专辑封面二进制数据
func fetchUrl(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download album cover: remote returned status %d", res.StatusCode)
	}
	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

// TagAudioFileFromMeta 根据解析后的 Meta 结构体，将标题、专辑、歌手、注释和封面完整注入给定的底层 Tagger 容器
func TagAudioFileFromMeta(tag Tagger, imgData []byte, meta *parser.Meta) error {
	// 1. 如果本地无图片字节，且存在远程 CoverUrl，则自适应通过网络下载封面
	if imgData == nil && meta.Album.CoverUrl != "" {
		if coverData, err := fetchUrl(meta.Album.CoverUrl); err != nil {
			log.Println(err)
		} else {
			imgData = coverData
		}
	}

	// 2. 注入专辑封面图片
	if imgData != nil {
		picMIME := mimeJPEG
		if containPNGHeader(imgData) {
			picMIME = mimePNG
		}
		tag.SetCover(imgData, picMIME)
	} else if meta.Album.CoverUrl != "" {
		tag.SetCoverUrl(meta.Album.CoverUrl)
	}

	// 3. 注入歌曲标题名
	if meta.Name != "" {
		tag.SetTitle(meta.Name)
	}

	// 4. 注入专辑名称
	if meta.Album.Name != "" {
		tag.SetAlbum(meta.Album.Name)
	}

	// 5. 注入艺术家/歌手列表（支持多歌手逗号分隔或多帧追加模式）
	artists := make([]string, 0)
	for _, artist := range meta.Artists {
		artists = append(artists, artist.Name)
	}
	if len(artists) > 0 {
		tag.SetArtist(artists)
	}

	// 6. 注入注释信息
	if meta.Comment != "" {
		tag.SetComment(meta.Comment)
	}

	// 7. 保存落盘
	return tag.Save()
}

