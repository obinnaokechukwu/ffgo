//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"

	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Decoder decodes media files.
type Decoder struct {
	mu sync.Mutex

	formatCtx avformat.FormatContext
	codecCtx  avcodec.Context
	packet    avcodec.Packet
	frame     avutil.Frame

	videoStreamIdx int
	audioStreamIdx int

	videoInfo *StreamInfo
	audioInfo *StreamInfo

	closed bool
}

// DecoderOptions configures decoder behavior.
type DecoderOptions struct {
	// Format hint (e.g., "mp4", "mkv") - optional
	Format string

	// FFmpeg options passed to avformat_open_input
	AVOptions map[string]string
}

// NewDecoder opens a media file for decoding.
func NewDecoder(path string) (*Decoder, error) {
	return NewDecoderWithOptions(path, nil)
}

// NewDecoderWithOptions opens a media file with custom options.
func NewDecoderWithOptions(path string, opts *DecoderOptions) (*Decoder, error) {
	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	d := &Decoder{
		videoStreamIdx: -1,
		audioStreamIdx: -1,
	}

	// Open input file
	if err := avformat.OpenInput(&d.formatCtx, path, nil, nil); err != nil {
		return nil, err
	}

	// Find stream info
	if err := avformat.FindStreamInfo(d.formatCtx, nil); err != nil {
		avformat.CloseInput(&d.formatCtx)
		return nil, err
	}

	// Find best video stream
	d.videoStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeVideo, -1, -1, nil, 0))
	if d.videoStreamIdx >= 0 {
		d.videoInfo = d.getStreamInfo(d.videoStreamIdx)
	}

	// Find best audio stream
	d.audioStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeAudio, -1, -1, nil, 0))
	if d.audioStreamIdx >= 0 {
		d.audioInfo = d.getStreamInfo(d.audioStreamIdx)
	}

	// Allocate packet and frame
	d.packet = avcodec.PacketAlloc()
	if d.packet == nil {
		d.Close()
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	d.frame = avutil.FrameAlloc()
	if d.frame == nil {
		d.Close()
		return nil, errors.New("ffgo: failed to allocate frame")
	}

	return d, nil
}

// getStreamInfo extracts stream information.
func (d *Decoder) getStreamInfo(streamIdx int) *StreamInfo {
	stream := avformat.GetStream(d.formatCtx, streamIdx)
	if stream == nil {
		return nil
	}

	codecPar := avformat.GetStreamCodecPar(stream)
	if codecPar == nil {
		return nil
	}

	codecType := avformat.GetCodecParType(codecPar)
	codecID := avformat.GetCodecParCodecID(codecPar)

	info := &StreamInfo{
		Index:   streamIdx,
		Type:    codecType,
		CodecID: codecID,
	}

	if codecType == avutil.MediaTypeVideo {
		info.Width = int(avformat.GetCodecParWidth(codecPar))
		info.Height = int(avformat.GetCodecParHeight(codecPar))
		info.PixelFmt = PixelFormat(avformat.GetCodecParFormat(codecPar))
	} else if codecType == avutil.MediaTypeAudio {
		info.SampleRate = int(avformat.GetCodecParSampleRate(codecPar))
	}

	return info
}

// VideoStream returns information about the video stream.
// Returns nil if no video stream is present.
func (d *Decoder) VideoStream() *StreamInfo {
	return d.videoInfo
}

// AudioStream returns information about the audio stream.
// Returns nil if no audio stream is present.
func (d *Decoder) AudioStream() *StreamInfo {
	return d.audioInfo
}

// HasVideo returns true if the file has a video stream.
func (d *Decoder) HasVideo() bool {
	return d.videoStreamIdx >= 0
}

// HasAudio returns true if the file has an audio stream.
func (d *Decoder) HasAudio() bool {
	return d.audioStreamIdx >= 0
}

// NumStreams returns the total number of streams.
func (d *Decoder) NumStreams() int {
	if d.formatCtx == nil {
		return 0
	}
	return avformat.GetNumStreams(d.formatCtx)
}

// Duration returns the duration in microseconds.
func (d *Decoder) Duration() int64 {
	if d.formatCtx == nil {
		return 0
	}
	return avformat.GetDuration(d.formatCtx)
}

// BitRate returns the bit rate.
func (d *Decoder) BitRate() int64 {
	if d.formatCtx == nil {
		return 0
	}
	return avformat.GetBitRate(d.formatCtx)
}

// ReadPacket reads the next packet from the file.
// Returns the packet stream index and the packet.
// When EOF is reached, returns -1 and an EOF error.
func (d *Decoder) ReadPacket() (streamIdx int, pkt Packet, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return -1, nil, errors.New("ffgo: decoder is closed")
	}

	// Unref previous packet
	avcodec.PacketUnref(d.packet)

	// Read next packet
	if err := avformat.ReadFrame(d.formatCtx, d.packet); err != nil {
		return -1, nil, err
	}

	idx := int(avcodec.GetPacketStreamIndex(d.packet))
	return idx, d.packet, nil
}

// OpenVideoDecoder opens a codec context for video decoding.
// Must be called before DecodeVideoPacket.
func (d *Decoder) OpenVideoDecoder() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.videoStreamIdx < 0 {
		return errors.New("ffgo: no video stream")
	}

	if d.codecCtx != nil {
		return nil // Already opened
	}

	stream := avformat.GetStream(d.formatCtx, d.videoStreamIdx)
	codecPar := avformat.GetStreamCodecPar(stream)
	codecID := avformat.GetCodecParCodecID(codecPar)

	// Find decoder
	codec := avcodec.FindDecoder(codecID)
	if codec == nil {
		return errors.New("ffgo: decoder not found")
	}

	// Allocate codec context
	d.codecCtx = avcodec.AllocContext3(codec)
	if d.codecCtx == nil {
		return errors.New("ffgo: failed to allocate codec context")
	}

	// Copy codec parameters
	if err := avcodec.ParametersToContext(d.codecCtx, codecPar); err != nil {
		avcodec.FreeContext(&d.codecCtx)
		return err
	}

	// Open codec
	if err := avcodec.Open2(d.codecCtx, codec, nil); err != nil {
		avcodec.FreeContext(&d.codecCtx)
		return err
	}

	return nil
}

// DecodeVideoPacket decodes a video packet and returns the decoded frame.
// Returns nil frame if more data is needed (EAGAIN) or on EOF.
// The returned frame is owned by the decoder; copy it if you need to keep it.
func (d *Decoder) DecodeVideoPacket(pkt Packet) (Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.codecCtx == nil {
		return nil, errors.New("ffgo: video decoder not opened; call OpenVideoDecoder first")
	}

	// Send packet to decoder
	if err := avcodec.SendPacket(d.codecCtx, pkt); err != nil {
		return nil, err
	}

	// Receive decoded frame
	avutil.FrameUnref(d.frame)
	err := avcodec.ReceiveFrame(d.codecCtx, d.frame)
	if err != nil {
		if avutil.IsAgain(err) || avutil.IsEOF(err) {
			return nil, nil
		}
		return nil, err
	}

	return d.frame, nil
}

// DecodeVideo reads and decodes the next video frame.
// This is a convenience method that handles packet reading internally.
// Returns nil frame on EOF.
func (d *Decoder) DecodeVideo() (Frame, error) {
	if d.codecCtx == nil {
		if err := d.OpenVideoDecoder(); err != nil {
			return nil, err
		}
	}

	for {
		streamIdx, pkt, err := d.ReadPacket()
		if err != nil {
			if avutil.IsEOF(err) {
				// Flush decoder
				frame, err := d.DecodeVideoPacket(nil)
				if err != nil || frame == nil {
					return nil, err
				}
				return frame, nil
			}
			return nil, err
		}

		// Skip non-video packets
		if streamIdx != d.videoStreamIdx {
			continue
		}

		// Decode the packet
		frame, err := d.DecodeVideoPacket(pkt)
		if err != nil {
			return nil, err
		}
		if frame != nil {
			return frame, nil
		}
		// Need more data, read next packet
	}
}

// FlushDecoder flushes the decoder buffers.
func (d *Decoder) FlushDecoder() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.codecCtx != nil {
		avcodec.FlushBuffers(d.codecCtx)
	}
}

// Seek seeks to a position in the file.
// timestamp is in AV_TIME_BASE (microseconds).
func (d *Decoder) Seek(timestamp int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return errors.New("ffgo: decoder is closed")
	}

	// Seek to keyframe before target
	if err := avformat.SeekFrame(d.formatCtx, -1, timestamp, avformat.SeekFlagBackward); err != nil {
		return err
	}

	// Flush decoder buffers
	if d.codecCtx != nil {
		avcodec.FlushBuffers(d.codecCtx)
	}

	return nil
}

// Close releases all resources.
func (d *Decoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	// Free frame
	if d.frame != nil {
		avutil.FrameFree(&d.frame)
	}

	// Free packet
	if d.packet != nil {
		avcodec.PacketFree(&d.packet)
	}

	// Free codec context
	if d.codecCtx != nil {
		avcodec.FreeContext(&d.codecCtx)
	}

	// Close input
	if d.formatCtx != nil {
		avformat.CloseInput(&d.formatCtx)
	}

	return nil
}
