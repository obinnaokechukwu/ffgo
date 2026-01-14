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
 * VERSION INFO
 * ============================================================================ */

unsigned int ffshim_avutil_version(void);
unsigned int ffshim_avcodec_version(void);
unsigned int ffshim_avformat_version(void);

#ifdef __cplusplus
}
#endif

#endif /* FFSHIM_H */
