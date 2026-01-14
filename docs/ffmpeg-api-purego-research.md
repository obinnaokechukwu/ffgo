# FFmpeg C API for PureGo Bindings Research Notes

## Executive Summary

Creating Go bindings for FFmpeg using purego is **partially feasible with significant workarounds**. The core decode/encode workflow is compatible, but several important features require workarounds or cannot be supported.

---

## 1. VARIADIC FUNCTIONS (BLOCKERS)

Purego **cannot call variadic functions**. FFmpeg has many in its public API.

### Critical Variadic Functions Found:

**libavutil/log.h (lines 268, 289):**
```c
void av_log(void *avcl, int level, const char *fmt, ...) av_printf_format(3, 4);
void av_log_once(void* avcl, int initial_level, int subsequent_level, int *state, const char *fmt, ...) av_printf_format(5, 6);
```

**libavutil/avstring.h:**
```c
int av_asprintf(char **s, const char *fmt, ...);
int av_sscanf(const char *string, const char *format, ...);
```

**libavutil/bprint.h:**
```c
void av_bprintf(AVBPrint *buf, const char *fmt, ...);
```

**libavutil/dict.h:**
```c
int av_dict_set_int(AVDictionary **pm, const char *key, int64_t value, int flags); // not variadic but...
```

**libavutil/error.h:**
```c
int av_strerror(int errnum, char *errbuf, size_t errbuf_size);
```

**libavformat/avio.h (line ~580+):**
```c
int avio_printf(AVIOContext *s, const char *fmt, ...);
```

### Workaround Assessment:
- **av_log**: FFmpeg provides `av_vlog()` (line 307 of log.h) which takes `va_list` - but purego cannot construct `va_list`
- **WORKAROUND**: Set a custom log callback via `av_log_set_callback()` to capture logs, or ignore logging entirely
- For most variadic functions, there is **no direct workaround** - you must avoid them or use pre-formatted strings

---

## 2. CALLBACK SIGNATURES

### Problematic Callbacks (String Parameters):

**libavutil/log.h (line 337):**
```c
void av_log_set_callback(void (*callback)(void*, int, const char*, va_list));
```
- **PROBLEM**: `const char *fmt` is a string parameter, and `va_list` is not supported
- **VERDICT**: INCOMPATIBLE with purego callbacks

**libavformat/avformat.h (line 1239):**
```c
typedef int (*AVOpenCallback)(struct AVFormatContext *s, AVIOContext **pb, const char *url, int flags,
                              const AVIOInterruptCB *int_cb, AVDictionary **options);
```
- **PROBLEM**: `const char *url` is a string parameter
- **VERDICT**: INCOMPATIBLE

**libavformat/internal.h (line 498):**
```c
typedef void (*ff_parse_key_val_cb)(void *context, const char *key,
                                    int key_len, char **dest, int *dest_len);
```
- **PROBLEM**: Multiple string parameters
- **VERDICT**: INCOMPATIBLE

### Safe Callbacks (No String/Slice Args, No Float/Struct Returns):

**libavcodec/avcodec.h (line 740-742):**
```c
void (*draw_horiz_band)(struct AVCodecContext *s,
                        const AVFrame *src, int offset[AV_NUM_DATA_POINTERS],
                        int y, int type, int height);
```
- **VERDICT**: SAFE (pointer params only, void return)

**libavcodec/avcodec.h (line 769):**
```c
enum AVPixelFormat (*get_format)(struct AVCodecContext *s, const enum AVPixelFormat * fmt);
```
- **VERDICT**: SAFE (returns enum/int, pointer params)

**libavcodec/avcodec.h (line 1208):**
```c
int (*get_buffer2)(struct AVCodecContext *s, AVFrame *frame, int flags);
```
- **VERDICT**: SAFE (int return, pointer params)

**libavcodec/avcodec.h (lines 1599, 1618):**
```c
int (*execute)(struct AVCodecContext *c, int (*func)(struct AVCodecContext *c2, void *arg), void *arg2, int *ret, int count, int size);
int (*execute2)(struct AVCodecContext *c, int (*func)(struct AVCodecContext *c2, void *arg, int jobnr, int threadnr), void *arg2, int *ret, int count);
```
- **VERDICT**: SAFE (int return, pointer and int params)

**libavformat/avio.h (AVIOContext structure, lines 234-257):**
```c
int (*read_packet)(void *opaque, uint8_t *buf, int buf_size);
int (*write_packet)(void *opaque, const uint8_t *buf, int buf_size);
int64_t (*seek)(void *opaque, int64_t offset, int whence);
int (*read_pause)(void *opaque, int pause);
int64_t (*read_seek)(void *opaque, int stream_index, int64_t timestamp, int flags);
```
- **VERDICT**: ALL SAFE - These are the custom I/O callbacks, all compatible

**libavformat/avio.h (line 283-284):**
```c
int (*write_data_type)(void *opaque, const uint8_t *buf, int buf_size,
                       enum AVIODataMarkerType type, int64_t time);
```
- **VERDICT**: SAFE

**libavformat/avio.h (line 246):**
```c
unsigned long (*update_checksum)(unsigned long checksum, const uint8_t *buf, unsigned int size);
```
- **VERDICT**: SAFE

**libavutil/tx.h (line 151):**
```c
typedef void (*av_tx_fn)(AVTXContext *s, void *out, void *in, ptrdiff_t stride);
```
- **VERDICT**: SAFE

---

## 3. STRUCT ANALYSIS

### Structs Passed/Returned by VALUE (PROBLEMATIC on non-Darwin):

**AVRational** (libavutil/rational.h lines 58-61):
```c
typedef struct AVRational{
    int num;
    int den;
} AVRational;
```

Functions returning AVRational by value:
- `./ffmpeg/libavutil/rational.h:128`: `AVRational av_mul_q(AVRational b, AVRational c)`
- `./ffmpeg/libavutil/rational.h:136`: `AVRational av_div_q(AVRational b, AVRational c)`
- `./ffmpeg/libavutil/rational.h:144`: `AVRational av_add_q(AVRational b, AVRational c)`
- `./ffmpeg/libavutil/rational.h:152`: `AVRational av_sub_q(AVRational b, AVRational c)`
- `./ffmpeg/libavutil/rational.h:180`: `AVRational av_d2q(double d, int max)`
- `./ffmpeg/libavutil/rational.h:219`: `AVRational av_gcd_q(AVRational a, AVRational b, int max_den, AVRational def)`
- `./ffmpeg/libavformat/avformat.h:2957`: `AVRational av_guess_sample_aspect_ratio(...)`
- `./ffmpeg/libavformat/avformat.h:2968`: `AVRational av_guess_frame_rate(...)`
- `./ffmpeg/libavformat/avformat.h:3011`: `AVRational av_stream_get_codec_timebase(...)`
- `./ffmpeg/libavfilter/buffersink.h:110`: `AVRational av_buffersink_get_time_base(...)`
- `./ffmpeg/libavfilter/buffersink.h:113`: `AVRational av_buffersink_get_frame_rate(...)`
- `./ffmpeg/libavfilter/buffersink.h:116`: `AVRational av_buffersink_get_sample_aspect_ratio(...)`

**WORKAROUND**: AVRational is only 8 bytes (two ints). On non-Darwin, implement these math operations in Go instead.

### AVChannelLayout (libavutil/channel_layout.h lines 319-377):
```c
typedef struct AVChannelLayout {
    enum AVChannelOrder order;  // 4 bytes
    int nb_channels;            // 4 bytes
    union {
        uint64_t mask;
        AVChannelCustom *map;
    } u;                        // 8 bytes
    void *opaque;               // 8 bytes
} AVChannelLayout;              // Total: 24 bytes (likely with padding)
```
- This struct is embedded in AVCodecContext (line 1047) and AVFrame
- It contains a union and pointer, making padding complex
- **sizeof(AVChannelLayout) IS part of public ABI** per comment on line 299-301
- **VERDICT**: Can work but requires careful manual padding calculation

### Major Opaque Structs (Accessed Only via Pointers - SAFE):

1. **AVCodecContext** (libavcodec/avcodec.h line 439) - "sizeof(AVCodecContext) must not be used outside libav*"
2. **AVFormatContext** (libavformat/avformat.h) - "size is not part of public ABI"
3. **AVFrame** (libavutil/frame.h line 421) - "sizeof(AVFrame) is not a part of the public ABI"
4. **AVPacket** (libavcodec/packet.h line 556) - Note says sizeof is deprecated
5. **AVIOContext** (libavformat/avio.h line 160) - "sizeof(AVIOContext) must not be used outside libav*"
6. **SwsContext** (libswscale/swscale.h line 187) - "sizeof(SwsContext) is not part of the ABI"

**VERDICT**: All major structs are opaque and allocated through FFmpeg functions - this is GOOD for purego.

---

## 4. FUNCTION SIGNATURE SURVEY

### Functions with Many Arguments (Max 15 allowed):

Most functions are well under 15 arguments. Found one borderline case in internal headers:
- `./ffmpeg/libavcodec/snow.h:223`: `add_yblock(...)` - 19+ params, but this is internal

### Core API Functions - ALL COMPATIBLE:

**libavcodec:**
```c
int avcodec_open2(AVCodecContext *avctx, const AVCodec *codec, AVDictionary **options);  // 3 args
int avcodec_send_packet(AVCodecContext *avctx, const AVPacket *avpkt);                   // 2 args
int avcodec_receive_frame(AVCodecContext *avctx, AVFrame *frame);                        // 2 args
int avcodec_send_frame(AVCodecContext *avctx, const AVFrame *frame);                     // 2 args
int avcodec_receive_packet(AVCodecContext *avctx, AVPacket *avpkt);                      // 2 args
```

**libavformat:**
```c
AVFormatContext *avformat_alloc_context(void);                                           // 0 args
int avformat_open_input(AVFormatContext **ps, const char *url, ...);                     // 4 args
int avformat_find_stream_info(AVFormatContext *ic, AVDictionary **options);              // 2 args
int av_read_frame(AVFormatContext *s, AVPacket *pkt);                                    // 2 args
int avformat_write_header(AVFormatContext *s, AVDictionary **options);                   // 2 args
int av_write_frame(AVFormatContext *s, AVPacket *pkt);                                   // 2 args
int av_interleaved_write_frame(AVFormatContext *s, AVPacket *pkt);                       // 2 args
void avformat_close_input(AVFormatContext **s);                                          // 1 arg
```

**libavutil:**
```c
AVFrame *av_frame_alloc(void);                                                           // 0 args
void av_frame_free(AVFrame **frame);                                                     // 1 arg
int av_frame_ref(AVFrame *dst, const AVFrame *src);                                      // 2 args
void av_frame_unref(AVFrame *frame);                                                     // 1 arg
int av_frame_get_buffer(AVFrame *frame, int align);                                      // 2 args
```

**libswscale:**
```c
SwsContext *sws_getContext(int srcW, int srcH, enum AVPixelFormat srcFormat,
                           int dstW, int dstH, enum AVPixelFormat dstFormat,
                           int flags, SwsFilter *srcFilter,
                           SwsFilter *dstFilter, const double *param);                   // 10 args - SAFE
int sws_scale(SwsContext *c, const uint8_t *const srcSlice[],
              const int srcStride[], int srcSliceY, int srcSliceH,
              uint8_t *const dst[], const int dstStride[]);                              // 7 args - SAFE
```

**avio custom I/O:**
```c
AVIOContext *avio_alloc_context(
                  unsigned char *buffer,
                  int buffer_size,
                  int write_flag,
                  void *opaque,
                  int (*read_packet)(void *opaque, uint8_t *buf, int buf_size),
                  int (*write_packet)(void *opaque, const uint8_t *buf, int buf_size),
                  int64_t (*seek)(void *opaque, int64_t offset, int whence));            // 7 args - SAFE
```

---

## 5. CRITICAL PATH ANALYSIS

### Decoding Video (Typical Workflow):

1. `avformat_open_input()` - SAFE
2. `avformat_find_stream_info()` - SAFE
3. `avcodec_find_decoder()` - SAFE
4. `avcodec_alloc_context3()` - SAFE
5. `avcodec_parameters_to_context()` - SAFE
6. `avcodec_open2()` - SAFE
7. `av_frame_alloc()` - SAFE
8. `av_packet_alloc()` - SAFE
9. Loop:
   - `av_read_frame()` - SAFE
   - `avcodec_send_packet()` - SAFE
   - `avcodec_receive_frame()` - SAFE
10. `avformat_close_input()` - SAFE

**VERDICT: Core decoding workflow is fully compatible with purego**

### Encoding Video:

1. `avformat_alloc_output_context2()` - SAFE
2. `avcodec_find_encoder()` - SAFE
3. `avformat_new_stream()` - SAFE
4. `avcodec_alloc_context3()` - SAFE
5. `avcodec_open2()` - SAFE
6. `avio_open()` - SAFE
7. `avformat_write_header()` - SAFE
8. Loop:
   - `avcodec_send_frame()` - SAFE
   - `avcodec_receive_packet()` - SAFE
   - `av_write_frame()` - SAFE
9. `av_write_trailer()` - SAFE
10. `avio_close()` - SAFE

**VERDICT: Core encoding workflow is fully compatible with purego**

### Custom I/O:
- `avio_alloc_context()` with custom callbacks - SAFE, all callbacks are compatible

### Scaling (libswscale):
- `sws_getContext()` - SAFE
- `sws_scale()` - SAFE
- `sws_scale_frame()` - SAFE
- `sws_freeContext()` - SAFE

**VERDICT: libswscale is fully compatible**

---

## 6. SUMMARY TABLE

| Feature | Compatibility | Notes |
|---------|---------------|-------|
| Core decode/encode | COMPATIBLE | Main workflow works |
| Custom I/O callbacks | COMPATIBLE | All callbacks are safe |
| Scaling (libswscale) | COMPATIBLE | No issues |
| Logging (av_log) | NOT COMPATIBLE | Variadic, no safe workaround |
| Log callback | NOT COMPATIBLE | String + va_list params |
| AVRational math | DARWIN ONLY | Struct by value returns |
| AVChannelLayout | CAREFUL | Embedded in structs, needs padding |
| AVOpenCallback | NOT COMPATIBLE | String parameter |
| Error formatting | WORKAROUND NEEDED | av_strerror is not variadic, works |
| String formatting | NOT COMPATIBLE | avio_printf etc are variadic |
| Filter graph | MOSTLY COMPATIBLE | Needs investigation |

---

## 7. RECOMMENDATIONS

1. **Feasible for core operations**: Decoding, encoding, muxing, demuxing, scaling all work
2. **Implement AVRational operations in Go**: Avoid FFmpeg's by-value return functions
3. **Skip logging**: Or implement your own error handling without av_log
4. **Use pre-formatted strings**: When FFmpeg needs string output, format in Go first
5. **Avoid variadic functions entirely**: There's no workaround
6. **Test on Darwin first**: Struct returns are safer there
7. **Calculate struct padding manually**: For AVChannelLayout and any embedded structs
8. **Max callbacks ~2000**: FFmpeg won't hit this limit in normal use

**Overall Assessment**: A purego-based FFmpeg binding is **viable for the common use case** of media transcoding/processing, but certain advanced features (logging, some callbacks) will need to be omitted or worked around.
