//go:build !ios && !android && (amd64 || arm64)

package avutil

// PixelFormat represents FFmpeg pixel formats.
type PixelFormat int32

// Common pixel formats (from FFmpeg's pixfmt.h)
const (
	PixelFormatNone    PixelFormat = -1
	PixelFormatYUV420P PixelFormat = 0  // Planar YUV 4:2:0
	PixelFormatYUYV422 PixelFormat = 1  // Packed YUV 4:2:2
	PixelFormatRGB24   PixelFormat = 2  // Packed RGB 8:8:8
	PixelFormatBGR24   PixelFormat = 3  // Packed BGR 8:8:8
	PixelFormatYUV422P PixelFormat = 4  // Planar YUV 4:2:2
	PixelFormatYUV444P PixelFormat = 5  // Planar YUV 4:4:4
	PixelFormatYUV410P PixelFormat = 6  // Planar YUV 4:1:0
	PixelFormatYUV411P PixelFormat = 7  // Planar YUV 4:1:1
	PixelFormatGray8   PixelFormat = 8  // 8-bit grayscale
	PixelFormatMonoW   PixelFormat = 9  // 1-bit monochrome
	PixelFormatMonoB   PixelFormat = 10 // 1-bit monochrome (black)
	PixelFormatPAL8    PixelFormat = 11 // 8-bit palette
	PixelFormatYUVJ420P PixelFormat = 12 // Planar YUV 4:2:0 (JPEG)
	PixelFormatYUVJ422P PixelFormat = 13 // Planar YUV 4:2:2 (JPEG)
	PixelFormatYUVJ444P PixelFormat = 14 // Planar YUV 4:4:4 (JPEG)

	PixelFormatNV12     PixelFormat = 23 // Planar YUV 4:2:0 (UV interleaved)
	PixelFormatNV21     PixelFormat = 24 // Planar YUV 4:2:0 (VU interleaved)
	PixelFormatARGB     PixelFormat = 25 // Packed ARGB 8:8:8:8
	PixelFormatRGBA     PixelFormat = 26 // Packed RGBA 8:8:8:8
	PixelFormatABGR     PixelFormat = 27 // Packed ABGR 8:8:8:8
	PixelFormatBGRA     PixelFormat = 28 // Packed BGRA 8:8:8:8
	PixelFormatGray16BE PixelFormat = 29 // 16-bit grayscale (big endian)
	PixelFormatGray16LE PixelFormat = 30 // 16-bit grayscale (little endian)

	PixelFormatRGB48BE PixelFormat = 41 // Packed RGB 16:16:16 (big endian)
	PixelFormatRGB48LE PixelFormat = 42 // Packed RGB 16:16:16 (little endian)
	PixelFormatRGBA64BE PixelFormat = 63 // Packed RGBA 16:16:16:16 (big endian)
	PixelFormatRGBA64LE PixelFormat = 64 // Packed RGBA 16:16:16:16 (little endian)
)

// MediaType represents FFmpeg media types.
type MediaType int32

const (
	MediaTypeUnknown    MediaType = -1
	MediaTypeVideo      MediaType = 0
	MediaTypeAudio      MediaType = 1
	MediaTypeData       MediaType = 2
	MediaTypeSubtitle   MediaType = 3
	MediaTypeAttachment MediaType = 4
)

// SampleFormat represents FFmpeg audio sample formats.
type SampleFormat int32

const (
	SampleFormatNone SampleFormat = -1
	SampleFormatU8   SampleFormat = 0  // Unsigned 8-bit
	SampleFormatS16  SampleFormat = 1  // Signed 16-bit
	SampleFormatS32  SampleFormat = 2  // Signed 32-bit
	SampleFormatFlt  SampleFormat = 3  // Float 32-bit
	SampleFormatDbl  SampleFormat = 4  // Float 64-bit
	SampleFormatU8P  SampleFormat = 5  // Unsigned 8-bit planar
	SampleFormatS16P SampleFormat = 6  // Signed 16-bit planar
	SampleFormatS32P SampleFormat = 7  // Signed 32-bit planar
	SampleFormatFltP SampleFormat = 8  // Float 32-bit planar
	SampleFormatDblP SampleFormat = 9  // Float 64-bit planar
	SampleFormatS64  SampleFormat = 10 // Signed 64-bit
	SampleFormatS64P SampleFormat = 11 // Signed 64-bit planar
)
