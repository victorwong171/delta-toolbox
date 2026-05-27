package flac

import (
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
)

func encodeUint32(n uint32) []byte {
	buf := bytes.NewBuffer([]byte{})
	if err := binary.Write(buf, binary.BigEndian, n); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func readUint8(r io.Reader) (res uint8, err error) {
	err = binary.Read(r, binary.BigEndian, &res)
	return
}

func readUint16(r io.Reader) (res uint16, err error) {
	err = binary.Read(r, binary.BigEndian, &res)
	return
}

func readUint32(r io.Reader) (res uint32, err error) {
	err = binary.Read(r, binary.BigEndian, &res)
	return
}

func readFLACStream(f io.Reader) ([]byte, error) {
	result, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if result[0] != 0xFF || result[1]>>2 != 0x3E {
		return nil, ErrorNoSyncCode
	}
	return result, nil
}

func parseMetadataBlock(f io.Reader) (block *MetaDataBlock, isfinal bool, err error) {
	block = new(MetaDataBlock)
	header := make([]byte, 4)
	_, err = io.ReadFull(f, header)
	if err != nil {
		return
	}
	isfinal = header[0]>>7 != 0
	block.Type = BlockType(header[0] << 1 >> 1)
	var length uint32
	err = binary.Read(bytes.NewBuffer(header), binary.BigEndian, &length)
	if err != nil {
		return
	}
	length = length << 8 >> 8

	buf := make([]byte, length)
	_, err = io.ReadFull(f, buf)
	if err != nil {
		return
	}
	block.Data = buf

	return
}

func readMetadataBlocks(f io.Reader) (blocks []*MetaDataBlock, err error) {
	finishMetaData := false
	for !finishMetaData {
		var block *MetaDataBlock
		block, finishMetaData, err = parseMetadataBlock(f)
		if err != nil {
			return
		}
		blocks = append(blocks, block)
	}
	return
}

func readFLACHead(f io.Reader) error {
	buffer := make([]byte, 4)
	_, err := io.ReadFull(f, buffer)
	if err != nil {
		return err
	}

	// 支持跳过 FLAC 文件开头可能存在的 ID3v2 标签
	if string(buffer[:3]) == "ID3" {
		headerRest := make([]byte, 6)
		if _, err := io.ReadFull(f, headerRest); err != nil {
			return err
		}
		flags := headerRest[1]
		size := int32(headerRest[2]&0x7f)<<21 |
			int32(headerRest[3]&0x7f)<<14 |
			int32(headerRest[4]&0x7f)<<7 |
			int32(headerRest[5]&0x7f)
		if (flags & 0x10) != 0 {
			size += 10
		}
		discarded := 0
		discardBuf := make([]byte, 4096)
		for discarded < int(size) {
			toRead := int(size) - discarded
			if toRead > len(discardBuf) {
				toRead = len(discardBuf)
			}
			n, err := io.ReadFull(f, discardBuf[:toRead])
			discarded += n
			if err != nil {
				return err
			}
		}
		_, err = io.ReadFull(f, buffer)
		if err != nil {
			return err
		}
	}

	if string(buffer) != "fLaC" {
		return ErrorNoFLACHeader
	}
	return nil
}
