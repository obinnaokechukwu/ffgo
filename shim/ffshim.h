/*
 * ffshim.h - FFmpeg shim library for ffgo
 *
 * This shim provides wrappers for FFmpeg functionality that purego cannot handle:
 * 1. Variadic functions (av_log, avio_printf)
 * 2. Struct-by-value returns on non-Darwin platforms (AVRational functions)
 * 3. Log callback with va_list parameter
 */

#ifndef FFSHIM_H
#define FFSHIM_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Callback type that Go can implement (no va_list) */
typedef void (*ffshim_log_callback_t)(void *avcl, int level, const char *msg);

/* ============================================================================
 * LOGGING SUBSYSTEM
 * ============================================================================ */

/* Set a custom log callback. Pass NULL to restore default. */
void ffshim_log_set_callback(ffshim_log_callback_t cb);

/* Set the log level */
void ffshim_log_set_level(int level);

/* Log a pre-formatted message */
void ffshim_log(void *avcl, int level, const char *msg);

/* ============================================================================
 * AVRATIONAL OPERATIONS (for non-Darwin platforms)
 * ============================================================================ */

void ffshim_rational_mul(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den);
void ffshim_rational_div(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den);
void ffshim_rational_add(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den);
void ffshim_rational_sub(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den);
void ffshim_d2q(double d, int max_den, int *out_num, int *out_den);
double ffshim_q2d(int num, int den);
int ffshim_rational_cmp(int a_num, int a_den, int b_num, int b_den);

/* ============================================================================
 * ERROR HANDLING
 * ============================================================================ */

int ffshim_strerror(int errnum, char *errbuf, size_t errbuf_size);

/* ============================================================================
 * AVIO HELPERS
 * ============================================================================ */

void ffshim_avio_write_string(void *avio_ctx, const char *str);

/* ============================================================================
 * CHAPTER HELPERS
 * ============================================================================ */

/* Create a new chapter in the format context */
void* ffshim_new_chapter(void *ctx, int64_t id, int tb_num, int tb_den, int64_t start, int64_t end, void *metadata);

/* ============================================================================
 * VERSION INFO
 * ============================================================================ */

unsigned int ffshim_avutil_version(void);
unsigned int ffshim_avcodec_version(void);
unsigned int ffshim_avformat_version(void);

/* ============================================================================
 * AVDEVICE HELPERS (OPTIONAL)
 * ============================================================================ */

/*
 * List input sources for a given device input format.
 *
 * - format_name: avdevice demuxer name (e.g. "v4l2", "alsa", "dshow", "avfoundation")
 * - device_name: optional selector string (nullable)
 * - avdict_opts: AVDictionary* (nullable) - forwarded to FFmpeg
 * - out_count: number of devices returned
 * - out_names/out_descs: char** arrays allocated by the shim (must be freed)
 *
 * Returns 0 on success, negative AVERROR on failure.
 */
int ffshim_avdevice_list_input_sources(
    const char *format_name,
    const char *device_name,
    void *avdict_opts,
    int *out_count,
    char ***out_names,
    char ***out_descs
);

/* Free a char** array allocated by the shim using FFmpeg allocators. */
void ffshim_avdevice_free_string_array(char **arr, int count);

/* ============================================================================
 * AVFRAME OFFSET HELPERS (OPTIONAL)
 * ============================================================================ */

/*
 * Returns offsets (in bytes) for AVFrame color-related fields.
 *
 * Returns 0 on success, -1 on failure.
 */
int ffshim_avframe_color_offsets(
    int *out_color_range,
    int *out_colorspace,
    int *out_color_primaries,
    int *out_color_trc
);

/* ============================================================================
 * CODEC FIELD HELPERS (OPTIONAL)
 * ============================================================================ */

/* AVCodecParameters field accessors */
int ffshim_codecpar_width(void *par);
int ffshim_codecpar_height(void *par);
int ffshim_codecpar_format(void *par);
int ffshim_codecpar_sample_rate(void *par);
int ffshim_codecpar_channels(void *par);

/* AVCodecContext field accessors */
int ffshim_codecctx_width(void *ctx);
void ffshim_codecctx_set_width(void *ctx, int width);
int ffshim_codecctx_height(void *ctx);
void ffshim_codecctx_set_height(void *ctx, int height);
int ffshim_codecctx_pix_fmt(void *ctx);
void ffshim_codecctx_set_pix_fmt(void *ctx, int pix_fmt);
int ffshim_codecctx_sample_fmt(void *ctx);
void ffshim_codecctx_set_sample_fmt(void *ctx, int sample_fmt);
void ffshim_codecctx_time_base(void *ctx, int *out_num, int *out_den);
void ffshim_codecctx_set_time_base(void *ctx, int num, int den);
void ffshim_codecctx_framerate(void *ctx, int *out_num, int *out_den);
void ffshim_codecctx_set_framerate(void *ctx, int num, int den);
void ffshim_codecctx_set_ch_layout_default(void *ctx, int nb_channels);

#ifdef __cplusplus
}
#endif

#endif /* FFSHIM_H */
