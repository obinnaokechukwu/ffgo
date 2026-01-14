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

// Muxer combines multiple streams into a container.
// It provides low-level control over muxing, allowing stream copy mode
// or encoding with multiple audio/subtitle tracks.
type Muxer struct {
	mu            sync.Mutex
	formatCtx     avformat.FormatContext
	ioCtx         avformat.IOContext
	streams       []*MuxerStream
	headerWritten bool
	path          string
	closed        bool
}

// MuxerStream represents a stream being muxed.
type MuxerStream struct {
	muxer     *Muxer
	stream    avformat.Stream
	codecCtx  avcodec.Context
	index     int
	timeBase  Rational
	mediaType MediaType
	encoder   *streamEncoder // nil for copy mode
	copyMode  bool
}

// streamEncoder handles encoding for a muxer stream.
type streamEncoder struct {
	codecCtx avcodec.Context
	packet   Packet
	frame    Frame // reusable frame for format conversion if needed
}

// NewMuxer creates a muxer for the given output path and format.
// The format parameter is the FFmpeg mux format name (e.g., "matroska", "mp4", "avi").
// If format is empty, it will be guessed from the file extension.
func NewMuxer(path string, format string) (*Muxer, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	if path == "" {
		return nil, errors.New("ffgo: output path cannot be empty")
	}

	// If format not specified, guess from extension
	if format == "" {
		format = guessFormatFromPath(path)
	}
	if format == "" {
		return nil, errors.New("ffgo: cannot determine output format")
	}

	m := &Muxer{
		path:    path,
		streams: make([]*MuxerStream, 0),
	}

	// Create output format context
	if err := avformat.AllocOutputContext2(&m.formatCtx, nil, format, path); err != nil {
		return nil, err
	}

	return m, nil
}

// VideoStreamConfig configures a video stream for the muxer.
type VideoStreamConfig struct {
	Codec       CodecID     // Video codec (e.g., CodecIDH264)
	Width       int         // Video width
	Height      int         // Video height
	PixelFormat PixelFormat // Pixel format (default: YUV420P)
	FrameRate   int         // Frame rate in fps
	BitRate     int64       // Bitrate in bits/second
	GOPSize     int         // GOP size (keyframe interval)
}

// AddVideoStream adds a video stream to the muxer with encoding.
func (m *Muxer) AddVideoStream(config *VideoStreamConfig) (*MuxerStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("ffgo: muxer is closed")
	}
	if m.headerWritten {
		return nil, errors.New("ffgo: cannot add streams after header is written")
	}
	if config == nil {
		return nil, errors.New("ffgo: video config is required")
	}

	// Apply defaults
	if config.Codec == CodecIDNone {
		config.Codec = CodecIDH264
	}
	if config.PixelFormat == PixelFormatNone {
		config.PixelFormat = PixelFormatYUV420P
	}
	if config.FrameRate <= 0 {
		config.FrameRate = 30
	}
	if config.BitRate <= 0 {
		config.BitRate = 2000000
	}
	if config.GOPSize <= 0 {
		config.GOPSize = 12
	}

	// Find encoder
	codec := avcodec.FindEncoder(config.Codec)
	if codec == nil {
		return nil, errors.New("ffgo: video encoder not found")
	}

	// Create stream
	stream := avformat.NewStream(m.formatCtx, codec)
	if stream == nil {
		return nil, errors.New("ffgo: failed to create video stream")
	}

	// Create codec context
	codecCtx := avcodec.AllocContext3(codec)
	if codecCtx == nil {
		return nil, errors.New("ffgo: failed to allocate video codec context")
	}

	// Configure codec
	avcodec.SetCtxWidth(codecCtx, int32(config.Width))
	avcodec.SetCtxHeight(codecCtx, int32(config.Height))
	avcodec.SetCtxPixFmt(codecCtx, int32(config.PixelFormat))
	avcodec.SetCtxTimeBase(codecCtx, 1, int32(config.FrameRate))
	avcodec.SetCtxFramerate(codecCtx, int32(config.FrameRate), 1)
	avcodec.SetCtxBitRate(codecCtx, config.BitRate)
	avcodec.SetCtxGopSize(codecCtx, int32(config.GOPSize))

	// Set global header flag (commonly needed for containers like MP4)
	flags := avcodec.GetCtxFlags(codecCtx)
	avcodec.SetCtxFlags(codecCtx, flags|avcodec.CodecFlagGlobalHeader)

	// Open codec
	if err := avcodec.Open2(codecCtx, codec, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		return nil, err
	}

	// Copy parameters to stream
	codecPar := avformat.GetStreamCodecPar(stream)
	if err := avcodec.ParametersFromContext(codecPar, codecCtx); err != nil {
		avcodec.FreeContext(&codecCtx)
		return nil, err
	}

	ms := &MuxerStream{
		muxer:     m,
		stream:    stream,
		codecCtx:  codecCtx,
		index:     len(m.streams),
		timeBase:  NewRational(1, int32(config.FrameRate)),
		mediaType: MediaTypeVideo,
		encoder: &streamEncoder{
			codecCtx: codecCtx,
			packet:   avcodec.PacketAlloc(),
		},
	}

	m.streams = append(m.streams, ms)
	return ms, nil
}

// AudioStreamConfig configures an audio stream for the muxer.
type AudioStreamConfig struct {
	Codec        CodecID      // Audio codec (e.g., CodecIDAAC)
	SampleRate   int          // Sample rate in Hz
	Channels     int          // Number of channels
	SampleFormat SampleFormat // Sample format
	BitRate      int64        // Bitrate in bits/second
}

// AddAudioStream adds an audio stream to the muxer with encoding.
func (m *Muxer) AddAudioStream(config *AudioStreamConfig) (*MuxerStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("ffgo: muxer is closed")
	}
	if m.headerWritten {
		return nil, errors.New("ffgo: cannot add streams after header is written")
	}
	if config == nil {
		return nil, errors.New("ffgo: audio config is required")
	}

	// Apply defaults
	if config.Codec == CodecIDNone {
		config.Codec = CodecIDAAC
	}
	if config.SampleRate <= 0 {
		config.SampleRate = 48000
	}
	if config.Channels <= 0 {
		config.Channels = 2
	}
	if config.SampleFormat == SampleFormatNone {
		config.SampleFormat = SampleFormatFltP
	}
	if config.BitRate <= 0 {
		config.BitRate = 128000
	}

	// Find encoder
	codec := avcodec.FindEncoder(config.Codec)
	if codec == nil {
		return nil, errors.New("ffgo: audio encoder not found")
	}

	// Create stream
	stream := avformat.NewStream(m.formatCtx, codec)
	if stream == nil {
		return nil, errors.New("ffgo: failed to create audio stream")
	}

	// Create codec context
	codecCtx := avcodec.AllocContext3(codec)
	if codecCtx == nil {
		return nil, errors.New("ffgo: failed to allocate audio codec context")
	}

	// Configure codec
	avcodec.SetCtxSampleRate(codecCtx, int32(config.SampleRate))
	avcodec.SetCtxSampleFmt(codecCtx, int32(config.SampleFormat))
	avcodec.SetCtxBitRate(codecCtx, config.BitRate)
	avcodec.SetCtxTimeBase(codecCtx, 1, int32(config.SampleRate))

	// Set channel layout based on channel count
	avcodec.SetCtxChannelLayout(codecCtx, int32(config.Channels))

	// Set global header flag (commonly needed for containers like MP4)
	flags := avcodec.GetCtxFlags(codecCtx)
	avcodec.SetCtxFlags(codecCtx, flags|avcodec.CodecFlagGlobalHeader)

	// Open codec
	if err := avcodec.Open2(codecCtx, codec, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		return nil, err
	}

	// Copy parameters to stream
	codecPar := avformat.GetStreamCodecPar(stream)
	if err := avcodec.ParametersFromContext(codecPar, codecCtx); err != nil {
		avcodec.FreeContext(&codecCtx)
		return nil, err
	}

	ms := &MuxerStream{
		muxer:     m,
		stream:    stream,
		codecCtx:  codecCtx,
		index:     len(m.streams),
		timeBase:  NewRational(1, int32(config.SampleRate)),
		mediaType: MediaTypeAudio,
		encoder: &streamEncoder{
			codecCtx: codecCtx,
			packet:   avcodec.PacketAlloc(),
		},
	}

	m.streams = append(m.streams, ms)
	return ms, nil
}

// CopyStreamConfig configures a stream for copy mode (no re-encoding).
type CopyStreamConfig struct {
	CodecParameters avcodec.Parameters // Source stream codec parameters
	TimeBase        Rational           // Source stream time base
}

// AddCopyStream adds a stream in copy mode (no re-encoding).
// The codec parameters are copied from the source stream.
func (m *Muxer) AddCopyStream(config *CopyStreamConfig) (*MuxerStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("ffgo: muxer is closed")
	}
	if m.headerWritten {
		return nil, errors.New("ffgo: cannot add streams after header is written")
	}
	if config == nil || config.CodecParameters == nil {
		return nil, errors.New("ffgo: codec parameters are required for copy stream")
	}

	// Create stream
	stream := avformat.NewStream(m.formatCtx, nil)
	if stream == nil {
		return nil, errors.New("ffgo: failed to create copy stream")
	}

	// Copy codec parameters
	codecPar := avformat.GetStreamCodecPar(stream)
	if err := avcodec.ParametersCopy(codecPar, config.CodecParameters); err != nil {
		return nil, err
	}

	// Set time base
	avformat.SetStreamTimeBase(stream, config.TimeBase.Num, config.TimeBase.Den)

	ms := &MuxerStream{
		muxer:     m,
		stream:    stream,
		index:     len(m.streams),
		timeBase:  config.TimeBase,
		mediaType: avformat.GetCodecParType(codecPar),
		copyMode:  true,
	}

	m.streams = append(m.streams, ms)
	return ms, nil
}

// WriteHeader writes the container header.
// Must be called after all streams are added and before writing any frames/packets.
func (m *Muxer) WriteHeader() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("ffgo: muxer is closed")
	}
	if m.headerWritten {
		return errors.New("ffgo: header already written")
	}
	if len(m.streams) == 0 {
		return errors.New("ffgo: no streams added")
	}

	// Open output file
	if err := avformat.IOOpen(&m.ioCtx, m.path, avformat.IOFlagWrite); err != nil {
		return err
	}
	avformat.SetIOContext(m.formatCtx, m.ioCtx)

	// Write header
	if err := avformat.WriteHeader(m.formatCtx, nil); err != nil {
		return err
	}

	m.headerWritten = true
	return nil
}

// WriteFrame encodes and writes a frame to a stream.
// Only valid for streams created with AddVideoStream or AddAudioStream.
func (m *Muxer) WriteFrame(ms *MuxerStream, frame Frame) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("ffgo: muxer is closed")
	}
	if !m.headerWritten {
		return errors.New("ffgo: header not written")
	}
	if ms == nil || ms.muxer != m {
		return errors.New("ffgo: invalid stream")
	}
	if ms.copyMode {
		return errors.New("ffgo: cannot write frames to copy-mode stream, use WritePacket")
	}
	if ms.encoder == nil {
		return errors.New("ffgo: stream has no encoder")
	}

	// Send frame to encoder
	if err := avcodec.SendFrame(ms.codecCtx, frame); err != nil {
		return err
	}

	// Receive and write packets
	for {
		avcodec.PacketUnref(ms.encoder.packet)
		err := avcodec.ReceivePacket(ms.codecCtx, ms.encoder.packet)
		if err != nil {
			if avutil.IsAgain(err) || avutil.IsEOF(err) {
				break
			}
			return err
		}

		// Set stream index and rescale timestamps
		avcodec.SetPacketStreamIndex(ms.encoder.packet, int32(ms.index))
		streamTbNum, streamTbDen := avformat.GetStreamTimeBase(ms.stream)
		streamTb := NewRational(streamTbNum, streamTbDen)
		avcodec.RescalePacketTS(ms.encoder.packet, ms.timeBase, streamTb)

		// Write packet
		if err := avformat.InterleavedWriteFrame(m.formatCtx, ms.encoder.packet); err != nil {
			return err
		}
	}

	return nil
}

// WritePacket writes a packet directly to a stream.
// For copy-mode streams, timestamps should already be in the source time base.
func (m *Muxer) WritePacket(ms *MuxerStream, packet Packet) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("ffgo: muxer is closed")
	}
	if !m.headerWritten {
		return errors.New("ffgo: header not written")
	}
	if ms == nil || ms.muxer != m {
		return errors.New("ffgo: invalid stream")
	}

	// Set stream index
	avcodec.SetPacketStreamIndex(packet, int32(ms.index))

	// Rescale timestamps for copy mode
	if ms.copyMode {
		streamTbNum, streamTbDen := avformat.GetStreamTimeBase(ms.stream)
		streamTb := NewRational(streamTbNum, streamTbDen)
		avcodec.RescalePacketTS(packet, ms.timeBase, streamTb)
	}

	// Write packet
	return avformat.InterleavedWriteFrame(m.formatCtx, packet)
}

// WriteTrailer finalizes the container.
// Must be called after all frames/packets are written.
func (m *Muxer) WriteTrailer() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("ffgo: muxer is closed")
	}
	if !m.headerWritten {
		return errors.New("ffgo: header not written")
	}

	// Flush encoders
	for _, ms := range m.streams {
		if ms.encoder != nil && ms.codecCtx != nil {
			m.flushEncoder(ms)
		}
	}

	return avformat.WriteTrailer(m.formatCtx)
}

// flushEncoder flushes remaining packets from an encoder.
func (m *Muxer) flushEncoder(ms *MuxerStream) {
	// Send flush signal
	avcodec.SendFrame(ms.codecCtx, nil)

	// Receive and write remaining packets
	for {
		avcodec.PacketUnref(ms.encoder.packet)
		err := avcodec.ReceivePacket(ms.codecCtx, ms.encoder.packet)
		if err != nil {
			break
		}

		avcodec.SetPacketStreamIndex(ms.encoder.packet, int32(ms.index))
		streamTbNum, streamTbDen := avformat.GetStreamTimeBase(ms.stream)
		streamTb := NewRational(streamTbNum, streamTbDen)
		avcodec.RescalePacketTS(ms.encoder.packet, ms.timeBase, streamTb)
		avformat.InterleavedWriteFrame(m.formatCtx, ms.encoder.packet)
	}
}

// Close releases all resources.
func (m *Muxer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	// Free encoder resources
	for _, ms := range m.streams {
		if ms.encoder != nil {
			if ms.encoder.packet != nil {
				avcodec.PacketFree(&ms.encoder.packet)
			}
			if ms.encoder.frame != nil {
				avutil.FrameFree(&ms.encoder.frame)
			}
		}
		if ms.codecCtx != nil && !ms.copyMode {
			avcodec.FreeContext(&ms.codecCtx)
		}
	}

	// Close I/O context
	if m.ioCtx != nil {
		avformat.IOCloseP(&m.ioCtx)
	}

	// Free format context
	if m.formatCtx != nil {
		avformat.FreeContext(m.formatCtx)
		m.formatCtx = nil
	}

	return nil
}

// Streams returns all streams in the muxer.
func (m *Muxer) Streams() []*MuxerStream {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams
}

// Index returns the stream index.
func (ms *MuxerStream) Index() int {
	return ms.index
}

// MediaType returns the stream's media type.
func (ms *MuxerStream) MediaType() MediaType {
	return ms.mediaType
}

// TimeBase returns the stream's time base.
func (ms *MuxerStream) TimeBase() Rational {
	return ms.timeBase
}

// IsCopyMode returns true if the stream is in copy mode (no encoding).
func (ms *MuxerStream) IsCopyMode() bool {
	return ms.copyMode
}
