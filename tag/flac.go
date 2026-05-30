package tag

import (
	"github.com/go-flac/flacpicture/v2"
	"github.com/go-flac/flacvorbis/v2"
	"github.com/go-flac/go-flac/v2"
)

// FlacTagger 实现了 FLAC 音频格式的元数据标签与封面图片注入工具
type FlacTagger struct {
	path string
	file *flac.File
	cmts *flacvorbis.MetaDataBlockVorbisComment
}

// NewFlacTagger 构造并解析 FLAC 文件，定位或初始化其 VorbisComment 元数据块
func NewFlacTagger(path string) (*FlacTagger, error) {
	// 直接读取并解析整个 FLAC 文件头和元数据结构
	f, err := flac.ParseFile(path)
	if err != nil {
		return nil, err
	}

	// 检索已存在的 VorbisComment 块
	var cmtmeta *flac.MetaDataBlock
	for _, m := range f.Meta {
		if m.Type == flac.VorbisComment {
			cmtmeta = m
			break
		}
	}
	var cmts *flacvorbis.MetaDataBlockVorbisComment
	if cmtmeta != nil {
		cmts, err = flacvorbis.ParseFromMetaDataBlock(*cmtmeta)
		if err != nil {
			return nil, err
		}
	} else {
		cmts = flacvorbis.New()
	}

	tagger := new(FlacTagger)
	tagger.file = f
	tagger.cmts = cmts
	tagger.path = path
	return tagger, nil
}

// SetCover 构造标准 FLAC 封面图片 Meta 块并追加到文件元数据链中
func (f *FlacTagger) SetCover(buf []byte, mime string) error {
	picture, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "Front cover", buf, mime)
	if err == nil {
		picturemeta := picture.Marshal()
		f.file.Meta = append(f.file.Meta, &picturemeta)
	}
	return err
}

// SetCoverUrl 将远程图片 URL 以外部引用的形式写入 FLAC 封面图片 Block 中
func (f *FlacTagger) SetCoverUrl(coverUrl string) error {
	picture := &flacpicture.MetadataBlockPicture{
		PictureType: flacpicture.PictureTypeFrontCover,
		MIME:        "-->",
		Description: "Front cover",
		ImageData:   []byte(coverUrl),
	}
	picturemeta := picture.Marshal()
	f.file.Meta = append(f.file.Meta, &picturemeta)
	return nil
}

// addTag 内部辅助函数：若对应键的标签项不存在，则向 VorbisComment 中写入新的值列表
func (f *FlacTagger) addTag(key string, values ...string) error {
	if old, err := f.cmts.Get(key); err != nil {
		return err
	} else if len(old) == 0 {
		for _, val := range values {
			if err = f.cmts.Add(key, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetTitle 注入歌曲标题
func (f *FlacTagger) SetTitle(title string) error {
	return f.addTag(flacvorbis.FIELD_TITLE, title)
}

// SetAlbum 注入专辑名
func (f *FlacTagger) SetAlbum(album string) error {
	return f.addTag(flacvorbis.FIELD_ALBUM, album)
}

// SetArtist 注入多位艺术家歌手
func (f *FlacTagger) SetArtist(artists []string) error {
	return f.addTag(flacvorbis.FIELD_ARTIST, artists...)
}

// SetComment 注入注释信息
func (f *FlacTagger) SetComment(comment string) error {
	return f.addTag(flacvorbis.FIELD_DESCRIPTION, comment)
}

// setVorbisCommentMeta 将修改后的 VorbisComment 块回填更新至 FLAC 文件元数据列表中
func (f *FlacTagger) setVorbisCommentMeta(block *flac.MetaDataBlock) {
	var idx = -1
	for i, m := range f.file.Meta {
		if m.Type == flac.VorbisComment {
			idx = i
			break
		}
	}
	if idx == -1 {
		f.file.Meta = append(f.file.Meta, block)
	} else {
		f.file.Meta[idx] = block
	}
}

// Save 编码所有改动并最终写盘持久化 FLAC 文件
func (f *FlacTagger) Save() error {
	block := f.cmts.Marshal()
	f.setVorbisCommentMeta(&block)
	return f.file.Save(f.path)
}

