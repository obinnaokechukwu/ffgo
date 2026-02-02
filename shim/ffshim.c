/*
 * ffshim.c - FFmpeg shim library for ffgo
 *
 * Provides wrappers for FFmpeg functionality that purego cannot handle directly.
 */

#include "ffshim.h"

#include <libavutil/log.h>
#include <libavutil/rational.h>
#include <libavutil/error.h>
#include <libavutil/avutil.h>
#include <libavutil/mem.h>
#include <libavutil/frame.h>
#include <libavcodec/avcodec.h>
#include <libavformat/avformat.h>
#include <libavformat/avio.h>

#include <stdio.h>
#include <stdarg.h>
#include <string.h>

/* ============================================================================
 * LOGGING SUBSYSTEM
 * ============================================================================ */

/* Global callback pointer - set by Go */
static ffshim_log_callback_t g_log_callback = NULL;

/* Internal callback that FFmpeg calls - formats the message then calls Go */
static void internal_log_callback(void *avcl, int level, const char *fmt, va_list vl) {
    if (g_log_callback == NULL) {
        return;
    }

    char buf[4096];
    int len = vsnprintf(buf, sizeof(buf), fmt, vl);

    /* Remove trailing newline if present (Go will add its own) */
    if (len > 0 && len < (int)sizeof(buf) && buf[len-1] == '\n') {
        buf[len-1] = '\0';
    }

    g_log_callback(avcl, level, buf);
}

/* Called by Go to set up logging */
void ffshim_log_set_callback(ffshim_log_callback_t cb) {
    g_log_callback = cb;
    if (cb != NULL) {
        av_log_set_callback(internal_log_callback);
    } else {
        av_log_set_callback(av_log_default_callback);
    }
}

/* Called by Go to set log level */
void ffshim_log_set_level(int level) {
    av_log_set_level(level);
}

/* Called by Go to log a pre-formatted message */
void ffshim_log(void *avcl, int level, const char *msg) {
    av_log(avcl, level, "%s", msg);
}

/* ============================================================================
 * AVRATIONAL OPERATIONS (for non-Darwin platforms)
 * ============================================================================ */

void ffshim_rational_mul(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_mul_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_rational_div(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_div_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_rational_add(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_add_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_rational_sub(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_sub_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_d2q(double d, int max_den, int *out_num, int *out_den) {
    AVRational result = av_d2q(d, max_den);
    *out_num = result.num;
    *out_den = result.den;
}

double ffshim_q2d(int num, int den) {
    AVRational q = {num, den};
    return av_q2d(q);
}

int ffshim_rational_cmp(int a_num, int a_den, int b_num, int b_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    return av_cmp_q(a, b);
}

/* ============================================================================
 * ERROR HANDLING
 * ============================================================================ */

int ffshim_strerror(int errnum, char *errbuf, size_t errbuf_size) {
    return av_strerror(errnum, errbuf, errbuf_size);
}

/* ============================================================================
 * AVIO HELPERS
 * ============================================================================ */

void ffshim_avio_write_string(void *avio_ctx, const char *str) {
    avio_write((AVIOContext*)avio_ctx, (const unsigned char*)str, strlen(str));
}

/* ============================================================================
 * CHAPTER HELPERS
 * ============================================================================ */

void* ffshim_new_chapter(void *ctx, int64_t id, int tb_num, int tb_den, int64_t start, int64_t end, void *metadata) {
    if (ctx == NULL) {
        return NULL;
    }

    AVFormatContext *fc = (AVFormatContext *)ctx;
    AVChapter *ch = (AVChapter *)av_mallocz(sizeof(AVChapter));
    if (ch == NULL) {
        return NULL;
    }

    ch->id = id;
    ch->time_base = (AVRational){tb_num, tb_den};
    ch->start = start;
    ch->end = end;
    ch->metadata = (AVDictionary *)metadata; // take ownership

    // Grow chapters array
    unsigned int newCount = fc->nb_chapters + 1;
    AVChapter **newArr = (AVChapter **)av_realloc_array(fc->chapters, newCount, sizeof(AVChapter *));
    if (newArr == NULL) {
        // If we took metadata ownership, keep it with the chapter so caller doesn't free it.
        av_free(ch);
        return NULL;
    }
    fc->chapters = newArr;
    fc->chapters[fc->nb_chapters] = ch;
    fc->nb_chapters = newCount;

    return ch;
}

/* ============================================================================
 * VERSION INFO
 * ============================================================================ */

unsigned int ffshim_avutil_version(void) {
    return avutil_version();
}

unsigned int ffshim_avcodec_version(void) {
    return avcodec_version();
}

unsigned int ffshim_avformat_version(void) {
    return avformat_version();
}

/* ============================================================================
 * AVDEVICE HELPERS (OPTIONAL)
 * ============================================================================ */

#ifdef FFSHIM_HAVE_AVDEVICE

#include <libavdevice/avdevice.h>

static int g_avdevice_registered = 0;

int ffshim_avdevice_list_input_sources(
    const char *format_name,
    const char *device_name,
    void *avdict_opts,
    int *out_count,
    char ***out_names,
    char ***out_descs
) {
    if (out_count == NULL || out_names == NULL || out_descs == NULL) {
        return AVERROR(EINVAL);
    }

    *out_count = 0;
    *out_names = NULL;
    *out_descs = NULL;

    if (format_name == NULL || format_name[0] == '\0') {
        return AVERROR(EINVAL);
    }

    if (!g_avdevice_registered) {
        avdevice_register_all();
        g_avdevice_registered = 1;
    }

    AVInputFormat *fmt = av_find_input_format(format_name);
    if (fmt == NULL) {
        return AVERROR(EINVAL);
    }

    AVDeviceInfoList *list = NULL;
    AVDictionary *dict = (AVDictionary *)avdict_opts;

    int ret = avdevice_list_input_sources(fmt,
                                         (device_name != NULL && device_name[0] != '\0') ? device_name : NULL,
                                         dict,
                                         &list);
    if (ret < 0) {
        if (list != NULL) {
            avdevice_free_list_devices(&list);
        }
        return ret;
    }
    if (list == NULL || list->nb_devices <= 0) {
        if (list != NULL) {
            avdevice_free_list_devices(&list);
        }
        return 0;
    }

    int count = list->nb_devices;

    char **names = (char **)av_mallocz((size_t)count * sizeof(char *));
    char **descs = (char **)av_mallocz((size_t)count * sizeof(char *));
    if (names == NULL || descs == NULL) {
        if (names != NULL) av_free(names);
        if (descs != NULL) av_free(descs);
        avdevice_free_list_devices(&list);
        return AVERROR(ENOMEM);
    }

    for (int i = 0; i < count; i++) {
        AVDeviceInfo *dev = list->devices[i];
        const char *dn = (dev != NULL && dev->device_name != NULL) ? dev->device_name : "";
        const char *dd = (dev != NULL && dev->device_description != NULL) ? dev->device_description : "";
        names[i] = av_strdup(dn);
        descs[i] = av_strdup(dd);
    }

    avdevice_free_list_devices(&list);

    *out_count = count;
    *out_names = names;
    *out_descs = descs;
    return 0;
}

void ffshim_avdevice_free_string_array(char **arr, int count) {
    if (arr == NULL || count <= 0) {
        return;
    }
    for (int i = 0; i < count; i++) {
        if (arr[i] != NULL) {
            av_free(arr[i]);
        }
    }
    av_free(arr);
}

#else

int ffshim_avdevice_list_input_sources(
    const char *format_name,
    const char *device_name,
    void *avdict_opts,
    int *out_count,
    char ***out_names,
    char ***out_descs
) {
    (void)format_name;
    (void)device_name;
    (void)avdict_opts;
    if (out_count) *out_count = 0;
    if (out_names) *out_names = NULL;
    if (out_descs) *out_descs = NULL;
    return AVERROR(ENOSYS);
}

void ffshim_avdevice_free_string_array(char **arr, int count) {
    (void)arr;
    (void)count;
}

#endif

/* ============================================================================
 * AVFRAME OFFSET HELPERS (OPTIONAL)
 * ============================================================================ */

int ffshim_avframe_color_offsets(
    int *out_color_range,
    int *out_colorspace,
    int *out_color_primaries,
    int *out_color_trc
) {
    if (out_color_range == NULL || out_colorspace == NULL || out_color_primaries == NULL || out_color_trc == NULL) {
        return -1;
    }

    *out_color_range = (int)offsetof(AVFrame, color_range);
    *out_colorspace = (int)offsetof(AVFrame, colorspace);
    *out_color_primaries = (int)offsetof(AVFrame, color_primaries);
    *out_color_trc = (int)offsetof(AVFrame, color_trc);
    return 0;
}

/* ============================================================================
 * CODEC FIELD HELPERS (OPTIONAL)
 * ============================================================================ */

int ffshim_codecpar_width(void *par) {
    if (par == NULL) {
        return 0;
    }
    return ((AVCodecParameters*)par)->width;
}

int ffshim_codecpar_height(void *par) {
    if (par == NULL) {
        return 0;
    }
    return ((AVCodecParameters*)par)->height;
}

int ffshim_codecpar_format(void *par) {
    if (par == NULL) {
        return -1;
    }
    return ((AVCodecParameters*)par)->format;
}

int ffshim_codecpar_sample_rate(void *par) {
    if (par == NULL) {
        return 0;
    }
    return ((AVCodecParameters*)par)->sample_rate;
}

int ffshim_codecpar_channels(void *par) {
    if (par == NULL) {
        return 0;
    }
    return ((AVCodecParameters*)par)->ch_layout.nb_channels;
}

int ffshim_codecctx_width(void *ctx) {
    if (ctx == NULL) {
        return 0;
    }
    return ((AVCodecContext*)ctx)->width;
}

void ffshim_codecctx_set_width(void *ctx, int width) {
    if (ctx == NULL) {
        return;
    }
    ((AVCodecContext*)ctx)->width = width;
}

int ffshim_codecctx_height(void *ctx) {
    if (ctx == NULL) {
        return 0;
    }
    return ((AVCodecContext*)ctx)->height;
}

void ffshim_codecctx_set_height(void *ctx, int height) {
    if (ctx == NULL) {
        return;
    }
    ((AVCodecContext*)ctx)->height = height;
}

int ffshim_codecctx_pix_fmt(void *ctx) {
    if (ctx == NULL) {
        return -1;
    }
    return ((AVCodecContext*)ctx)->pix_fmt;
}

void ffshim_codecctx_set_pix_fmt(void *ctx, int pix_fmt) {
    if (ctx == NULL) {
        return;
    }
    ((AVCodecContext*)ctx)->pix_fmt = pix_fmt;
}

void ffshim_codecctx_time_base(void *ctx, int *out_num, int *out_den) {
    if (ctx == NULL || out_num == NULL || out_den == NULL) {
        return;
    }
    *out_num = ((AVCodecContext*)ctx)->time_base.num;
    *out_den = ((AVCodecContext*)ctx)->time_base.den;
}

void ffshim_codecctx_set_time_base(void *ctx, int num, int den) {
    if (ctx == NULL) {
        return;
    }
    ((AVCodecContext*)ctx)->time_base = (AVRational){num, den};
}

void ffshim_codecctx_framerate(void *ctx, int *out_num, int *out_den) {
    if (ctx == NULL || out_num == NULL || out_den == NULL) {
        return;
    }
    *out_num = ((AVCodecContext*)ctx)->framerate.num;
    *out_den = ((AVCodecContext*)ctx)->framerate.den;
}

void ffshim_codecctx_set_framerate(void *ctx, int num, int den) {
    if (ctx == NULL) {
        return;
    }
    ((AVCodecContext*)ctx)->framerate = (AVRational){num, den};
}
