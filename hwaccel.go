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

// HWDeviceType represents a hardware accelerator type.
type HWDeviceType = avutil.HWDeviceType

// Hardware device type constants (re-exported from avutil)
const (
	HWDeviceTypeNone         = avutil.HWDeviceTypeNone
	HWDeviceTypeVDPAU        = avutil.HWDeviceTypeVDPAU
	HWDeviceTypeCUDA         = avutil.HWDeviceTypeCUDA
	HWDeviceTypeVAAPI        = avutil.HWDeviceTypeVAAPI
	HWDeviceTypeDXVA2        = avutil.HWDeviceTypeDXVA2
	HWDeviceTypeQSV          = avutil.HWDeviceTypeQSV
	HWDeviceTypeVideoToolbox = avutil.HWDeviceTypeVideoToolbox
	HWDeviceTypeD3D11VA      = avutil.HWDeviceTypeD3D11VA
	HWDeviceTypeDRM          = avutil.HWDeviceTypeDRM
	HWDeviceTypeOpenCL       = avutil.HWDeviceTypeOpenCL
	HWDeviceTypeMediaCodec   = avutil.HWDeviceTypeMediaCodec
	HWDeviceTypeVulkan       = avutil.HWDeviceTypeVulkan
)

// HWDevice represents a hardware acceleration device.
// It wraps an FFmpeg AVBufferRef containing a hardware device context.
type HWDevice struct {
	mu         sync.Mutex
	deviceCtx  avutil.HWDeviceContext
	deviceType HWDeviceType
	closed     bool
}

// NewHWDevice creates a hardware device context for the given type.
// deviceType specifies the hardware accelerator (e.g., HWDeviceTypeVAAPI).
// device is an optional device path (e.g., "/dev/dri/renderD128" for VAAPI).
// Pass empty string to use the default device.
func NewHWDevice(deviceType HWDeviceType, device string) (*HWDevice, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	ctx, err := avutil.HWDeviceCtxCreate(deviceType, device)
	if err != nil {
		return nil, err
	}

	return &HWDevice{
		deviceCtx:  ctx,
		deviceType: deviceType,
	}, nil
}

// NewHWDeviceByName creates a hardware device context by name.
// name is the hardware accelerator name (e.g., "vaapi", "cuda", "videotoolbox").
// device is an optional device path.
func NewHWDeviceByName(name, device string) (*HWDevice, error) {
	deviceType := avutil.HWDeviceFindTypeByName(name)
	if deviceType == HWDeviceTypeNone {
		return nil, errors.New("ffgo: unknown hardware device type: " + name)
	}
	return NewHWDevice(deviceType, device)
}

// Type returns the hardware device type.
func (d *HWDevice) Type() HWDeviceType {
	return d.deviceType
}

// TypeName returns the name of the hardware device type.
func (d *HWDevice) TypeName() string {
	return avutil.HWDeviceGetTypeName(d.deviceType)
}

// Context returns the underlying hardware device context.
// This can be passed to decoders for hardware acceleration.
func (d *HWDevice) Context() avutil.HWDeviceContext {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.deviceCtx
}

// Close releases the hardware device resources.
func (d *HWDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	if d.deviceCtx != nil {
		avutil.FreeBufferRef(&d.deviceCtx)
	}
	return nil
}

// HWDecoderConfig configures a hardware-accelerated decoder.
type HWDecoderConfig struct {
	// HWDevice is the hardware device to use for acceleration.
	HWDevice *HWDevice

	// OutputSoftwareFrames controls whether to automatically transfer
	// decoded frames from GPU to CPU memory. If true (default), DecodeVideo
	// returns software frames that can be processed normally.
	// If false, frames remain in GPU memory and must be transferred manually.
	OutputSoftwareFrames bool
}

// HWDecoder is a hardware-accelerated video decoder.
// It provides a similar API to Decoder but uses GPU hardware for decoding.
type HWDecoder struct {
	mu sync.Mutex

	formatCtx     avformat.FormatContext
	videoCodecCtx avcodec.Context
	packet        avcodec.Packet
	frame         avutil.Frame
	swFrame       avutil.Frame // Software frame for GPU->CPU transfer

	videoStreamIdx int
	videoInfo      *StreamInfo

	hwDevice            *HWDevice
	outputSoftwareFrame bool
	closed              bool
}

// NewHWDecoder creates a hardware-accelerated decoder for the given file.
func NewHWDecoder(inputPath string, cfg *HWDecoderConfig) (*HWDecoder, error) {
	if cfg == nil || cfg.HWDevice == nil {
		return nil, errors.New("ffgo: HWDevice is required for hardware decoding")
	}

	if err := bindings.Load(); err != nil {
		return nil, err
	}

	// Open format context
	var formatCtx avformat.FormatContext
	if err := avformat.OpenInput(&formatCtx, inputPath, nil, nil); err != nil {
		return nil, err
	}

	if err := avformat.FindStreamInfo(formatCtx, nil); err != nil {
		avformat.CloseInput(&formatCtx)
		return nil, err
	}

	// Find best video stream
	videoStreamIdx := avformat.FindBestStream(formatCtx, avutil.MediaTypeVideo, -1, -1, nil, 0)
	if videoStreamIdx < 0 {
		avformat.CloseInput(&formatCtx)
		return nil, errors.New("ffgo: no video stream found")
	}

	// Get stream and codec info
	stream := avformat.GetStream(formatCtx, int(videoStreamIdx))
	codecPar := avformat.GetStreamCodecPar(stream)
	codecID := avformat.GetCodecParCodecID(codecPar)

	// Find decoder
	decoder := avcodec.FindDecoder(codecID)
	if decoder == nil {
		avformat.CloseInput(&formatCtx)
		return nil, errors.New("ffgo: decoder not found for video stream")
	}

	// Allocate codec context
	codecCtx := avcodec.AllocContext3(decoder)
	if codecCtx == nil {
		avformat.CloseInput(&formatCtx)
		return nil, errors.New("ffgo: failed to allocate codec context")
	}

	// Copy codec parameters
	if err := avcodec.ParametersToContext(codecCtx, codecPar); err != nil {
		avcodec.FreeContext(&codecCtx)
		avformat.CloseInput(&formatCtx)
		return nil, err
	}

	// Set hardware device context BEFORE opening the codec
	avcodec.SetCtxHWDeviceCtx(codecCtx, cfg.HWDevice.Context())

	// Open codec
	if err := avcodec.Open2(codecCtx, decoder, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		avformat.CloseInput(&formatCtx)
		return nil, err
	}

	// Allocate packet and frame
	packet := avcodec.PacketAlloc()
	if packet == nil {
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		avformat.CloseInput(&formatCtx)
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	frame := avutil.FrameAlloc()
	if frame == nil {
		avcodec.PacketFree(&packet)
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		avformat.CloseInput(&formatCtx)
		return nil, errors.New("ffgo: failed to allocate frame")
	}

	// Allocate software frame for transfers if needed
	var swFrame avutil.Frame
	if cfg.OutputSoftwareFrames {
		swFrame = avutil.FrameAlloc()
		if swFrame == nil {
			avutil.FrameFree(&frame)
			avcodec.PacketFree(&packet)
			avcodec.Close(codecCtx)
			avcodec.FreeContext(&codecCtx)
			avformat.CloseInput(&formatCtx)
			return nil, errors.New("ffgo: failed to allocate software frame")
		}
	}

	// Get video stream info
	width := avformat.GetCodecParWidth(codecPar)
	height := avformat.GetCodecParHeight(codecPar)
	frameRateNum, frameRateDen := avformat.GetStreamAvgFrameRate(stream)

	videoInfo := &StreamInfo{
		Index:     int(videoStreamIdx),
		Type:      MediaTypeVideo,
		CodecID:   CodecID(codecID),
		Width:     int(width),
		Height:    int(height),
		FrameRate: Rational{Num: frameRateNum, Den: frameRateDen},
	}

	return &HWDecoder{
		formatCtx:           formatCtx,
		videoCodecCtx:       codecCtx,
		packet:              packet,
		frame:               frame,
		swFrame:             swFrame,
		videoStreamIdx:      int(videoStreamIdx),
		videoInfo:           videoInfo,
		hwDevice:            cfg.HWDevice,
		outputSoftwareFrame: cfg.OutputSoftwareFrames,
	}, nil
}

// VideoStream returns information about the video stream.
func (d *HWDecoder) VideoStream() *StreamInfo {
	return d.videoInfo
}

// HasVideo returns true (HWDecoder always has video).
func (d *HWDecoder) HasVideo() bool {
	return true
}

// DecodeVideo reads and decodes the next video frame.
// If OutputSoftwareFrames is enabled, frames are automatically
// transferred from GPU to CPU memory.
func (d *HWDecoder) DecodeVideo() (Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, errors.New("ffgo: decoder is closed")
	}

	for {
		// Try to receive a frame first
		ret := avcodec.ReceiveFrame(d.videoCodecCtx, d.frame)
		if ret == nil {
			// Successfully received a frame
			// Check if we need to transfer from GPU to CPU
			if d.outputSoftwareFrame && d.swFrame != nil {
				avutil.FrameUnref(d.swFrame)
				err := avutil.HWFrameTransferData(d.swFrame, d.frame, 0)
				if err == nil {
					// Transfer succeeded, copy properties
					avutil.SetFramePTS(d.swFrame, avutil.GetFramePTS(d.frame))
					return d.swFrame, nil
				}
				// Transfer failed (frame might already be in software format)
			}
			return d.frame, nil
		}

		// Need more data, read a packet
		if err := avformat.ReadFrame(d.formatCtx, d.packet); err != nil {
			return nil, err
		}

		// Check if this packet is for our video stream
		streamIdx := avcodec.GetPacketStreamIndex(d.packet)
		if int(streamIdx) != d.videoStreamIdx {
			avcodec.PacketUnref(d.packet)
			continue
		}

		// Send packet to decoder
		if err := avcodec.SendPacket(d.videoCodecCtx, d.packet); err != nil {
			avcodec.PacketUnref(d.packet)
			// EAGAIN means try receive again
			if !avutil.IsAgain(err) {
				return nil, err
			}
		}
		avcodec.PacketUnref(d.packet)
	}
}

// ReadHWFrame reads and decodes the next video frame, keeping it in GPU memory.
// This is useful when you want to process frames on the GPU or control
// when transfers to CPU memory occur.
// Use TransferToSystem to transfer the frame to CPU memory when needed.
func (d *HWDecoder) ReadHWFrame() (Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, errors.New("ffgo: decoder is closed")
	}

	for {
		// Try to receive a frame first
		ret := avcodec.ReceiveFrame(d.videoCodecCtx, d.frame)
		if ret == nil {
			// Successfully received a frame (stays in GPU memory)
			return d.frame, nil
		}

		// Need more data, read a packet
		if err := avformat.ReadFrame(d.formatCtx, d.packet); err != nil {
			return nil, err
		}

		// Check if this packet is for our video stream
		streamIdx := avcodec.GetPacketStreamIndex(d.packet)
		if int(streamIdx) != d.videoStreamIdx {
			avcodec.PacketUnref(d.packet)
			continue
		}

		// Send packet to decoder
		if err := avcodec.SendPacket(d.videoCodecCtx, d.packet); err != nil {
			avcodec.PacketUnref(d.packet)
			// EAGAIN means try receive again
			if !avutil.IsAgain(err) {
				return nil, err
			}
		}
		avcodec.PacketUnref(d.packet)
	}
}

// TransferToSystem transfers a hardware frame to a software frame in CPU memory.
// Use this if you called ReadHWFrame and need to process the frame on the CPU.
// The returned frame must be freed by the caller when no longer needed.
func (d *HWDecoder) TransferToSystem(hwFrame Frame) (Frame, error) {
	swFrame := avutil.FrameAlloc()
	if swFrame == nil {
		return nil, errors.New("ffgo: failed to allocate frame")
	}

	if err := avutil.HWFrameTransferData(swFrame, hwFrame, 0); err != nil {
		avutil.FrameFree(&swFrame)
		return nil, err
	}

	// Copy PTS
	avutil.SetFramePTS(swFrame, avutil.GetFramePTS(hwFrame))
	return swFrame, nil
}

// TransferToSoftware transfers a hardware frame to a software frame.
// Deprecated: Use TransferToSystem instead.
func (d *HWDecoder) TransferToSoftware(hwFrame Frame) (Frame, error) {
	return d.TransferToSystem(hwFrame)
}

// Close releases all decoder resources.
func (d *HWDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	if d.swFrame != nil {
		avutil.FrameFree(&d.swFrame)
	}
	if d.frame != nil {
		avutil.FrameFree(&d.frame)
	}
	if d.packet != nil {
		avcodec.PacketFree(&d.packet)
	}
	if d.videoCodecCtx != nil {
		avcodec.Close(d.videoCodecCtx)
		avcodec.FreeContext(&d.videoCodecCtx)
	}
	if d.formatCtx != nil {
		avformat.CloseInput(&d.formatCtx)
	}

	return nil
}

// AvailableHWDeviceTypes returns a list of available hardware device types on this system.
func AvailableHWDeviceTypes() []HWDeviceType {
	types := []HWDeviceType{}

	// Test each known device type
	testTypes := []HWDeviceType{
		HWDeviceTypeVAAPI,
		HWDeviceTypeCUDA,
		HWDeviceTypeVideoToolbox,
		HWDeviceTypeDXVA2,
		HWDeviceTypeD3D11VA,
		HWDeviceTypeQSV,
		HWDeviceTypeVDPAU,
		HWDeviceTypeVulkan,
		HWDeviceTypeDRM,
	}

	for _, t := range testTypes {
		// Try to create a device context to test availability
		ctx, err := avutil.HWDeviceCtxCreate(t, "")
		if err == nil && ctx != nil {
			types = append(types, t)
			avutil.FreeBufferRef(&ctx)
		}
	}

	return types
}

// GetHWDeviceTypeName returns the name for a hardware device type.
func GetHWDeviceTypeName(t HWDeviceType) string {
	return avutil.HWDeviceGetTypeName(t)
}
