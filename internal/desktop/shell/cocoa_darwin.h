// C interface of the macOS window shim (cocoa_darwin.m), see cocoa_darwin.go.
#ifndef ARENILE_SHELL_COCOA_H
#define ARENILE_SHELL_COCOA_H

void arenile_shell_install(void *nswindow, const char *menu_json);
void arenile_shell_uninstall(void);
void arenile_shell_show(void);
void arenile_shell_hide(void);
void arenile_shell_set_zoom(double level);
void arenile_shell_print(void);
int arenile_shell_get_frame(double *x, double *y, double *w, double *h, int *zoomed);
void arenile_shell_set_frame(double x, double y, double w, double h);
void arenile_shell_set_zoomed(void);
void arenile_shell_set_accessory(void);

#endif
