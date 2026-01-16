//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// TwoPassTranscode performs a simple 2-pass video transcode using the provided encoder options.
//
// Notes:
// - This helper currently transcodes video only (audio is ignored).
// - Input must be seekable (regular files are fine).
func TwoPassTranscode(input, output string, opts *EncoderOptions) error {
	if input == "" || output == "" {
		return errors.New("ffgo: input and output are required")
	}
	if opts == nil || opts.Video == nil {
		return errors.New("ffgo: EncoderOptions.Video is required")
	}

	dec, err := NewDecoder(input)
	if err != nil {
		return err
	}
	defer dec.Close()

	if !dec.HasVideo() {
		return errors.New("ffgo: input has no video stream")
	}
	if err := dec.OpenVideoDecoder(); err != nil {
		return err
	}
	videoInfo := dec.VideoStream()
	if videoInfo == nil {
		return errors.New("ffgo: video stream info not available")
	}

	// Fill common defaults from input if unset.
	if opts.Video.Width <= 0 {
		opts.Video.Width = videoInfo.Width
	}
	if opts.Video.Height <= 0 {
		opts.Video.Height = videoInfo.Height
	}
	if opts.Video.PixelFormat == PixelFormatNone {
		// A safe default for most H.264 encoders.
		opts.Video.PixelFormat = PixelFormatYUV420P
	}

	// Determine passlog base
	passBase := opts.PassLogFile
	cleanupPassFiles := false
	if passBase == "" {
		f, err := os.CreateTemp("", "ffgo-passlog-*")
		if err != nil {
			return err
		}
		passBase = f.Name()
		_ = f.Close()
		_ = os.Remove(passBase) // base path only
		cleanupPassFiles = true
	}

	// Build pass1 output path (discard later)
	pass1Out := opts.PassOutput
	cleanupPass1Out := false
	if pass1Out == "" {
		ext := filepath.Ext(output)
		if ext == "" {
			ext = ".mp4"
		}
		f, err := os.CreateTemp("", "ffgo-pass1-*"+ext)
		if err != nil {
			return err
		}
		pass1Out = f.Name()
		_ = f.Close()
		cleanupPass1Out = true
	}

	if err := runPass(dec, videoInfo, pass1Out, opts, 1, passBase); err != nil {
		if cleanupPass1Out {
			_ = os.Remove(pass1Out)
		}
		if cleanupPassFiles {
			cleanupPassLogFiles(passBase)
		}
		return err
	}

	// Ensure we don't keep the pass1 output.
	if cleanupPass1Out {
		_ = os.Remove(pass1Out)
	}

	// Seek back to start for pass 2.
	if err := dec.SeekTimestamp(0); err != nil {
		if cleanupPassFiles {
			cleanupPassLogFiles(passBase)
		}
		return err
	}

	if err := runPass(dec, videoInfo, output, opts, 2, passBase); err != nil {
		if cleanupPassFiles {
			cleanupPassLogFiles(passBase)
		}
		return err
	}

	if cleanupPassFiles {
		cleanupPassLogFiles(passBase)
	}
	return nil
}

func runPass(dec *Decoder, videoInfo *StreamInfo, output string, baseOpts *EncoderOptions, pass int, passBase string) error {
	// Clone options for this pass
	passOpts := *baseOpts
	passOpts.Pass = pass
	passOpts.PassLogFile = passBase
	// PassOutput is only meaningful to the helper, not encoder creation.

	enc, err := NewEncoderWithOptions(output, &passOpts)
	if err != nil {
		return err
	}
	defer enc.Close()

	// Scaler if needed
	var scaler *Scaler
	if videoInfo.PixelFmt != passOpts.Video.PixelFormat && passOpts.Video.PixelFormat != PixelFormatNone {
		s, err := NewScalerWithConfig(ScalerConfig{
			SrcWidth:  videoInfo.Width,
			SrcHeight: videoInfo.Height,
			SrcFormat: videoInfo.PixelFmt,
			DstWidth:  passOpts.Video.Width,
			DstHeight: passOpts.Video.Height,
			DstFormat: passOpts.Video.PixelFormat,
			Flags:     ScaleBilinear,
		})
		if err != nil {
			return err
		}
		defer s.Close()
		scaler = s
	}

	for {
		frame, err := dec.DecodeVideo()
		if err != nil {
			if IsEOF(err) {
				break
			}
			return err
		}
		if frame.IsNil() {
			break
		}

		outFrame := frame
		if scaler != nil {
			sf, err := scaler.Scale(frame)
			if err != nil {
				return err
			}
			outFrame = sf
		}

		if err := enc.WriteVideoFrame(outFrame); err != nil {
			return err
		}
	}

	// Flush + trailer
	if err := enc.Close(); err != nil {
		return err
	}
	return nil
}

func cleanupPassLogFiles(base string) {
	if base == "" {
		return
	}
	dir := filepath.Dir(base)
	prefix := filepath.Base(base)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

