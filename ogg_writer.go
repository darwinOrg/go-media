package dgmedia

import (
	"encoding/binary"
	"errors"
	"github.com/darwinOrg/go-common/utils"
	"io"
	"os"
)

const (
	pageHeaderTypeContinuationOfStream = 0x00
	pageHeaderTypeEndOfStream          = 0x04
	defaultPreSkip                     = 3840 // 3840 recommended in the RFC
	commentPageSignature               = "OpusTags"
	pageHeaderSize                     = 27
)

var (
	errFileNotOpened        = errors.New("file not opened")
	VendorName              = "star"
	VendorLength     uint32 = 4
)

// OggWriter is used to take RTP packets and write them to an OGG on disk
type OggWriter struct {
	stream                  io.Writer
	fd                      *os.File
	sampleRate              uint32
	channelCount            uint16
	serial                  uint32
	pageIndex               uint32
	checksumTable           *[256]uint32
	previousGranulePosition uint64
	previousTimestamp       uint32
	lastPayloadSize         int
}

// New builds a new OGG Opus writer
func NewOggWriter(fileName string, sampleRate uint32, channelCount uint16) (*OggWriter, error) {
	f, err := os.Create(fileName) //nolint:gosec
	if err != nil {
		return nil, err
	}
	writer, err := NewOggWriterWith(f, sampleRate, channelCount)
	if err != nil {
		return nil, f.Close()
	}
	writer.fd = f
	return writer, nil
}

// NewWith initialize a new OGG Opus writer with an io.Writer output
func NewOggWriterWith(out io.Writer, sampleRate uint32, channelCount uint16) (*OggWriter, error) {
	if out == nil {
		return nil, errFileNotOpened
	}

	writer := &OggWriter{
		stream:        out,
		sampleRate:    sampleRate,
		channelCount:  channelCount,
		serial:        utils.NewMathRandomGenerator().Uint32(),
		checksumTable: generateChecksumTable(),

		// Timestamp and Granule MUST start from 1
		// Only headers can have 0 values
		previousTimestamp:       1,
		previousGranulePosition: 1,
	}
	if err := writer.writeHeaders(); err != nil {
		return nil, err
	}

	return writer, nil
}

/*
    ref: https://tools.ietf.org/html/rfc7845.html
    https://git.xiph.org/?p=opus-tools.git;a=blob;f=src/opus_header.c#l219

       Page 0         Pages 1 ... n        Pages (n+1) ...
    +------------+ +---+ +---+ ... +---+ +-----------+ +---------+ +--
    |            | |   | |   |     |   | |           | |         | |
    |+----------+| |+-----------------+| |+-------------------+ +-----
    |||ID Header|| ||  Comment Header || ||Audio Data Packet 1| | ...
    |+----------+| |+-----------------+| |+-------------------+ +-----
    |            | |   | |   |     |   | |           | |         | |
    +------------+ +---+ +---+ ... +---+ +-----------+ +---------+ +--
    ^      ^                           ^
    |      |                           |
    |      |                           Mandatory Page Break
    |      |
    |      ID header is contained on a single page
    |
    'Beginning Of Stream'

   Figure 1: Example Packet Organization for a Logical Ogg Opus Stream
*/

func (i *OggWriter) writeHeaders() error {
	// ID Header
	oggIDHeader := make([]byte, 19)

	copy(oggIDHeader[0:], idPageSignature)                          // Magic Signature 'OpusHead'
	oggIDHeader[8] = 1                                              // Version
	oggIDHeader[9] = uint8(i.channelCount)                          // Channel count
	binary.LittleEndian.PutUint16(oggIDHeader[10:], defaultPreSkip) // pre-skip
	binary.LittleEndian.PutUint32(oggIDHeader[12:], i.sampleRate)   // original sample rate, any valid sample e.g 48000
	binary.LittleEndian.PutUint16(oggIDHeader[16:], 0)              // output gain
	oggIDHeader[18] = 0                                             // channel map 0 = one stream: mono or stereo

	// Reference: https://tools.ietf.org/html/rfc7845.html#page-6
	// RFC specifies that the ID Header page should have a granule position of 0 and a Header Type set to 2 (StartOfStream)
	data := i.createPage(oggIDHeader, pageHeaderTypeBeginningOfStream, 0, i.pageIndex)
	if err := i.writeToStream(data); err != nil {
		return err
	}
	i.pageIndex++

	// Comment Header
	oggCommentHeader := make([]byte, 21)
	copy(oggCommentHeader[0:], commentPageSignature)                  // Magic Signature 'OpusTags'
	binary.LittleEndian.PutUint32(oggCommentHeader[8:], VendorLength) // Vendor Length
	copy(oggCommentHeader[12:], VendorName)                           // Vendor name
	binary.LittleEndian.PutUint32(oggCommentHeader[17:], 0)           // User Comment List Length

	// RFC specifies that the page where the CommentHeader completes should have a granule position of 0
	data = i.createPage(oggCommentHeader, pageHeaderTypeContinuationOfStream, 0, i.pageIndex)
	if err := i.writeToStream(data); err != nil {
		return err
	}
	i.pageIndex++

	return nil
}

func (i *OggWriter) createPage(payload []uint8, headerType uint8, granulePos uint64, pageIndex uint32) []byte {
	i.lastPayloadSize = len(payload)
	nSegments := (len(payload) / 255) + 1 // A segment can be at most 255 bytes long.

	page := make([]byte, pageHeaderSize+i.lastPayloadSize+nSegments)

	copy(page[0:], pageHeaderSignature)                 // page headers starts with 'OggS'
	page[4] = 0                                         // Version
	page[5] = headerType                                // 1 = continuation, 2 = beginning of stream, 4 = end of stream
	binary.LittleEndian.PutUint64(page[6:], granulePos) // granule position
	binary.LittleEndian.PutUint32(page[14:], i.serial)  // Bitstream serial number
	binary.LittleEndian.PutUint32(page[18:], pageIndex) // Page sequence number
	page[26] = uint8(nSegments)                         // Number of segments in page.

	// Filling segment table with the lacing values.
	// First (nSegments - 1) values will always be 255.
	for i := 0; i < nSegments-1; i++ {
		page[pageHeaderSize+i] = 255
	}
	// The last value will be the remainder.
	page[pageHeaderSize+nSegments-1] = uint8(len(payload) % 255)

	copy(page[pageHeaderSize+nSegments:], payload) // Payload goes after the segment table, so at pageHeaderSize+nSegments.

	var checksum uint32
	for index := range page {
		checksum = (checksum << 8) ^ i.checksumTable[byte(checksum>>24)^page[index]]
	}

	binary.LittleEndian.PutUint32(page[22:], checksum) // Checksum - generating for page data and inserting at 22th position into 32 bits

	return page
}

func (i *OggWriter) WritePayload(payload []byte, timestamp uint32) error {
	if len(payload) == 0 {
		return nil
	}

	// Should be equivalent to sampleRate * duration
	if i.previousTimestamp != 1 {
		increment := timestamp - i.previousTimestamp
		i.previousGranulePosition += uint64(increment)
	}
	i.previousTimestamp = timestamp

	data := i.createPage(payload, pageHeaderTypeContinuationOfStream, i.previousGranulePosition, i.pageIndex)
	i.pageIndex++
	return i.writeToStream(data)
}

// Close stops the recording
func (i *OggWriter) Close() error {
	defer func() {
		i.fd = nil
		i.stream = nil
	}()

	// Returns no error has it may be convenient to call
	// Close() multiple times
	if i.fd == nil {
		// Close stream if we are operating on a stream
		if closer, ok := i.stream.(io.Closer); ok {
			return closer.Close()
		}
		return nil
	}

	// Seek back one page, we need to update the header and generate new CRC
	pageOffset, err := i.fd.Seek(-1*int64(i.lastPayloadSize+pageHeaderSize+1), 2)
	if err != nil {
		return err
	}

	payload := make([]byte, i.lastPayloadSize)
	if _, err := i.fd.ReadAt(payload, pageOffset+pageHeaderSize+1); err != nil {
		return err
	}

	data := i.createPage(payload, pageHeaderTypeEndOfStream, i.previousGranulePosition, i.pageIndex-1)
	if err := i.writeToStream(data); err != nil {
		return err
	}

	// Update the last page if we are operating on files
	// to mark it as the EOS
	return i.fd.Close()
}

// Wraps writing to the stream and maintains state
// so we can set values for EOS
func (i *OggWriter) writeToStream(p []byte) error {
	if i.stream == nil {
		return errFileNotOpened
	}

	_, err := i.stream.Write(p)
	return err
}

func generateChecksumTable() *[256]uint32 {
	var table [256]uint32
	const poly = 0x04c11db7

	for i := range table {
		r := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if (r & 0x80000000) != 0 {
				r = (r << 1) ^ poly
			} else {
				r <<= 1
			}
			table[i] = r & 0xffffffff
		}
	}
	return &table
}
