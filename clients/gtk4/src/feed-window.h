/*
 * feed-window.h - application window with sidebar navigation, a shared
 * header bar (title, busy spinner, per-view actions) and a toast overlay.
 */

#ifndef FEED_WINDOW_H
#define FEED_WINDOW_H

#include <adwaita.h>

#include "api.h"
#include "config.h"

G_BEGIN_DECLS

#define FEED_TYPE_WINDOW (feed_window_get_type())
G_DECLARE_FINAL_TYPE(FeedWindow, feed_window, FEED, WINDOW, AdwApplicationWindow)

/* Takes ownership of `config`. */
FeedWindow *feed_window_new(AdwApplication *app, FeedConfig *config);

FeedApi *feed_window_get_api(FeedWindow *self);
FeedConfig *feed_window_get_config(FeedWindow *self);

void feed_window_busy_push(FeedWindow *self);
void feed_window_busy_pop(FeedWindow *self);
void feed_window_show_toast(FeedWindow *self, const char *message);

typedef void (*FeedViewShowFunc)(gpointer view, FeedWindow *window);

/* Registers a sidebar entry and a content page. `extras` (may be NULL) is a
 * widget shown in the header bar while the view is active; `on_show` (may be
 * NULL) is called whenever the view becomes the visible page. */
void feed_window_add_view(FeedWindow *self, const char *name,
                          const char *title, const char *icon_name,
                          GtkWidget *view, GtkWidget *extras,
                          FeedViewShowFunc on_show);

G_END_DECLS

#endif /* FEED_WINDOW_H */
