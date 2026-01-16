//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// SubtitleType represents the type of subtitle content.
type SubtitleType int

const (
	// SubtitleTypeNone indicates no subtitle data.
	SubtitleTypeNone SubtitleType = iota
	// SubtitleTypeBitmap indicates a graphical/bitmap subtitle (e.g., DVD subtitles).
	SubtitleTypeBitmap
	// SubtitleTypeText indicates plain text subtitles.
	SubtitleTypeText
	// SubtitleTypeASS indicates Advanced SubStation Alpha formatted text.
	SubtitleTypeASS
)

// Subtitle represents a decoded subtitle.
type Subtitle struct {
	// StartTime is when the subtitle should appear.
	StartTime time.Duration
	// EndTime is when the subtitle should disappear.
	EndTime time.Duration
	// PTS is the presentation timestamp.
	PTS int64
	// Type indicates the subtitle format.
	Type SubtitleType
	// Text is the subtitle text (for text/ASS subtitles).
	Text string
	// Rects contains subtitle rectangles for bitmap subtitles.
	Rects []SubtitleRect
}

// SubtitleRect represents a rectangular area for bitmap subtitles.
type SubtitleRect struct {
	X, Y          int
	Width, Height int
	// Data contains bitmap indices for SubtitleTypeBitmap (palette-based).
	// Length is LineSize*Height.
	Data []byte
	// LineSize is the number of bytes per row in Data.
	LineSize int
	// Palette contains BGRA palette entries (4 bytes per color), if available.
	Palette []byte
}

// SubtitleDecoder decodes subtitles from a media file.
type SubtitleDecoder struct {
	mu sync.Mutex

	codecCtx       avcodec.Context
	subtitleStreamIdx int
	streamInfo     *StreamInfo

	// AVSubtitle struct for decoding
	subtitle unsafe.Pointer

	closed bool
}

// NewSubtitleDecoder creates a decoder for subtitle streams.
//
// The stream should come from Decoder.SubtitleStream().
func NewSubtitleDecoder(stream *StreamInfo) (*SubtitleDecoder, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	if stream == nil {
		return nil, errors.New("ffgo: subtitle stream cannot be nil")
	}
	if stream.Type != MediaTypeSubtitle {
		return nil, errors.New("ffgo: stream is not a subtitle stream")
	}
	codecPar := stream.CodecParameters()
	if codecPar == nil {
		return nil, errors.New("ffgo: subtitle stream codec parameters are not available")
	}
	codecID := avformat.GetCodecParCodecID(codecPar)

	// Find decoder
	decoder := avcodec.FindDecoder(codecID)
	if decoder == nil {
		return nil, errors.New("ffgo: subtitle decoder not found")
	}

	// Allocate codec context
	codecCtx := avcodec.AllocContext3(decoder)
	if codecCtx == nil {
		return nil, errors.New("ffgo: failed to allocate codec context")
	}

	// Copy parameters
	if err := avcodec.ParametersToContext(codecCtx, codecPar); err != nil {
		avcodec.FreeContext(&codecCtx)
		return nil, err
	}

	// Open decoder
	if err := avcodec.Open2(codecCtx, decoder, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		return nil, err
	}

	// Allocate AVSubtitle struct (32 bytes)
	subtitle := avutil.Malloc(32)
	if subtitle == nil {
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		return nil, errors.New("ffgo: failed to allocate subtitle struct")
	}

	return &SubtitleDecoder{
		codecCtx:          codecCtx,
		subtitleStreamIdx: stream.Index,
		streamInfo:        stream,
		subtitle:          subtitle,
	}, nil
}

// NewSubtitleDecoderFromFile is a convenience that opens a file and selects its best subtitle stream.
// Deprecated: prefer using Decoder + NewSubtitleDecoder(decoder.SubtitleStream()) as documented.
func NewSubtitleDecoderFromFile(inputPath string) (*SubtitleDecoder, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	var formatCtx avformat.FormatContext
	if err := avformat.OpenInput(&formatCtx, inputPath, nil, nil); err != nil {
		return nil, err
	}
	defer avformat.CloseInput(&formatCtx)

	if err := avformat.FindStreamInfo(formatCtx, nil); err != nil {
		return nil, err
	}

	subIdx := avformat.FindBestStream(formatCtx, avutil.MediaTypeSubtitle, -1, -1, nil, 0)
	if subIdx < 0 {
		return nil, errors.New("ffgo: no subtitle stream found")
	}

	// Build a minimal StreamInfo with codec parameters so NewSubtitleDecoder can work.
	stream := avformat.GetStream(formatCtx, int(subIdx))
	codecPar := avformat.GetStreamCodecPar(stream)
	tbNum, tbDen := avformat.GetStreamTimeBase(stream)
	si := &StreamInfo{
		Index:    int(subIdx),
		Type:     MediaTypeSubtitle,
		CodecID:  CodecID(avformat.GetCodecParCodecID(codecPar)),
		TimeBase: Rational{Num: tbNum, Den: tbDen},
		codecPar: codecPar,
	}
	return NewSubtitleDecoder(si)
}

// StreamInfo returns information about the subtitle stream.
func (d *SubtitleDecoder) StreamInfo() *StreamInfo {
	return d.streamInfo
}

// Decode decodes a subtitle from a packet.
// Returns (nil, nil) if no subtitle was decoded from this packet.
func (d *SubtitleDecoder) Decode(packet *Packet) (*Subtitle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, errors.New("ffgo: decoder is closed")
	}

	if packet == nil || packet.ptr == nil {
		return nil, nil
	}
	if packet.StreamIndex() != d.subtitleStreamIdx {
		return nil, nil
	}

	// Clear the subtitle struct
	clearSubtitle(d.subtitle)

	// Decode subtitle using avcodec_decode_subtitle2
	gotSub, err := avcodec.DecodeSubtitle2(d.codecCtx, d.subtitle, packet.ptr)
	if err != nil {
		return nil, err
	}
	if !gotSub {
		return nil, nil
	}

	// Parse the decoded subtitle
	sub := parseSubtitle(d.subtitle)

	// Free subtitle resources
	avcodec.SubtitleFree(d.subtitle)

	return sub, nil
}

// Close releases all resources.
func (d *SubtitleDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	if d.subtitle != nil {
		avutil.Free(d.subtitle)
	}
	if d.codecCtx != nil {
		avcodec.Close(d.codecCtx)
		avcodec.FreeContext(&d.codecCtx)
	}

	return nil
}

// AVSubtitle struct offsets (FFmpeg 6.x/7.x)
const (
	offsetSubFormat           = 0  // uint32_t format
	offsetSubStartDisplayTime = 4  // uint32_t start_display_time
	offsetSubEndDisplayTime   = 8  // uint32_t end_display_time
	offsetSubNumRects         = 12 // unsigned num_rects
	offsetSubRects            = 16 // AVSubtitleRect **rects
	offsetSubPTS              = 24 // int64_t pts
)

// AVSubtitleRect struct offsets
const (
	offsetRectX        = 0  // int x
	offsetRectY        = 4  // int y
	offsetRectW        = 8  // int w
	offsetRectH        = 12 // int h
	offsetRectNbColors = 16 // int nb_colors
	offsetRectData0    = 24 // uint8_t *data[0]
	offsetRectData1    = 32 // uint8_t *data[1] (palette)
	offsetRectLinesize0 = 56 // int linesize[0]
	offsetRectType     = 72 // enum AVSubtitleType type
	offsetRectText     = 80 // char *text
	offsetRectASS      = 88 // char *ass
)

// AVSubtitleType constants
const (
	subtitleTypeNone   = 0
	subtitleTypeBitmap = 1
	subtitleTypeText   = 2
	subtitleTypeASS    = 3
)

// clearSubtitle zeroes out the AVSubtitle struct.
func clearSubtitle(sub unsafe.Pointer) {
	for i := 0; i < 32; i++ {
		*(*byte)(unsafe.Pointer(uintptr(sub) + uintptr(i))) = 0
	}
}

// parseSubtitle extracts data from AVSubtitle into our Subtitle type.
func parseSubtitle(sub unsafe.Pointer) *Subtitle {
	startTime := *(*uint32)(unsafe.Pointer(uintptr(sub) + offsetSubStartDisplayTime))
	endTime := *(*uint32)(unsafe.Pointer(uintptr(sub) + offsetSubEndDisplayTime))
	pts := *(*int64)(unsafe.Pointer(uintptr(sub) + offsetSubPTS))
	numRects := *(*uint32)(unsafe.Pointer(uintptr(sub) + offsetSubNumRects))

	result := &Subtitle{
		StartTime: time.Duration(startTime) * time.Millisecond,
		EndTime:   time.Duration(endTime) * time.Millisecond,
		PTS:       pts,
	}

	if numRects == 0 {
		return result
	}

	// Get rectangles
	rectsPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(sub) + offsetSubRects))
	if rectsPtr == nil {
		return result
	}

	rectsArray := (*[1024]unsafe.Pointer)(rectsPtr)
	for i := uint32(0); i < numRects && i < 1024; i++ {
		rectPtr := rectsArray[i]
		if rectPtr == nil {
			continue
		}

		rectType := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectType))

		switch rectType {
		case subtitleTypeText:
			result.Type = SubtitleTypeText
			textPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(rectPtr) + offsetRectText))
			if textPtr != nil {
				result.Text = goString(textPtr)
			}
		case subtitleTypeASS:
			result.Type = SubtitleTypeASS
			assPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(rectPtr) + offsetRectASS))
			if assPtr != nil {
				result.Text = goString(assPtr)
			}
		case subtitleTypeBitmap:
			result.Type = SubtitleTypeBitmap
			x := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectX))
			y := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectY))
			w := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectW))
			h := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectH))
			ls0 := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectLinesize0))

			var data []byte
			if w > 0 && h > 0 && ls0 > 0 {
				dataPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(rectPtr) + offsetRectData0))
				// Copy bitmap indices: linesize*height bytes
				if dataPtr != nil {
					n := int(ls0) * int(h)
					// Safety cap to avoid runaway allocations on corrupt files.
					if n > 0 && n <= 64*1024*1024 {
						data = make([]byte, n)
						copy(data, unsafe.Slice((*byte)(dataPtr), n))
					}
				}
			}

			var palette []byte
			palPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(rectPtr) + offsetRectData1))
			nbColors := *(*int32)(unsafe.Pointer(uintptr(rectPtr) + offsetRectNbColors))
			if palPtr != nil && nbColors > 0 && nbColors <= 256 {
				n := int(nbColors) * 4
				palette = make([]byte, n)
				copy(palette, unsafe.Slice((*byte)(palPtr), n))
			}

			result.Rects = append(result.Rects, SubtitleRect{
				X:        int(x),
				Y:        int(y),
				Width:    int(w),
				Height:   int(h),
				Data:     data,
				LineSize: int(ls0),
				Palette:  palette,
			})
		}
	}

	return result
}

// goString converts a C string to Go string.
func goString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(length)))
		if b == 0 {
			break
		}
		length++
		if length > 4096 {
			break
		}
	}
	if length == 0 {
		return ""
	}
	return string((*[4096]byte)(ptr)[:length:length])
}

// HasSubtitle returns true if the decoder has a subtitle stream.
func (d *Decoder) HasSubtitle() bool {
	numStreams := avformat.GetNumStreams(d.formatCtx)
	for i := 0; i < numStreams; i++ {
		stream := avformat.GetStream(d.formatCtx, i)
		codecPar := avformat.GetStreamCodecPar(stream)
		mediaType := avformat.GetCodecParType(codecPar)
		if mediaType == avutil.MediaTypeSubtitle {
			return true
		}
	}
	return false
}

// SubtitleStream returns information about the first subtitle stream, or nil if none.
func (d *Decoder) SubtitleStream() *StreamInfo {
	numStreams := avformat.GetNumStreams(d.formatCtx)
	for i := 0; i < numStreams; i++ {
		stream := avformat.GetStream(d.formatCtx, i)
		codecPar := avformat.GetStreamCodecPar(stream)
		mediaType := avformat.GetCodecParType(codecPar)
		if mediaType == avutil.MediaTypeSubtitle {
			// Use the shared stream info extractor to ensure codec parameters are present.
			return d.getStreamInfo(i)
		}
	}
	return nil
}

// SubtitleStreamIndex returns the index of the first subtitle stream, or -1 if none.
func (d *Decoder) SubtitleStreamIndex() int {
	s := d.SubtitleStream()
	if s == nil {
		return -1
	}
	return s.Index
}
