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

// Remuxer copies streams from input to output without re-encoding.
// This is useful for changing container formats or extracting streams.
type Remuxer struct {
	mu sync.Mutex

	// Output context
	outputCtx avformat.FormatContext
	outputIO  avformat.IOContext

	// Stream mapping: inputStreamIdx -> outputStreamIdx
	streamMap map[int]int

	// Time base mapping for timestamp rescaling
	inputTimeBases  map[int]avutil.Rational
	outputTimeBases map[int]avutil.Rational

	// Reusable packet
	packet avcodec.Packet

	headerWritten bool
	closed        bool
}

// RemuxerConfig configures a remuxer.
type RemuxerConfig struct {
	// InputStreams specifies which input stream indices to copy.
	// If empty, all streams are copied.
	InputStreams []int
}

// NewRemuxer creates a new remuxer that copies packets from decoder to output file.
// The decoder is used to get input stream information.
func NewRemuxer(outputPath string, decoder *Decoder, cfg *RemuxerConfig) (*Remuxer, error) {
	if decoder == nil {
		return nil, errors.New("ffgo: decoder is required for remuxing")
	}

	if err := bindings.Load(); err != nil {
		return nil, err
	}

	r := &Remuxer{
		streamMap:       make(map[int]int),
		inputTimeBases:  make(map[int]avutil.Rational),
		outputTimeBases: make(map[int]avutil.Rational),
	}

	// Determine output format from filename
	formatName := guessFormatFromPath(outputPath)
	if formatName == "" {
		return nil, errors.New("ffgo: cannot determine output format from filename")
	}

	// Create output format context
	if err := avformat.AllocOutputContext2(&r.outputCtx, nil, formatName, outputPath); err != nil {
		return nil, err
	}

	// Determine which streams to copy
	var streamsToCopy []int
	if cfg != nil && len(cfg.InputStreams) > 0 {
		streamsToCopy = cfg.InputStreams
	} else {
		// Copy all streams
		numStreams := avformat.GetNbStreams(decoder.formatCtx)
		streamsToCopy = make([]int, numStreams)
		for i := 0; i < numStreams; i++ {
			streamsToCopy[i] = i
		}
	}

	// Create output streams
	outputStreamIdx := 0
	for _, inputIdx := range streamsToCopy {
		inputStream := avformat.GetStream(decoder.formatCtx, inputIdx)
		if inputStream == nil {
			r.cleanup()
			return nil, errors.New("ffgo: invalid input stream index")
		}

		// Create output stream
		outputStream := avformat.NewStream(r.outputCtx, nil)
		if outputStream == nil {
			r.cleanup()
			return nil, errors.New("ffgo: failed to create output stream")
		}

		// Copy codec parameters from input to output
		inputCodecPar := avformat.GetStreamCodecPar(inputStream)
		outputCodecPar := avformat.GetStreamCodecPar(outputStream)
		if err := avcodec.ParametersCopy(outputCodecPar, inputCodecPar); err != nil {
			r.cleanup()
			return nil, err
		}

		// Clear codec tag for compatibility with different containers
		avcodec.SetCodecParTag(outputCodecPar, 0)

		// Store stream mapping and time bases
		r.streamMap[inputIdx] = outputStreamIdx

		inTbNum, inTbDen := avformat.GetStreamTimeBase(inputStream)
		r.inputTimeBases[inputIdx] = avutil.NewRational(inTbNum, inTbDen)

		outTbNum, outTbDen := avformat.GetStreamTimeBase(outputStream)
		r.outputTimeBases[inputIdx] = avutil.NewRational(outTbNum, outTbDen)

		outputStreamIdx++
	}

	// Open output file if needed
	if !avformat.HasNoFile(r.outputCtx) {
		if err := avformat.IOOpen(&r.outputIO, outputPath, avformat.IOFlagWrite); err != nil {
			r.cleanup()
			return nil, err
		}
		avformat.SetIOContext(r.outputCtx, r.outputIO)
	}

	// Allocate packet
	r.packet = avcodec.PacketAlloc()
	if r.packet == nil {
		r.cleanup()
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	return r, nil
}

// WriteHeader writes the output file header.
// Must be called before WritePacket.
func (r *Remuxer) WriteHeader() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.New("ffgo: remuxer is closed")
	}
	if r.headerWritten {
		return nil
	}

	if err := avformat.WriteHeader(r.outputCtx, nil); err != nil {
		return err
	}
	r.headerWritten = true

	// Update output time bases after header is written
	// (some formats may change time bases during header write)
	for inputIdx := range r.streamMap {
		outputIdx := r.streamMap[inputIdx]
		outputStream := avformat.GetStream(r.outputCtx, outputIdx)
		if outputStream != nil {
			tbNum, tbDen := avformat.GetStreamTimeBase(outputStream)
			r.outputTimeBases[inputIdx] = avutil.NewRational(tbNum, tbDen)
		}
	}

	return nil
}

// WritePacket copies a packet to the output.
// The packet's stream index is remapped to the output stream.
// Timestamps are rescaled from input to output time base.
func (r *Remuxer) WritePacket(pkt avcodec.Packet, inputStreamIdx int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.New("ffgo: remuxer is closed")
	}

	// Check if this stream is being copied
	outputIdx, ok := r.streamMap[inputStreamIdx]
	if !ok {
		// Stream not being copied, skip
		return nil
	}

	// Auto-write header if needed
	if !r.headerWritten {
		if err := avformat.WriteHeader(r.outputCtx, nil); err != nil {
			return err
		}
		r.headerWritten = true

		// Update output time bases
		for inIdx := range r.streamMap {
			outIdx := r.streamMap[inIdx]
			outputStream := avformat.GetStream(r.outputCtx, outIdx)
			if outputStream != nil {
				tbNum, tbDen := avformat.GetStreamTimeBase(outputStream)
				r.outputTimeBases[inIdx] = avutil.NewRational(tbNum, tbDen)
			}
		}
	}

	// Reference the packet (don't copy data, just increment refcount)
	_ = avcodec.PacketRef(r.packet, pkt)

	// Set output stream index
	avcodec.SetPacketStreamIndex(r.packet, int32(outputIdx))

	// Rescale timestamps from input to output time base
	inputTB := r.inputTimeBases[inputStreamIdx]
	outputTB := r.outputTimeBases[inputStreamIdx]
	avcodec.RescalePacketTS(r.packet, inputTB, outputTB)

	// Write the packet
	err := avformat.InterleavedWriteFrame(r.outputCtx, r.packet)

	// Unref the packet
	avcodec.PacketUnref(r.packet)

	return err
}

// Remux copies all packets from a decoder to the output.
// This is a convenience method that reads all packets and writes them.
func (r *Remuxer) Remux(decoder *Decoder) error {
	if err := r.WriteHeader(); err != nil {
		return err
	}

	for {
		pkt, err := decoder.ReadPacket()
		if err != nil {
			return err
		}
		if pkt == nil {
			break
		}

		streamIdx := pkt.StreamIndex()
		if err := r.WritePacket(pkt.ptr, streamIdx); err != nil {
			return err
		}
	}

	return nil
}

// Close finalizes and closes the remuxer.
func (r *Remuxer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	var firstErr error

	// Write trailer
	if r.outputCtx != nil && r.headerWritten {
		if err := avformat.WriteTrailer(r.outputCtx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	r.cleanup()
	return firstErr
}

func (r *Remuxer) cleanup() {
	if r.packet != nil {
		avcodec.PacketFree(&r.packet)
	}
	if r.outputIO != nil && r.outputCtx != nil {
		_ = avformat.IOCloseP(&r.outputIO)
	}
	if r.outputCtx != nil {
		avformat.FreeContext(r.outputCtx)
		r.outputCtx = nil
	}
}

// StreamMapping returns the mapping from input stream indices to output stream indices.
func (r *Remuxer) StreamMapping() map[int]int {
	return r.streamMap
}

// NumOutputStreams returns the number of output streams.
func (r *Remuxer) NumOutputStreams() int {
	return len(r.streamMap)
}
