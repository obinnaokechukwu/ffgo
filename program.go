//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"

	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
)

// ProgramInfo describes a program in a multi-program input (e.g. MPEG-TS).
type ProgramInfo struct {
	// ID is the program id.
	ID int

	// StreamIndexes lists the FFmpeg stream indexes that belong to this program.
	StreamIndexes []int

	// Metadata contains program-level metadata, if present.
	Metadata Metadata
}

// Programs returns the programs present in the input, if any.
func (d *Decoder) Programs() []ProgramInfo {
	if d == nil || d.formatCtx == nil {
		return nil
	}
	n := avformat.GetNumPrograms(d.formatCtx)
	if n <= 0 {
		return nil
	}
	out := make([]ProgramInfo, 0, n)
	for i := 0; i < n; i++ {
		p := avformat.GetProgram(d.formatCtx, i)
		if p == nil {
			continue
		}
		out = append(out, ProgramInfo{
			ID:            avformat.GetProgramID(p),
			StreamIndexes: avformat.GetProgramStreamIndexes(p),
			Metadata:      getMetadataFromDict(avformat.GetProgramMetadata(p)),
		})
	}
	return out
}

func (d *Decoder) selectProgramStreams(programID int, wantVideo, wantAudio bool) error {
	if d == nil || d.formatCtx == nil {
		return errors.New("ffgo: decoder is not initialized")
	}
	if programID <= 0 {
		return errors.New("ffgo: invalid program id")
	}

	n := avformat.GetNumPrograms(d.formatCtx)
	for i := 0; i < n; i++ {
		p := avformat.GetProgram(d.formatCtx, i)
		if p == nil {
			continue
		}
		if avformat.GetProgramID(p) != programID {
			continue
		}

		// Reset selection.
		d.videoStreamIdx, d.audioStreamIdx = -1, -1
		d.videoInfo, d.audioInfo = nil, nil

		for _, si := range avformat.GetProgramStreamIndexes(p) {
			stream := avformat.GetStream(d.formatCtx, si)
			if stream == nil {
				continue
			}
			codecPar := avformat.GetStreamCodecPar(stream)
			if codecPar == nil {
				continue
			}
			mt := avformat.GetCodecParType(codecPar)
			if wantVideo && d.videoStreamIdx < 0 && mt == avutil.MediaTypeVideo {
				d.videoStreamIdx = si
				d.videoInfo = d.getStreamInfo(si)
			}
			if wantAudio && d.audioStreamIdx < 0 && mt == avutil.MediaTypeAudio {
				d.audioStreamIdx = si
				d.audioInfo = d.getStreamInfo(si)
			}
			if (!wantVideo || d.videoStreamIdx >= 0) && (!wantAudio || d.audioStreamIdx >= 0) {
				break
			}
		}

		return nil
	}
	return errors.New("ffgo: program not found")
}
