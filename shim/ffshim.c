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
