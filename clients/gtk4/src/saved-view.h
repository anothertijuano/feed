/*
 * saved-view.h - the Saved view: newest-first list of saved items, without
 * vote controls. Un-saving an item removes its row.
 */

#ifndef FEED_SAVED_VIEW_H
#define FEED_SAVED_VIEW_H

#include <gtk/gtk.h>

#include "feed-window.h"

G_BEGIN_DECLS

#define FEED_TYPE_SAVED_VIEW (saved_view_get_type())
G_DECLARE_FINAL_TYPE(SavedView, saved_view, FEED, SAVED_VIEW, GtkBox)

GtkWidget *saved_view_new(FeedWindow *window);

/* Header bar extras (refresh button). */
GtkWidget *saved_view_get_extras(SavedView *self);

/* Called when the view becomes visible. */
void saved_view_on_show(SavedView *self, FeedWindow *window);

/* Reloads the saved list (also used after settings changes). */
void saved_view_reload(SavedView *self);

G_END_DECLS

#endif /* FEED_SAVED_VIEW_H */
