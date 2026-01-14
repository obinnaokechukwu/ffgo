//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
)

// SaveFrame saves a frame to an image file.
// The format is determined by the file extension (png, jpg, jpeg, bmp).
// frame must be in a pixel format compatible with the image encoder (RGB24 recommended).
func SaveFrame(frame Frame, filename string) error {
	if frame == nil {
		return errors.New("ffgo: frame is nil")
	}

	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(filename))
	var encoderName string
	var targetPixFmtConst PixelFormat
	switch ext {
	case ".png":
		encoderName = "png"
		targetPixFmtConst = PixelFormatRGB24
	case ".jpg", ".jpeg":
		encoderName = "mjpeg"
		targetPixFmtConst = PixelFormatYUVJ420P
	case ".bmp":
		encoderName = "bmp"
		targetPixFmtConst = PixelFormatBGR24
	default:
		return errors.New("ffgo: unsupported image format: " + ext)
	}

	// Get frame dimensions
	width := avutil.GetFrameWidth(frame)
	height := avutil.GetFrameHeight(frame)
	pixFmt := avutil.GetFrameFormat(frame)

	if width == 0 || height == 0 {
		return errors.New("ffgo: frame has invalid dimensions")
	}

	// Find encoder by name
	encoder := avcodec.FindEncoderByName(encoderName)
	if encoder == nil {
		return errors.New("ffgo: image encoder not found: " + encoderName)
	}

	// Allocate codec context
	codecCtx := avcodec.AllocContext3(encoder)
	if codecCtx == nil {
		return errors.New("ffgo: failed to allocate encoder context")
	}

	// Configure encoder
	avcodec.SetCtxWidth(codecCtx, width)
	avcodec.SetCtxHeight(codecCtx, height)
	avcodec.SetCtxTimeBase(codecCtx, 1, 25)

	// Set target pixel format
	targetPixFmt := int32(targetPixFmtConst)
	avcodec.SetCtxPixFmt(codecCtx, targetPixFmt)

	// Open encoder
	if err := avcodec.Open2(codecCtx, encoder, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		return err
	}

	// Convert frame if needed
	var frameToEncode Frame = frame
	var scaler *Scaler
	if pixFmt != targetPixFmt {
		var err error
		scaler, err = NewScaler(int(width), int(height), PixelFormat(pixFmt),
			int(width), int(height), PixelFormat(targetPixFmt), ScaleBilinear)
		if err != nil {
			avcodec.Close(codecCtx)
			avcodec.FreeContext(&codecCtx)
			return err
		}

		// Note: Scale() returns a frame owned by the scaler - don't free it separately
		frameToEncode, err = scaler.Scale(frame)
		if err != nil {
			scaler.Close()
			avcodec.Close(codecCtx)
			avcodec.FreeContext(&codecCtx)
			return err
		}
	}

	// Allocate packet
	packet := avcodec.PacketAlloc()
	if packet == nil {
		if scaler != nil {
			scaler.Close()
		}
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		return errors.New("ffgo: failed to allocate packet")
	}

	// Send frame
	err := avcodec.SendFrame(codecCtx, frameToEncode)
	if err != nil {
		avcodec.PacketFree(&packet)
		if scaler != nil {
			scaler.Close()
		}
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		return err
	}

	// Receive packet
	err = avcodec.ReceivePacket(codecCtx, packet)
	if err != nil {
		avcodec.PacketFree(&packet)
		if scaler != nil {
			scaler.Close()
		}
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		return err
	}

	// Get packet data
	packetData := avcodec.GetPacketData(packet)
	packetSize := avcodec.GetPacketSize(packet)

	if packetData == nil || packetSize <= 0 {
		avcodec.PacketFree(&packet)
		if scaler != nil {
			scaler.Close()
		}
		avcodec.Close(codecCtx)
		avcodec.FreeContext(&codecCtx)
		return errors.New("ffgo: encoder produced no data")
	}

	// Copy data to Go slice
	data := make([]byte, packetSize)
	copy(data, unsafe.Slice((*byte)(packetData), packetSize))

	// Clean up FFmpeg resources first
	avcodec.PacketFree(&packet)
	if scaler != nil {
		scaler.Close() // This frees the internal dstFrame
	}
	avcodec.Close(codecCtx)
	avcodec.FreeContext(&codecCtx)

	// Write to file
	return os.WriteFile(filename, data, 0644)
}

// ExtractFrame extracts a single frame from a video file at the specified timestamp
// and saves it to an image file.
func ExtractFrame(inputPath string, ts time.Duration, outputPath string) error {
	decoder, err := NewDecoder(inputPath)
	if err != nil {
		return err
	}
	defer decoder.Close()

	// Seek to the requested timestamp
	if ts > 0 {
		if err := decoder.Seek(ts); err != nil {
			return err
		}
	}

	// Decode a frame at that position
	// Note: frame is owned by decoder, don't free it
	frame, err := decoder.DecodeVideo()
	if err != nil {
		return err
	}
	if frame == nil {
		return errors.New("ffgo: no video frame at the specified timestamp")
	}

	return SaveFrame(frame, outputPath)
}

// GenerateThumbnails extracts multiple frames at evenly spaced intervals and saves them.
// pattern should contain a format specifier like %02d for the frame number.
// interval is the time between thumbnails, maxCount limits the number of thumbnails.
func GenerateThumbnails(inputPath string, interval time.Duration, maxCount int, outputPattern string) ([]string, error) {
	decoder, err := NewDecoder(inputPath)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()

	duration := decoder.Duration()
	if duration <= 0 {
		return nil, errors.New("ffgo: cannot determine duration")
	}

	// Calculate number of thumbnails
	count := int(duration / interval)
	if count > maxCount && maxCount > 0 {
		count = maxCount
	}
	if count <= 0 {
		count = 1
	}

	var filenames []string
	for i := 0; i < count; i++ {
		ts := interval * time.Duration(i)
		if ts >= duration {
			break
		}

		// Seek to the timestamp
		if err := decoder.Seek(ts); err != nil {
			continue // Skip frames that fail
		}

		// Decode a frame at that position
		// Note: frame is owned by decoder, don't free it
		frame, err := decoder.DecodeVideo()
		if err != nil || frame == nil {
			continue // Skip frames that fail
		}

		filename := fmt.Sprintf(outputPattern, i)

		if err := SaveFrame(frame, filename); err != nil {
			continue
		}
		filenames = append(filenames, filename)
	}

	return filenames, nil
}

// Keyframe represents a keyframe position in the video
type Keyframe struct {
	PTS      int64         // Presentation timestamp
	Time     time.Duration // Time in the video
	Position int64         // Byte position in file (if available)
	Frame    int64         // Estimated frame number
}

// GetKeyframes returns a list of keyframes in the video.
// This scans the video for keyframes without decoding.
func (d *Decoder) GetKeyframes() ([]Keyframe, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, errors.New("ffgo: decoder is closed")
	}

	if d.videoStreamIdx < 0 {
		return nil, errors.New("ffgo: no video stream")
	}

	// Save current position - seek back to beginning
	avformat.SeekFrame(d.formatCtx, -1, 0, avformat.SeekFlagBackward)

	stream := avformat.GetStream(d.formatCtx, d.videoStreamIdx)
	if stream == nil {
		return nil, errors.New("ffgo: failed to get video stream")
	}

	tbNum, tbDen := avformat.GetStreamTimeBase(stream)
	fpsNum, fpsDen := avformat.GetStreamAvgFrameRate(stream)

	var keyframes []Keyframe

	// Scan packets for keyframes
	for {
		if err := avformat.ReadFrame(d.formatCtx, d.packet); err != nil {
			if avutil.IsEOF(err) {
				break
			}
			break
		}

		streamIdx := avcodec.GetPacketStreamIndex(d.packet)
		if int(streamIdx) != d.videoStreamIdx {
			avcodec.PacketUnref(d.packet)
			continue
		}

		// Check if keyframe (AV_PKT_FLAG_KEY = 1)
		flags := avcodec.GetPacketFlags(d.packet)
		if flags&1 != 0 {
			pts := avcodec.GetPacketPTS(d.packet)
			pos := avcodec.GetPacketPos(d.packet)

			// Convert PTS to time
			var timeDur time.Duration
			if tbDen != 0 {
				timeUS := pts * int64(tbNum) * 1000000 / int64(tbDen)
				timeDur = time.Duration(timeUS) * time.Microsecond
			}

			// Estimate frame number
			var frameNum int64
			if fpsNum != 0 && fpsDen != 0 && tbDen != 0 {
				frameNum = pts * int64(fpsNum) * int64(tbNum) / (int64(fpsDen) * int64(tbDen))
			}

			keyframes = append(keyframes, Keyframe{
				PTS:      pts,
				Time:     timeDur,
				Position: pos,
				Frame:    frameNum,
			})
		}

		avcodec.PacketUnref(d.packet)
	}

	// Seek back to beginning
	avformat.SeekFrame(d.formatCtx, -1, 0, avformat.SeekFlagBackward)
	if d.videoCodecCtx != nil {
		avcodec.FlushBuffers(d.videoCodecCtx)
	}

	return keyframes, nil
}
