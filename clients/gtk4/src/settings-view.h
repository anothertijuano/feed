/*
 * settings-view.h - the Settings view: server/token entry, connection test,
 * memos settings and persistence through the config module.
 */

#ifndef FEED_SETTINGS_VIEW_H
#define FEED_SETTINGS_VIEW_H

#include <gtk/gtk.h>

#include "feed-window.h"

G_BEGIN_DECLS

#define FEED_TYPE_SETTINGS_VIEW (settings_view_get_type())
G_DECLARE_FINAL_TYPE(SettingsView, settings_view, FEED, SETTINGS_VIEW, GtkBox)

GtkWidget *settings_view_new(FeedWindow *window);

/* Signals: "saved" (void) — emitted after settings were persisted locally. */

G_END_DECLS

#endif /* FEED_SETTINGS_VIEW_H */
