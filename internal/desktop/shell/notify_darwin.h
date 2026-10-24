// C interface of the macOS notification shim (notify_darwin.m), see notify_darwin.go.
#ifndef ARENILE_SHELL_NOTIFY_H
#define ARENILE_SHELL_NOTIFY_H

#include <stdint.h>

int arenile_notify_post(const char *title, const char *body, const char *target, uintptr_t fallback);

#endif
