//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// BitstreamFilter represents an FFmpeg bitstream filter.
// Bitstream filters modify packet data without decoding.
type BitstreamFilter struct {
	mu     sync.Mutex
	ctx    bsfContext
	packet avcodec.Packet
	closed bool
}

// bsfContext is an opaque FFmpeg AVBSFContext pointer.
type bsfContext = unsafe.Pointer

// BSF function bindings
var (
	avBsfGetByName     func(name string) uintptr
	avBsfAllocContext  func(filter uintptr, ctx *unsafe.Pointer) int32
	avBsfInit          func(ctx uintptr) int32
	avBsfFree          func(ctx *unsafe.Pointer)
	avBsfSendPacket    func(ctx, pkt uintptr) int32
	avBsfReceivePacket func(ctx, pkt uintptr) int32

	bsfBindingsRegistered bool
)

func registerBSFBindings() {
	if bsfBindingsRegistered {
		return
	}

	if err := bindings.Load(); err != nil {
		return
	}

	lib := bindings.LibAVCodec()
	if lib == 0 {
		return
	}

	purego.RegisterLibFunc(&avBsfGetByName, lib, "av_bsf_get_by_name")
	purego.RegisterLibFunc(&avBsfAllocContext, lib, "av_bsf_alloc")
	purego.RegisterLibFunc(&avBsfInit, lib, "av_bsf_init")
	purego.RegisterLibFunc(&avBsfFree, lib, "av_bsf_free")
	purego.RegisterLibFunc(&avBsfSendPacket, lib, "av_bsf_send_packet")
	purego.RegisterLibFunc(&avBsfReceivePacket, lib, "av_bsf_receive_packet")

	bsfBindingsRegistered = true
}

// Common bitstream filter names
const (
	// BSFNameH264Mp4ToAnnexB converts H.264 from MP4 format to Annex B format.
	// Useful when remuxing MP4 to transport streams or raw H.264.
	BSFNameH264Mp4ToAnnexB = "h264_mp4toannexb"

	// BSFNameHEVCMp4ToAnnexB converts HEVC from MP4 format to Annex B format.
	BSFNameHEVCMp4ToAnnexB = "hevc_mp4toannexb"

	// BSFNameAACADTSToASC converts AAC from ADTS header format to AudioSpecificConfig.
	// Useful when remuxing to MP4 containers.
	BSFNameAACADTSToASC = "aac_adtstoasc"

	// BSFNameExtractExtradata extracts codec-specific extradata from packets.
	BSFNameExtractExtradata = "extract_extradata"

	// BSFNameDumpExtradata dumps extradata to packets (opposite of extract).
	BSFNameDumpExtradata = "dump_extra"

	// BSFNameRemoveExtradata removes extradata from packets.
	BSFNameRemoveExtradata = "remove_extra"

	// BSFNameNull is a passthrough filter (for testing).
	BSFNameNull = "null"
)

// BSFContext field offsets
const (
	offsetBsfParIn       = 24 // AVCodecParameters *par_in
	offsetBsfParOut      = 32 // AVCodecParameters *par_out
	offsetBsfTimeBaseIn  = 40 // AVRational time_base_in
	offsetBsfTimeBaseOut = 48 // AVRational time_base_out
)

// NewBitstreamFilter creates a new bitstream filter.
// filterName is the name of the filter (e.g., BSFNameH264Mp4ToAnnexB).
func NewBitstreamFilter(filterName string) (*BitstreamFilter, error) {
	registerBSFBindings()

	if avBsfGetByName == nil || avBsfAllocContext == nil {
		return nil, bindings.ErrNotLoaded
	}

	// Find the filter
	filter := unsafe.Pointer(avBsfGetByName(filterName))
	if filter == nil {
		return nil, errors.New("ffgo: bitstream filter not found: " + filterName)
	}

	// Allocate context
	var ctx bsfContext
	ret := avBsfAllocContext(uintptr(filter), &ctx)
	if ret < 0 {
		return nil, avutil.NewError(ret, "av_bsf_alloc")
	}

	// Allocate packet
	packet := avcodec.PacketAlloc()
	if packet == nil {
		avBsfFree(&ctx)
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	return &BitstreamFilter{
		ctx:    ctx,
		packet: packet,
	}, nil
}

// SetInputCodecParameters sets the input codec parameters.
// This must be called before Init() and should match the codec of packets being filtered.
func (f *BitstreamFilter) SetInputCodecParameters(par avcodec.Parameters) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return errors.New("ffgo: filter is closed")
	}

	// Get par_in pointer and copy parameters
	parIn := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(f.ctx) + offsetBsfParIn))
	if parIn == nil {
		return errors.New("ffgo: filter has no par_in")
	}

	return avcodec.ParametersCopy(parIn, par)
}

// SetInputTimeBase sets the input time base.
func (f *BitstreamFilter) SetInputTimeBase(num, den int32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return
	}

	*(*int32)(unsafe.Pointer(uintptr(f.ctx) + offsetBsfTimeBaseIn)) = num
	*(*int32)(unsafe.Pointer(uintptr(f.ctx) + offsetBsfTimeBaseIn + 4)) = den
}

// Init initializes the filter after configuration.
// Must be called after SetInputCodecParameters.
func (f *BitstreamFilter) Init() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return errors.New("ffgo: filter is closed")
	}

	if avBsfInit == nil {
		return bindings.ErrNotLoaded
	}

	ret := avBsfInit(uintptr(f.ctx))
	if ret < 0 {
		return avutil.NewError(ret, "av_bsf_init")
	}
	return nil
}

// Filter sends a packet through the filter and receives the filtered packet.
// The input packet's data is consumed. Returns the filtered packet or error.
// Returns nil, nil if more input is needed (EAGAIN).
func (f *BitstreamFilter) Filter(pkt avcodec.Packet) (avcodec.Packet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return nil, errors.New("ffgo: filter is closed")
	}

	// Send packet
	ret := avBsfSendPacket(uintptr(f.ctx), uintptr(pkt))
	if ret < 0 {
		if isEAGAIN(ret) {
			return nil, nil
		}
		return nil, avutil.NewError(ret, "av_bsf_send_packet")
	}

	// Receive filtered packet
	ret = avBsfReceivePacket(uintptr(f.ctx), uintptr(f.packet))
	if ret < 0 {
		if isEAGAIN(ret) || isEOF(ret) {
			return nil, nil
		}
		return nil, avutil.NewError(ret, "av_bsf_receive_packet")
	}

	return f.packet, nil
}

// Flush flushes any remaining packets from the filter.
// Call this after all input packets have been sent.
func (f *BitstreamFilter) Flush() (avcodec.Packet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return nil, errors.New("ffgo: filter is closed")
	}

	// Send NULL packet to flush
	ret := avBsfSendPacket(uintptr(f.ctx), 0)
	if ret < 0 && !isEAGAIN(ret) {
		return nil, avutil.NewError(ret, "av_bsf_send_packet")
	}

	// Receive flushed packet
	ret = avBsfReceivePacket(uintptr(f.ctx), uintptr(f.packet))
	if ret < 0 {
		if isEAGAIN(ret) || isEOF(ret) {
			return nil, nil
		}
		return nil, avutil.NewError(ret, "av_bsf_receive_packet")
	}

	return f.packet, nil
}

// GetOutputCodecParameters gets the output codec parameters after Init().
func (f *BitstreamFilter) GetOutputCodecParameters() avcodec.Parameters {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return nil
	}

	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(f.ctx) + offsetBsfParOut))
}

// GetOutputTimeBase gets the output time base after Init().
func (f *BitstreamFilter) GetOutputTimeBase() (num, den int32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.ctx == nil {
		return 0, 1
	}

	num = *(*int32)(unsafe.Pointer(uintptr(f.ctx) + offsetBsfTimeBaseOut))
	den = *(*int32)(unsafe.Pointer(uintptr(f.ctx) + offsetBsfTimeBaseOut + 4))
	return
}

// Close releases all filter resources.
func (f *BitstreamFilter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}
	f.closed = true

	if f.packet != nil {
		avcodec.PacketFree(&f.packet)
	}
	if f.ctx != nil && avBsfFree != nil {
		avBsfFree(&f.ctx)
	}

	return nil
}

// Helper functions for error checking
func isEAGAIN(ret int32) bool {
	return ret == -11 || ret == -35 // EAGAIN on Linux/macOS
}

func isEOF(ret int32) bool {
	// AVERROR_EOF is typically FFERRTAG('E','O','F',' ') = -541478725
	return ret == -541478725
}

// BitstreamFilterExists checks if a bitstream filter with the given name exists.
func BitstreamFilterExists(name string) bool {
	registerBSFBindings()

	if avBsfGetByName == nil {
		return false
	}

	return unsafe.Pointer(avBsfGetByName(name)) != nil
}

// ListBitstreamFilters returns a list of common bitstream filter names.
func ListBitstreamFilters() []string {
	return []string{
		BSFNameH264Mp4ToAnnexB,
		BSFNameHEVCMp4ToAnnexB,
		BSFNameAACADTSToASC,
		BSFNameExtractExtradata,
		BSFNameDumpExtradata,
		BSFNameRemoveExtradata,
		BSFNameNull,
	}
}
