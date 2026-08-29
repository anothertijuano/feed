/*
 * feed-view.h - main feed list with infinite scrolling, voting, saving and
 * a refresh button that triggers a server-side fetch.
 */

#ifndef FEED_VIEW_H
#define FEED_VIEW_H

#include <gtk/gtk.h>

#include "feed-window.h"

G_BEGIN_DECLS

#define FEED_TYPE_VIEW (feed_view_get_type())
G_DECLARE_FINAL_TYPE(FeedView, feed_view, FEED, VIEW, GtkBox)

GtkWidget *feed_view_new(FeedWindow *window);

/* Header bar extras (refresh button). */
GtkWidget *feed_view_get_extras(FeedView *self);

/* Called when the view becomes visible. */
void feed_view_on_show(FeedView *self, FeedWindow *window);

/* Reloads the first page (also used after settings changes). */
void feed_view_reload(FeedView *self);

G_END_DECLS

#endif /* FEED_VIEW_H */
