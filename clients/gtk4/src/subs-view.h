/*
 * subs-view.h - the Subscriptions view: list of subscriptions with
 * notification policy badges, add/remove actions and a refresh-all button.
 */

#ifndef FEED_SUBS_VIEW_H
#define FEED_SUBS_VIEW_H

#include <gtk/gtk.h>

#include "feed-window.h"

G_BEGIN_DECLS

#define FEED_TYPE_SUBS_VIEW (subs_view_get_type())
G_DECLARE_FINAL_TYPE(SubsView, subs_view, FEED, SUBS_VIEW, GtkBox)

GtkWidget *subs_view_new(FeedWindow *window);

/* Header bar extras (add + refresh-all buttons). */
GtkWidget *subs_view_get_extras(SubsView *self);

/* Called when the view becomes visible. */
void subs_view_on_show(SubsView *self, FeedWindow *window);

/* Reloads the subscription list (also used after settings changes). */
void subs_view_reload(SubsView *self);

G_END_DECLS

#endif /* FEED_SUBS_VIEW_H */
