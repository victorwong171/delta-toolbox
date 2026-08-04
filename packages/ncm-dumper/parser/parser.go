package parser

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

var (
	aesCoreKey   = []byte{0x68, 0x7A, 0x48, 0x52, 0x41, 0x6D, 0x73, 0x6F, 0x35, 0x6B, 0x49, 0x6E, 0x62, 0x61, 0x78, 0x57}
	aesModifyKey = []byte{0x23, 0x31, 0x34, 0x6C, 0x6A, 0x6B, 0x5F, 0x21, 0x5C, 0x5D, 0x26, 0x30, 0x55, 0x3C, 0x27, 0x28}
)

// ParsedNCM 代表解析后的 NCM 只读视图接口，解耦音频流与元数据
type ParsedNCM interface {
	Metadata() *Meta            // 获取歌曲的元数据信息（歌曲名、艺术家等）
	Cover() []byte              // 获取专辑封面二进制数据
	AudioFormat() string        // 获取推导出的音频格式（如 mp3 或 flac）
	DecryptedStream() io.Reader // 获取流式即时解密的音频数据读取器
}

// NCMParser 定义了解析 NCM 数据流的标准接口，以支持不同形式的解析算法
type NCMParser interface {
	Parse(r io.Reader) (ParsedNCM, error)
}

// SequentialNCMParser 顺序线性单趟解析器，无需 Seek 回退，单次流式扫过直接提取全部信息
type SequentialNCMParser struct{}

// parsedNCMImpl 解析后的 NCM 结果实体实现类
type parsedNCMImpl struct {
	meta        *Meta
	cover       []byte
	audioFormat string
	audioStream io.Reader
}

func (p *parsedNCMImpl) Metadata() *Meta            { return p.meta }
func (p *parsedNCMImpl) Cover() []byte              { return p.cover }
func (p *parsedNCMImpl) AudioFormat() string        { return p.audioFormat }
func (p *parsedNCMImpl) DecryptedStream() io.Reader { return p.audioStream }

// DecryptReader 实现了对底层 io.Reader 的流式包装，在 Read 读取时即时 (on-the-fly) 进行 XOR 异或解密
type DecryptReader struct {
	r            io.Reader  // 底层被读取的音频加密数据流
	xorLookup    *[256]byte // 预计算的 RC4 解密查找表 (256字节)，使用指向固定大小数组的指针消除所有边界检查
	streamOffset int        // 全局流偏移量，保障分块并发读取时异或密钥索引能完美咬合
}

// Read 执行流式解密，集成了编译器边界检查消除 (BCE) 指令级微优化以极大提升解密吞吐率
// 此外，将解密循环展开为 8，减少了循环控制开销并启用指令级并行 (ILP)
func (dr *DecryptReader) Read(p []byte) (n int, err error) {
	n, err = dr.r.Read(p)
	if n > 0 {
		p = p[:n]
		// BCE optimization: assert slice bounds before entering the loop
		_ = p[n-1]
		offset := byte(dr.streamOffset)
		lookup := dr.xorLookup // Lift pointer dereference out of loop to avoid reloading receiver field
		_ = lookup

		// Loop unrolling optimization (unrolled by 8):
		// This reduces loop overhead (fewer condition checks and increments) and allows instruction-level
		// parallelism (ILP) by exposing independent operations to the CPU scheduler/pipeline.
		// Since 'offset' is a byte, expressions like offset+1 etc. are checked and statically proven
		// by Go's compiler to be completely within the range [0, 255], ensuring zero bounds check overhead.
		i := 0
		for ; i <= n-8; i += 8 {
			sub := p[i : i+8]
			_ = sub[7]
			sub[0] ^= lookup[byte(offset+1)]
			sub[1] ^= lookup[byte(offset+2)]
			sub[2] ^= lookup[byte(offset+3)]
			sub[3] ^= lookup[byte(offset+4)]
			sub[4] ^= lookup[byte(offset+5)]
			sub[5] ^= lookup[byte(offset+6)]
			sub[6] ^= lookup[byte(offset+7)]
			sub[7] ^= lookup[byte(offset+8)]
			offset += 8
		}
		// Clean up remaining bytes using range over a sub-slice to achieve 100% bounds-check free loop
		if i < n {
			rem := p[i:]
			for j := range rem {
				offset++
				rem[j] ^= lookup[byte(offset)]
			}
		}
		dr.streamOffset += n
	}
	return
}

// Parse 顺序线性提取密钥、元数据和专辑封面，并在单趟流式处理中完成，避免所有 Seek 重复磁盘读取
func (sp *SequentialNCMParser) Parse(r io.Reader) (ParsedNCM, error) {
	// 1. 读取 NCM 头部标志位魔数 (8 字节 magic + 2 字节 gap 填充)
	header := make([]byte, 10)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read NCM header: %w", err)
	}
	if string(header[:8]) != "CTENFDAM" {
		return nil, fmt.Errorf("invalid NCM signature")
	}

	// 2. 读取加密的 AES 密钥长度与密钥数据
	keyData, err := readLenAndData(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read key: %w", err)
	}
	xorBytes(keyData, 0x64)                                 // 与 0x64 进行异或还原
	deKeyData, err := decryptAes128Ecb(aesCoreKey, keyData) // 使用 AES-128-ECB 解密
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}
	if len(deKeyData) < 17 {
		return nil, fmt.Errorf("decrypted key is too short")
	}
	rc4Key := deKeyData[17:] // 剔除 "neteasecloudmusic" 头部字段，截取真正的 RC4 密钥

	// 3. 读取加密的歌曲元数据 (Metadata) 长度与数据
	modifyData, err := readLenAndData(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta Meta
	if len(modifyData) > 0 {
		xorBytes(modifyData, 0x63) // 与 0x63 异或还原
		// 剔除 "163 key(Don't modify):" 前缀 (22字节) 后进行 Base64 解码
		deModifyData := make([]byte, base64.StdEncoding.DecodedLen(len(modifyData)-22))
		if _, err = base64.StdEncoding.Decode(deModifyData, modifyData[22:]); err != nil {
			return nil, fmt.Errorf("failed to base64 decode metadata: %w", err)
		}
		// 使用特殊的 AES 密钥对其进行解密
		deData, err := decryptAes128Ecb(aesModifyKey, deModifyData)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt metadata: %w", err)
		}
		if len(deData) > 6 {
			// 剔除前缀 "music:" 后反序列化为 Meta 结构体对象
			if err := json.Unmarshal(deData[6:], &meta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata JSON: %w", err)
			}
		}
	}

	// 4. 跳过 9 字节的 CRC/Gap 空白校验块
	gap := make([]byte, 9)
	if _, err := io.ReadFull(r, gap); err != nil {
		return nil, fmt.Errorf("failed to read gap: %w", err)
	}

	// 5. 读取专辑封面图片数据 (如果有)
	cover, err := readLenAndData(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read cover: %w", err)
	}

	// Build key box and lookup table
	box := buildKeyBox(rc4Key)
	var xorLookup [256]byte
	for j := 0; j < 256; j++ {
		bj := byte(j)
		xorLookup[bj] = box[(box[bj]+box[(box[bj]+bj)&0xff])&0xff]
	}

	// Deduce format if empty
	format := meta.Format
	if format == "" {
		format = "mp3" // Default fallback
	}

	return &parsedNCMImpl{
		meta:        &meta,
		cover:       cover,
		audioFormat: format,
		audioStream: &DecryptReader{
			r:         r,
			xorLookup: &xorLookup,
		},
	}, nil
}

// Helper functions for binary reading and decryption

func readLenAndData(r io.Reader) ([]byte, error) {
	var dataLen uint32
	if err := binary.Read(r, binary.LittleEndian, &dataLen); err != nil {
		return nil, err
	}
	if dataLen == 0 {
		return []byte{}, nil
	}
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}

func decryptAes128Ecb(key, data []byte) ([]byte, error) {
	data = data[:len(data)/aes.BlockSize*aes.BlockSize]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(data))
	bs := block.BlockSize()
	for i := 0; i <= len(data)-bs; i += bs {
		block.Decrypt(decrypted[i:i+bs], data[i:i+bs])
	}
	return _PKCS7UnPadding(decrypted), nil
}

func _PKCS7UnPadding(src []byte) []byte {
	length := len(src)
	if length == 0 {
		return nil
	}
	unpadding := int(src[length-1])
	if unpadding > length || unpadding <= 0 {
		return nil
	}
	return src[:(length - unpadding)]
}

func xorBytes(data []byte, val uint8) {
	for i := range data {
		data[i] ^= val
	}
}

func buildKeyBox(key []byte) []byte {
	box := make([]byte, 256)
	for i := 0; i < 256; i++ {
		box[i] = byte(i)
	}
	keyLen := byte(len(key))
	var c, lastByte, keyOffset byte
	for i := 0; i < 256; i++ {
		c = (box[i] + lastByte + key[keyOffset]) & 0xff
		keyOffset++
		if keyOffset >= keyLen {
			keyOffset = 0
		}
		box[i], box[c] = box[c], box[i]
		lastByte = c
	}
	return box
}
