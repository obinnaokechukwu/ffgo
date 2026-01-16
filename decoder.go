//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"
	"time"

	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Decoder decodes media files.
type Decoder struct {
	mu sync.Mutex

	formatCtx     avformat.FormatContext
	videoCodecCtx avcodec.Context
	audioCodecCtx avcodec.Context
	codecCtx      avcodec.Context // Deprecated: use videoCodecCtx
	packet        avcodec.Packet
	frame         avutil.Frame

	videoStreamIdx int
	audioStreamIdx int

	videoInfo *StreamInfo
	audioInfo *StreamInfo

	videoDecoderOpen bool
	audioDecoderOpen bool
	closed           bool
}

// DecoderOptions configures decoder behavior.
type DecoderOptions struct {
	// Format hint (e.g., "mp4", "mkv") - optional
	Format string

	// FFmpeg options passed to avformat_open_input
	AVOptions map[string]string

	// Streams specifies which stream types to decode (nil = all streams)
	Streams []MediaType

	// HWDevice specifies the hardware device for hardware acceleration (e.g., "cuda", "vaapi")
	HWDevice string
}

// DecoderOption is a functional option for configuring a decoder.
type DecoderOption func(*DecoderOptions)

// WithFormat sets the format hint for the decoder.
func WithFormat(format string) DecoderOption {
	return func(o *DecoderOptions) {
		o.Format = format
	}
}

// WithStreams specifies which stream types to decode.
// Only the specified stream types will be available for decoding.
func WithStreams(types ...MediaType) DecoderOption {
	return func(o *DecoderOptions) {
		o.Streams = types
	}
}

// WithHWDevice enables hardware acceleration using the specified device.
// Common values: "cuda" (NVIDIA), "vaapi" (Linux VA-API), "videotoolbox" (macOS).
// Note: Hardware acceleration support depends on FFmpeg build and available hardware.
func WithHWDevice(device string) DecoderOption {
	return func(o *DecoderOptions) {
		o.HWDevice = device
	}
}

// WithAVOptions sets FFmpeg options passed to avformat_open_input.
func WithAVOptions(options map[string]string) DecoderOption {
	return func(o *DecoderOptions) {
		o.AVOptions = options
	}
}

// NewDecoder opens a media file for decoding.
// Optional functional options can be passed to configure the decoder.
func NewDecoder(path string, options ...DecoderOption) (*Decoder, error) {
	opts := &DecoderOptions{}
	for _, opt := range options {
		opt(opts)
	}
	return NewDecoderWithOptions(path, opts)
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

	// Build AVDictionary from options
	var avDict avutil.Dictionary
	if opts != nil && len(opts.AVOptions) > 0 {
		for key, value := range opts.AVOptions {
			if err := avutil.DictSet(&avDict, key, value, 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
	}

	// Open input file
	if err := avformat.OpenInput(&d.formatCtx, path, nil, &avDict); err != nil {
		if avDict != nil {
			avutil.DictFree(&avDict)
		}
		return nil, err
	}

	// Free any remaining dictionary entries (FFmpeg may have consumed some)
	if avDict != nil {
		avutil.DictFree(&avDict)
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

	// Get time base
	tbNum, tbDen := avformat.GetStreamTimeBase(stream)

	// Get codec name
	var codecName string
	if codec := avcodec.FindDecoder(codecID); codec != nil {
		codecName = avcodec.GetCodecName(codec)
	}

	info := &StreamInfo{
		Index:     streamIdx,
		Type:      codecType,
		CodecID:   codecID,
		CodecName: codecName,
		TimeBase:  avutil.NewRational(tbNum, tbDen),
		codecPar:  codecPar,
	}

	if codecType == avutil.MediaTypeVideo {
		info.Width = int(avformat.GetCodecParWidth(codecPar))
		info.Height = int(avformat.GetCodecParHeight(codecPar))
		info.PixelFmt = PixelFormat(avformat.GetCodecParFormat(codecPar))

		// Get frame rate
		frNum, frDen := avformat.GetStreamAvgFrameRate(stream)
		info.FrameRate = avutil.NewRational(frNum, frDen)
	} else if codecType == avutil.MediaTypeAudio {
		info.SampleRate = int(avformat.GetCodecParSampleRate(codecPar))
		info.Channels = int(avformat.GetCodecParChannels(codecPar))
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

// Duration returns the duration as time.Duration.
func (d *Decoder) Duration() time.Duration {
	us := d.DurationMicroseconds()
	if us <= 0 {
		return 0
	}
	return time.Duration(us) * time.Microsecond
}

// DurationMicroseconds returns the duration in microseconds (AV_TIME_BASE units).
func (d *Decoder) DurationMicroseconds() int64 {
	if d.formatCtx == nil {
		return 0
	}
	return avformat.GetDuration(d.formatCtx)
}

// DurationTime is an alias for Duration for backward compatibility.
// Deprecated: Use Duration() instead.
func (d *Decoder) DurationTime() time.Duration {
	return d.Duration()
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

	if d.videoDecoderOpen {
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
	d.videoCodecCtx = avcodec.AllocContext3(codec)
	if d.videoCodecCtx == nil {
		return errors.New("ffgo: failed to allocate codec context")
	}

	// Copy codec parameters
	if err := avcodec.ParametersToContext(d.videoCodecCtx, codecPar); err != nil {
		avcodec.FreeContext(&d.videoCodecCtx)
		return err
	}

	// Open codec
	if err := avcodec.Open2(d.videoCodecCtx, codec, nil); err != nil {
		avcodec.FreeContext(&d.videoCodecCtx)
		return err
	}

	d.codecCtx = d.videoCodecCtx // For backward compatibility
	d.videoDecoderOpen = true
	return nil
}

// OpenAudioDecoder opens a codec context for audio decoding.
func (d *Decoder) OpenAudioDecoder() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.audioStreamIdx < 0 {
		return errors.New("ffgo: no audio stream")
	}

	if d.audioDecoderOpen {
		return nil // Already opened
	}

	stream := avformat.GetStream(d.formatCtx, d.audioStreamIdx)
	codecPar := avformat.GetStreamCodecPar(stream)
	codecID := avformat.GetCodecParCodecID(codecPar)

	// Find decoder
	codec := avcodec.FindDecoder(codecID)
	if codec == nil {
		return errors.New("ffgo: audio decoder not found")
	}

	// Allocate codec context
	d.audioCodecCtx = avcodec.AllocContext3(codec)
	if d.audioCodecCtx == nil {
		return errors.New("ffgo: failed to allocate audio codec context")
	}

	// Copy codec parameters
	if err := avcodec.ParametersToContext(d.audioCodecCtx, codecPar); err != nil {
		avcodec.FreeContext(&d.audioCodecCtx)
		return err
	}

	// Open codec
	if err := avcodec.Open2(d.audioCodecCtx, codec, nil); err != nil {
		avcodec.FreeContext(&d.audioCodecCtx)
		return err
	}

	d.audioDecoderOpen = true
	return nil
}

// DecodeVideoPacket decodes a video packet and returns the decoded frame.
// Returns nil frame if more data is needed (EAGAIN) or on EOF.
// The returned frame is owned by the decoder; copy it if you need to keep it.
func (d *Decoder) DecodeVideoPacket(pkt Packet) (Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.videoDecoderOpen {
		return Frame{}, errors.New("ffgo: video decoder not opened; call OpenVideoDecoder first")
	}

	// Send packet to decoder
	if err := avcodec.SendPacket(d.videoCodecCtx, pkt); err != nil {
		return Frame{}, err
	}

	// Receive decoded frame
	avutil.FrameUnref(d.frame)
	err := avcodec.ReceiveFrame(d.videoCodecCtx, d.frame)
	if err != nil {
		if avutil.IsAgain(err) || avutil.IsEOF(err) {
			return Frame{}, nil
		}
		return Frame{}, err
	}

	return Frame{ptr: d.frame, owned: false}, nil
}

// DecodeVideoPacketCopy decodes a video packet and returns an owned frame.
//
// Unlike DecodeVideoPacket (which returns a decoder-owned, internally reused frame),
// this method returns a cloned frame that the caller MUST free with FrameFree.
// Returns (nil, nil) if more data is needed (EAGAIN) or on EOF.
func (d *Decoder) DecodeVideoPacketCopy(pkt Packet) (Frame, error) {
	frame, err := d.DecodeVideoPacket(pkt)
	if err != nil || frame.IsNil() {
		return Frame{}, err
	}
	return FrameClone(frame)
}

// DecodeAudioPacket decodes an audio packet and returns the decoded frame.
// Returns nil frame if more data is needed (EAGAIN) or on EOF.
// The returned frame is owned by the decoder; copy it if you need to keep it.
func (d *Decoder) DecodeAudioPacket(pkt Packet) (Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.audioDecoderOpen {
		return Frame{}, errors.New("ffgo: audio decoder not opened; call OpenAudioDecoder first")
	}

	// Send packet to decoder
	if err := avcodec.SendPacket(d.audioCodecCtx, pkt); err != nil {
		return Frame{}, err
	}

	// Receive decoded frame
	avutil.FrameUnref(d.frame)
	err := avcodec.ReceiveFrame(d.audioCodecCtx, d.frame)
	if err != nil {
		if avutil.IsAgain(err) || avutil.IsEOF(err) {
			return Frame{}, nil
		}
		return Frame{}, err
	}

	return Frame{ptr: d.frame, owned: false}, nil
}

// DecodeAudioPacketCopy decodes an audio packet and returns an owned frame.
//
// Unlike DecodeAudioPacket (which returns a decoder-owned, internally reused frame),
// this method returns a cloned frame that the caller MUST free with FrameFree.
// Returns (nil, nil) if more data is needed (EAGAIN) or on EOF.
func (d *Decoder) DecodeAudioPacketCopy(pkt Packet) (Frame, error) {
	frame, err := d.DecodeAudioPacket(pkt)
	if err != nil || frame.IsNil() {
		return Frame{}, err
	}
	return FrameClone(frame)
}

// DecodeVideo reads and decodes the next video frame.
// This is a convenience method that handles packet reading internally.
// The returned frame is owned by the decoder; do not call FrameFree on it.
// If you need to keep the frame beyond the next decode call, make a copy.
// Returns nil frame on EOF.
func (d *Decoder) DecodeVideo() (Frame, error) {
	if !d.videoDecoderOpen {
		if err := d.OpenVideoDecoder(); err != nil {
			return Frame{}, err
		}
	}

	for {
		streamIdx, pkt, err := d.ReadPacket()
		if err != nil {
			if avutil.IsEOF(err) {
				// Flush decoder
				frame, err := d.DecodeVideoPacket(nil)
				if err != nil || frame.IsNil() {
					return Frame{}, err
				}
				return frame, nil
			}
			return Frame{}, err
		}

		// Skip non-video packets
		if streamIdx != d.videoStreamIdx {
			continue
		}

		// Decode the packet
		frame, err := d.DecodeVideoPacket(pkt)
		if err != nil {
			return Frame{}, err
		}
		if !frame.IsNil() {
			return frame, nil
		}
		// Need more data, read next packet
	}
}

// DecodeVideoCopy reads and decodes the next video frame and returns an owned frame.
//
// The caller MUST free the returned frame with FrameFree.
// Returns nil frame on EOF.
func (d *Decoder) DecodeVideoCopy() (Frame, error) {
	frame, err := d.DecodeVideo()
	if err != nil || frame.IsNil() {
		return Frame{}, err
	}
	return FrameClone(frame)
}

// DecodeAudio reads and decodes the next audio frame.
// This is a convenience method that handles packet reading internally.
// The returned frame is owned by the decoder; do not call FrameFree on it.
// If you need to keep the frame beyond the next decode call, make a copy.
// Returns nil frame on EOF.
func (d *Decoder) DecodeAudio() (Frame, error) {
	if !d.audioDecoderOpen {
		if err := d.OpenAudioDecoder(); err != nil {
			return Frame{}, err
		}
	}

	for {
		streamIdx, pkt, err := d.ReadPacket()
		if err != nil {
			if avutil.IsEOF(err) {
				// Flush decoder
				frame, err := d.DecodeAudioPacket(nil)
				if err != nil || frame.IsNil() {
					return Frame{}, err
				}
				return frame, nil
			}
			return Frame{}, err
		}

		// Skip non-audio packets
		if streamIdx != d.audioStreamIdx {
			continue
		}

		// Decode the packet
		frame, err := d.DecodeAudioPacket(pkt)
		if err != nil {
			return Frame{}, err
		}
		if !frame.IsNil() {
			return frame, nil
		}
		// Need more data, read next packet
	}
}

// ReadFrame reads and decodes the next frame (video or audio).
// Returns a FrameWrapper with the MediaType set.
// The frame is owned by the decoder; call Copy() if you need to keep it.
// Returns nil, nil on EOF.
func (d *Decoder) ReadFrame() (*FrameWrapper, error) {
	// Open decoders if needed
	if d.HasVideo() && !d.videoDecoderOpen {
		if err := d.OpenVideoDecoder(); err != nil {
			return nil, err
		}
	}
	if d.HasAudio() && !d.audioDecoderOpen {
		if err := d.OpenAudioDecoder(); err != nil {
			return nil, err
		}
	}

	for {
		streamIdx, pkt, err := d.ReadPacket()
		if err != nil {
			if avutil.IsEOF(err) {
				// Flush video decoder first
				if d.videoDecoderOpen {
					frame, err := d.DecodeVideoPacket(nil)
					if err != nil {
						return nil, err
					}
					if !frame.IsNil() {
						return WrapFrame(frame, MediaTypeVideo), nil
					}
				}
				// Flush audio decoder
				if d.audioDecoderOpen {
					frame, err := d.DecodeAudioPacket(nil)
					if err != nil {
						return nil, err
					}
					if !frame.IsNil() {
						return WrapFrame(frame, MediaTypeAudio), nil
					}
				}
				return nil, nil // EOF
			}
			return nil, err
		}

		// Decode video packet
		if streamIdx == d.videoStreamIdx && d.videoDecoderOpen {
			frame, err := d.DecodeVideoPacket(pkt)
			if err != nil {
				return nil, err
			}
			if !frame.IsNil() {
				return WrapFrame(frame, MediaTypeVideo), nil
			}
		}

		// Decode audio packet
		if streamIdx == d.audioStreamIdx && d.audioDecoderOpen {
			frame, err := d.DecodeAudioPacket(pkt)
			if err != nil {
				return nil, err
			}
			if !frame.IsNil() {
				return WrapFrame(frame, MediaTypeAudio), nil
			}
		}
	}
}

// ReadFrameCopy reads and decodes the next frame (video or audio) and returns an owned frame wrapper.
//
// The returned wrapper owns its underlying frame; the caller MUST call Free() when done.
// Returns (nil, nil) on EOF.
func (d *Decoder) ReadFrameCopy() (*FrameWrapper, error) {
	fw, err := d.ReadFrame()
	if err != nil || fw == nil {
		return nil, err
	}
	return fw.Copy()
}

// FlushDecoder flushes all decoder buffers.
func (d *Decoder) FlushDecoder() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.videoCodecCtx != nil {
		avcodec.FlushBuffers(d.videoCodecCtx)
	}
	if d.audioCodecCtx != nil {
		avcodec.FlushBuffers(d.audioCodecCtx)
	}
}

// Seek seeks to a position in the file.
// The timestamp is specified as time.Duration from the start.
func (d *Decoder) Seek(ts time.Duration) error {
	return d.SeekTimestamp(ts.Microseconds())
}

// SeekTimestamp seeks to a position in the file.
// timestamp is in AV_TIME_BASE (microseconds).
func (d *Decoder) SeekTimestamp(timestamp int64) error {
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
	if d.videoCodecCtx != nil {
		avcodec.FlushBuffers(d.videoCodecCtx)
	}
	if d.audioCodecCtx != nil {
		avcodec.FlushBuffers(d.audioCodecCtx)
	}

	return nil
}

// SeekTime is an alias for Seek for backwards compatibility.
// Deprecated: Use Seek instead.
func (d *Decoder) SeekTime(dur time.Duration) error {
	return d.Seek(dur)
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

	// Free video codec context
	if d.videoCodecCtx != nil {
		avcodec.FreeContext(&d.videoCodecCtx)
	}

	// Free audio codec context
	if d.audioCodecCtx != nil {
		avcodec.FreeContext(&d.audioCodecCtx)
	}

	// Clear deprecated field
	d.codecCtx = nil

	// Close input
	if d.formatCtx != nil {
		avformat.CloseInput(&d.formatCtx)
	}

	return nil
}
