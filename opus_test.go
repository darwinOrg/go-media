package dgmedia

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

func TestConvertOpusDataToOgg(t *testing.T) {
	data, _ := os.ReadFile("1.opus")
	dataLength := len(data)
	oggBuffer := bytes.NewBuffer(nil)
	oggWriter, _ := NewOggWriterWith(oggBuffer, 16000, 1)
	var timestamp uint32
	var startIndex int
	for {
		if dataLength <= startIndex+2 {
			break
		}

		// 提取前两个字节并转换为长度
		length := int(binary.BigEndian.Uint16(data[startIndex : startIndex+2]))
		if length == 0 {
			break
		}

		startIndex += 2
		timestamp += 4000
		err := oggWriter.WritePayload(data[startIndex:startIndex+length], timestamp)
		if err != nil {
			panic(err)
		}

		startIndex = startIndex + length
	}

	err := oggWriter.Close()
	if err != nil {
		panic(err)
	}
	err = os.WriteFile("1.ogg", oggBuffer.Bytes(), os.ModePerm)
	if err != nil {
		panic(err)
	}
}
