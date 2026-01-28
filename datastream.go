//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"

	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
)

// HasDataStream reports whether the input contains any AVMEDIA_TYPE_DATA streams.
func (d *Decoder) HasDataStream() bool {
	return len(d.DataStreams()) > 0
}

// DataStreams returns information about all data streams (AVMEDIA_TYPE_DATA).
func (d *Decoder) DataStreams() []*StreamInfo {
	if d == nil || d.formatCtx == nil {
		return nil
	}
	numStreams := avformat.GetNumStreams(d.formatCtx)
	var out []*StreamInfo
	for i := 0; i < numStreams; i++ {
		stream := avformat.GetStream(d.formatCtx, i)
		codecPar := avformat.GetStreamCodecPar(stream)
		if codecPar == nil {
			continue
		}
		if avformat.GetCodecParType(codecPar) == avutil.MediaTypeData {
			out = append(out, d.getStreamInfo(i))
		}
	}
	return out
}

// ReadDataPacket reads packets until it finds one belonging to a data stream (AVMEDIA_TYPE_DATA).
//
// The returned packet is BORROWED (decoder-owned and internally reused). Do not free it.
//
// Note: This helper consumes packets from the underlying demuxer. If you are decoding video/audio
// from the same Decoder concurrently, you likely want a dedicated Decoder instance for data streams.
//
// Returns (nil, nil) on EOF.
func (d *Decoder) ReadDataPacket() (*Packet, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, errors.New("ffgo: decoder is closed")
	}
	if d.formatCtx == nil || d.packet == nil {
		return nil, errors.New("ffgo: decoder is not initialized")
	}

	// Build a small lookup of data stream indexes for this call.
	data := make(map[int]struct{})
	numStreams := avformat.GetNumStreams(d.formatCtx)
	for i := 0; i < numStreams; i++ {
		stream := avformat.GetStream(d.formatCtx, i)
		codecPar := avformat.GetStreamCodecPar(stream)
		if codecPar == nil {
			continue
		}
		if avformat.GetCodecParType(codecPar) == avutil.MediaTypeData {
			data[i] = struct{}{}
		}
	}
	if len(data) == 0 {
		return nil, nil
	}

	for {
		avcodec.PacketUnref(d.packet)
		if err := avformat.ReadFrame(d.formatCtx, d.packet); err != nil {
			if avutil.IsEOF(err) {
				return nil, nil
			}
			return nil, err
		}
		si := int(avcodec.GetPacketStreamIndex(d.packet))
		if _, ok := data[si]; ok {
			return &Packet{ptr: d.packet, owned: false}, nil
		}
	}
}
