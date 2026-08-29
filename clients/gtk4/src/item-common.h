/*
 * item-common.h - shared feed item model, card widget and helpers used by
 * both the Feed and Saved views.
 */

#ifndef FEED_ITEM_COMMON_H
#define FEED_ITEM_COMMON_H

#include <gtk/gtk.h>
#include <json-glib/json-glib.h>

G_BEGIN_DECLS

/* ---------- FeedItem ---------- */

typedef struct {
  char *id;
  char *title;
  char *link;
  char *source_name;
  char *subscription;
  char *thumbnail_url;
  gboolean thumbnail_contain;
  GPtrArray *paragraphs; /* char* */
  char *published_at;
  char *fetched_at;
  gint64 vote;   /* -1, 0 or 1 */
  gboolean saved;
} FeedItem;

FeedItem *feed_item_new(void);
FeedItem *feed_item_new_from_json(JsonObject *object);
void feed_item_free(FeedItem *item);

/* Parses {"total":int,"items":[...]} or {"items":[...]}. Returns NULL on
 * malformed input; otherwise a GPtrArray of FeedItem (free func set). */
GPtrArray *feed_items_from_json(JsonNode *root, gint64 *total_out);

/* Formats an ISO 8601 timestamp for display; result must be g_free()d. */
char *feed_format_timestamp(const char *iso8601);

/* ---------- FeedCard ---------- */

#define FEED_TYPE_CARD (feed_card_get_type())
G_DECLARE_FINAL_TYPE(FeedCard, feed_card, FEED, CARD, GtkBox)

/* Takes ownership of `item`. When `show_votes` is FALSE the up/down/clear
 * vote buttons are omitted (used by the Saved view). */
GtkWidget *feed_card_new(FeedItem *item, gboolean show_votes);

void feed_card_set_vote(FeedCard *card, gint64 vote);
void feed_card_set_saved(FeedCard *card, gboolean saved);
FeedItem *feed_card_get_item(FeedCard *card); /* borrowed */

/* Signals: "vote" (gint64), "save" (gboolean), "open" (void). */

/* ---------- thumbnail loading ---------- */

/* Loads a remote image into `picture` asynchronously over HTTP (no auth
 * headers are sent for image requests). Results are cached by URL. */
void item_image_load_into(GtkPicture *picture, const char *url);

G_END_DECLS

#endif /* FEED_ITEM_COMMON_H */
