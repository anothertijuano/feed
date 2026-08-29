#include "item-common.h"

#include <libsoup/soup.h>

/* ---------- FeedItem ---------- */

FeedItem *
feed_item_new(void)
{
  FeedItem *item = g_new0(FeedItem, 1);
  item->paragraphs = g_ptr_array_new_with_free_func(g_free);
  return item;
}

void
feed_item_free(FeedItem *item)
{
  if (item == NULL)
    return;
  g_free(item->id);
  g_free(item->title);
  g_free(item->link);
  g_free(item->source_name);
  g_free(item->subscription);
  g_free(item->thumbnail_url);
  g_free(item->published_at);
  g_free(item->fetched_at);
  if (item->paragraphs != NULL)
    g_ptr_array_unref(item->paragraphs);
  g_free(item);
}

FeedItem *
feed_item_new_from_json(JsonObject *object)
{
  FeedItem *item = feed_item_new();

  item->id = g_strdup(json_object_get_string_member(object, "id"));
  item->title = g_strdup(json_object_get_string_member(object, "title"));
  item->link = g_strdup(json_object_get_string_member(object, "link"));
  item->source_name = g_strdup(json_object_get_string_member(object, "sourceName"));
  item->subscription = g_strdup(json_object_get_string_member(object, "subscription"));
  item->published_at = g_strdup(json_object_get_string_member(object, "publishedAt"));
  item->fetched_at = g_strdup(json_object_get_string_member(object, "fetchedAt"));
  item->vote = json_object_get_int_member(object, "vote");
  item->saved = json_object_get_boolean_member(object, "saved");

  JsonArray *media = json_object_get_array_member(object, "media");
  if (media != NULL && json_array_get_length(media) > 0) {
    JsonObject *first = json_array_get_object_element(media, 0);
    if (first != NULL) {
      item->thumbnail_url = g_strdup(json_object_get_string_member(first, "src"));
      item->thumbnail_contain = json_object_get_boolean_member(first, "contain");
    }
  }

  JsonArray *paragraphs = json_object_get_array_member(object, "paragraphs");
  if (paragraphs != NULL) {
    guint length = json_array_get_length(paragraphs);
    for (guint i = 0; i < length; i++) {
      const char *text = json_array_get_string_element(paragraphs, i);
      if (text != NULL)
        g_ptr_array_add(item->paragraphs, g_strdup(text));
    }
  }

  return item;
}

GPtrArray *
feed_items_from_json(JsonNode *root, gint64 *total_out)
{
  if (total_out != NULL)
    *total_out = -1;

  if (root == NULL || !JSON_NODE_HOLDS_OBJECT(root))
    return NULL;

  JsonObject *object = json_node_get_object(root);

  if (total_out != NULL && json_object_has_member(object, "total"))
    *total_out = json_object_get_int_member(object, "total");

  JsonArray *array = json_object_get_array_member(object, "items");
  if (array == NULL)
    return NULL;

  GPtrArray *items = g_ptr_array_new_with_free_func((GDestroyNotify) feed_item_free);
  guint length = json_array_get_length(array);
  for (guint i = 0; i < length; i++) {
    JsonObject *entry = json_array_get_object_element(array, i);
    if (entry != NULL)
      g_ptr_array_add(items, feed_item_new_from_json(entry));
  }
  return items;
}

/* ---------- timestamps ---------- */

char *
feed_format_timestamp(const char *iso8601)
{
  if (iso8601 == NULL || iso8601[0] == '\0')
    return g_strdup("");

  GDateTime *when = g_date_time_new_from_iso8601(iso8601, NULL);
  if (when == NULL)
    return g_strdup("");

  GDateTime *now = g_date_time_new_now_utc();
  GTimeSpan diff = g_date_time_difference(when, now); /* now - when, microseconds */
  g_date_time_unref(now);

  char *out;
  if (diff >= 0 && diff < 60 * G_TIME_SPAN_MINUTE) {
    gint64 minutes = diff / G_TIME_SPAN_MINUTE;
    out = g_strdup_printf(minutes <= 1 ? "1m ago" : "%" G_GINT64_FORMAT "m ago",
                          minutes);
  } else if (diff >= 0 && diff < 24 * G_TIME_SPAN_HOUR) {
    gint64 hours = diff / G_TIME_SPAN_HOUR;
    out = g_strdup_printf(hours <= 1 ? "1h ago" : "%" G_GINT64_FORMAT "h ago",
                          hours);
  } else {
    GDateTime *local = g_date_time_to_local(when);
    out = g_date_time_format(local, "%b %d %Y");
    g_date_time_unref(local);
  }

  g_date_time_unref(when);
  return out;
}

/* ---------- FeedCard ---------- */

struct _FeedCard {
  GtkBox parent_instance;
  FeedItem *item;
  GtkToggleButton *up_btn;
  GtkToggleButton *down_btn;
  GtkButton *clear_btn;
  GtkToggleButton *save_btn;
  gboolean syncing; /* guards programmatic state syncs */
};

G_DEFINE_FINAL_TYPE(FeedCard, feed_card, GTK_TYPE_BOX)

enum {
  SIGNAL_VOTE,
  SIGNAL_SAVE,
  SIGNAL_OPEN,
  N_SIGNALS
};

static guint card_signals[N_SIGNALS];

static void
on_open_clicked(GtkButton *button, gpointer data)
{
  FeedCard *self = FEED_CARD(data);
  (void) button;
  g_signal_emit(self, card_signals[SIGNAL_OPEN], 0);
}

static void
on_up_toggled(GtkToggleButton *button, gpointer data)
{
  FeedCard *self = FEED_CARD(data);

  if (self->syncing)
    return;

  if (gtk_toggle_button_get_active(button)) {
    self->syncing = TRUE;
    gtk_toggle_button_set_active(self->down_btn, FALSE);
    self->syncing = FALSE;
    g_signal_emit(self, card_signals[SIGNAL_VOTE], 0, (gint64) 1);
  } else {
    g_signal_emit(self, card_signals[SIGNAL_VOTE], 0, (gint64) 0);
  }
}

static void
on_down_toggled(GtkToggleButton *button, gpointer data)
{
  FeedCard *self = FEED_CARD(data);

  if (self->syncing)
    return;

  if (gtk_toggle_button_get_active(button)) {
    self->syncing = TRUE;
    gtk_toggle_button_set_active(self->up_btn, FALSE);
    self->syncing = FALSE;
    g_signal_emit(self, card_signals[SIGNAL_VOTE], 0, (gint64) -1);
  } else {
    g_signal_emit(self, card_signals[SIGNAL_VOTE], 0, (gint64) 0);
  }
}

static void
on_clear_clicked(GtkButton *button, gpointer data)
{
  FeedCard *self = FEED_CARD(data);
  (void) button;
  g_signal_emit(self, card_signals[SIGNAL_VOTE], 0, (gint64) 0);
}

static void
on_save_toggled(GtkToggleButton *button, gpointer data)
{
  FeedCard *self = FEED_CARD(data);

  if (self->syncing)
    return;

  g_signal_emit(self, card_signals[SIGNAL_SAVE], 0,
                gtk_toggle_button_get_active(button));
}

static GtkWidget *
make_toggle_button(const char *icon_name, const char *tooltip,
                   GCallback toggled_cb, FeedCard *self)
{
  GtkWidget *button = gtk_toggle_button_new();
  GtkWidget *icon = gtk_image_new_from_icon_name(icon_name);
  gtk_button_set_child(GTK_BUTTON(button), icon);
  gtk_widget_set_valign(button, GTK_ALIGN_CENTER);
  gtk_widget_set_tooltip_text(button, tooltip);
  g_signal_connect(button, "toggled", toggled_cb, self);
  return button;
}

static void
feed_card_build(FeedCard *self, gboolean show_votes)
{
  GtkWidget *box = GTK_WIDGET(self);

  gtk_widget_add_css_class(box, "card");
  gtk_widget_set_margin_start(box, 12);
  gtk_widget_set_margin_end(box, 12);
  gtk_widget_set_margin_top(box, 6);
  gtk_widget_set_margin_bottom(box, 6);

  /* Header: source, time, open-in-browser. */
  GtkWidget *header = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);

  const char *source_name =
    self->item->source_name != NULL && self->item->source_name[0] != '\0'
      ? self->item->source_name : "Unknown source";
  GtkWidget *source = gtk_label_new(source_name);
  gtk_widget_add_css_class(source, "heading");
  gtk_label_set_xalign(GTK_LABEL(source), 0.0f);
  gtk_label_set_ellipsize(GTK_LABEL(source), PANGO_ELLIPSIZE_END);
  gtk_widget_set_hexpand(source, TRUE);
  gtk_box_append(GTK_BOX(header), source);

  const char *stamp =
    self->item->published_at != NULL && self->item->published_at[0] != '\0'
      ? self->item->published_at : self->item->fetched_at;
  g_autofree char *when = feed_format_timestamp(stamp);
  GtkWidget *time_label = gtk_label_new(when);
  gtk_widget_add_css_class(time_label, "caption");
  gtk_widget_add_css_class(time_label, "dim-label");
  gtk_widget_set_valign(time_label, GTK_ALIGN_CENTER);
  gtk_box_append(GTK_BOX(header), time_label);

  GtkWidget *open = gtk_button_new_from_icon_name("web-browser-symbolic");
  gtk_button_set_has_frame(GTK_BUTTON(open), FALSE);
  gtk_widget_set_valign(open, GTK_ALIGN_CENTER);
  gtk_widget_set_tooltip_text(open, "Open in browser");
  g_signal_connect(open, "clicked", G_CALLBACK(on_open_clicked), self);
  gtk_box_append(GTK_BOX(header), open);

  gtk_box_append(GTK_BOX(box), header);

  /* Content: text column plus optional thumbnail. */
  GtkWidget *content = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
  GtkWidget *texts = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
  gtk_widget_set_hexpand(texts, TRUE);

  GtkWidget *title = gtk_label_new(self->item->title);
  gtk_label_set_wrap(GTK_LABEL(title), TRUE);
  gtk_label_set_xalign(GTK_LABEL(title), 0.0f);
  gtk_label_set_lines(GTK_LABEL(title), 2);
  gtk_label_set_ellipsize(GTK_LABEL(title), PANGO_ELLIPSIZE_END);
  gtk_widget_add_css_class(title, "title-4");
  gtk_box_append(GTK_BOX(texts), title);

  const char *snippet =
    self->item->paragraphs != NULL && self->item->paragraphs->len > 0
      ? g_ptr_array_index(self->item->paragraphs, 0) : NULL;
  if (snippet != NULL) {
    GtkWidget *body = gtk_label_new(snippet);
    gtk_label_set_wrap(GTK_LABEL(body), TRUE);
    gtk_label_set_xalign(GTK_LABEL(body), 0.0f);
    gtk_label_set_lines(GTK_LABEL(body), 3);
    gtk_label_set_ellipsize(GTK_LABEL(body), PANGO_ELLIPSIZE_END);
    gtk_widget_add_css_class(body, "dim-label");
    gtk_box_append(GTK_BOX(texts), body);
  }

  gtk_box_append(GTK_BOX(content), texts);

  if (self->item->thumbnail_url != NULL && self->item->thumbnail_url[0] != '\0') {
    GtkWidget *picture = gtk_picture_new();
    gtk_picture_set_content_fit(GTK_PICTURE(picture),
                                self->item->thumbnail_contain
                                  ? GTK_CONTENT_FIT_CONTAIN
                                  : GTK_CONTENT_FIT_COVER);
    gtk_picture_set_can_shrink(GTK_PICTURE(picture), TRUE);
    gtk_widget_set_size_request(picture, 132, 74);
    gtk_widget_set_valign(picture, GTK_ALIGN_START);
    gtk_widget_add_css_class(picture, "feed-thumb");
    item_image_load_into(GTK_PICTURE(picture), self->item->thumbnail_url);
    gtk_box_append(GTK_BOX(content), picture);
  }

  gtk_box_append(GTK_BOX(box), content);

  /* Actions: votes (optional), then save on the right. */
  GtkWidget *actions = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);

  if (show_votes) {
    self->up_btn =
      GTK_TOGGLE_BUTTON(make_toggle_button("go-up-symbolic", "Upvote",
                                           G_CALLBACK(on_up_toggled), self));
    gtk_box_append(GTK_BOX(actions), GTK_WIDGET(self->up_btn));

    self->down_btn =
      GTK_TOGGLE_BUTTON(make_toggle_button("go-down-symbolic",
                                           "Downvote (permanently removes the item)",
                                           G_CALLBACK(on_down_toggled), self));
    gtk_box_append(GTK_BOX(actions), GTK_WIDGET(self->down_btn));

    GtkWidget *clear = gtk_button_new_from_icon_name("edit-clear-all-symbolic");
    self->clear_btn = GTK_BUTTON(clear);
    gtk_widget_set_valign(clear, GTK_ALIGN_CENTER);
    gtk_widget_set_tooltip_text(clear, "Clear vote");
    g_signal_connect(clear, "clicked", G_CALLBACK(on_clear_clicked), self);
    gtk_box_append(GTK_BOX(actions), clear);
  }

  GtkWidget *spacer = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
  gtk_widget_set_hexpand(spacer, TRUE);
  gtk_box_append(GTK_BOX(actions), spacer);

  GtkWidget *save = gtk_toggle_button_new();
  self->save_btn = GTK_TOGGLE_BUTTON(save);
  GtkWidget *save_icon = gtk_image_new();
  gtk_button_set_child(GTK_BUTTON(save), save_icon);
  gtk_widget_set_valign(save, GTK_ALIGN_CENTER);
  g_signal_connect(save, "toggled", G_CALLBACK(on_save_toggled), self);
  gtk_box_append(GTK_BOX(actions), save);

  gtk_box_append(GTK_BOX(box), actions);
}

GtkWidget *
feed_card_new(FeedItem *item, gboolean show_votes)
{
  g_return_val_if_fail(item != NULL, NULL);

  FeedCard *card = g_object_new(FEED_TYPE_CARD,
                                "orientation", GTK_ORIENTATION_VERTICAL,
                                "spacing", 6,
                                NULL);
  card->item = item;

  feed_card_build(card, show_votes);
  feed_card_set_vote(card, item->vote);
  feed_card_set_saved(card, item->saved);

  return GTK_WIDGET(card);
}

void
feed_card_set_vote(FeedCard *card, gint64 vote)
{
  g_return_if_fail(FEED_IS_CARD(card));

  if (card->up_btn == NULL)
    return;

  card->item->vote = vote;
  card->syncing = TRUE;
  gtk_toggle_button_set_active(card->up_btn, vote == 1);
  gtk_toggle_button_set_active(card->down_btn, vote == -1);
  gtk_widget_set_sensitive(GTK_WIDGET(card->clear_btn), vote != 0);
  card->syncing = FALSE;
}

void
feed_card_set_saved(FeedCard *card, gboolean saved)
{
  g_return_if_fail(FEED_IS_CARD(card));

  card->item->saved = saved;
  card->syncing = TRUE;
  gtk_toggle_button_set_active(card->save_btn, saved);
  card->syncing = FALSE;

  GtkWidget *child = gtk_button_get_child(GTK_BUTTON(card->save_btn));
  if (GTK_IS_IMAGE(child)) {
    gtk_image_set_from_icon_name(GTK_IMAGE(child),
                                 saved ? "starred-symbolic"
                                       : "non-starred-symbolic");
  }
  gtk_widget_set_tooltip_text(GTK_WIDGET(card->save_btn),
                              saved ? "Remove from saved" : "Save for later");
}

FeedItem *
feed_card_get_item(FeedCard *card)
{
  g_return_val_if_fail(FEED_IS_CARD(card), NULL);
  return card->item;
}

static void
feed_card_finalize(GObject *object)
{
  FeedCard *self = FEED_CARD(object);

  feed_item_free(self->item);
  self->item = NULL;

  G_OBJECT_CLASS(feed_card_parent_class)->finalize(object);
}

static void
feed_card_class_init(FeedCardClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->finalize = feed_card_finalize;

  card_signals[SIGNAL_VOTE] =
    g_signal_new("vote", FEED_TYPE_CARD, G_SIGNAL_RUN_LAST, 0,
                 NULL, NULL, NULL, G_TYPE_NONE, 1, G_TYPE_INT64);
  card_signals[SIGNAL_SAVE] =
    g_signal_new("save", FEED_TYPE_CARD, G_SIGNAL_RUN_LAST, 0,
                 NULL, NULL, NULL, G_TYPE_NONE, 1, G_TYPE_BOOLEAN);
  card_signals[SIGNAL_OPEN] =
    g_signal_new("open", FEED_TYPE_CARD, G_SIGNAL_RUN_LAST, 0,
                 NULL, NULL, NULL, G_TYPE_NONE, 0);
}

static void
feed_card_init(FeedCard *self)
{
  self->up_btn = NULL;
  self->down_btn = NULL;
  self->clear_btn = NULL;
  self->save_btn = NULL;
}

/* ---------- thumbnail loading ---------- */

static SoupSession *image_session = NULL;
static GHashTable *image_cache = NULL; /* url -> GBytes* */

typedef struct {
  GtkPicture *picture; /* owned ref */
  char *url;
} ImageLoadData;

static void
ensure_image_session(void)
{
  if (image_session != NULL)
    return;

  image_session = soup_session_new();
  g_object_set(image_session, "user-agent", "feed-gtk4/0.1", NULL);
  image_cache = g_hash_table_new_full(g_str_hash, g_str_equal, g_free,
                                      (GDestroyNotify) g_bytes_unref);
}

static void
on_image_loaded(GObject *source, GAsyncResult *result, gpointer data)
{
  ImageLoadData *load = data;
  GError *error = NULL;

  (void) source;

  GBytes *bytes = soup_session_send_and_read_finish(image_session, result, &error);
  if (bytes == NULL) {
    g_error_free(error);
  } else {
    GdkTexture *texture = gdk_texture_new_from_bytes(bytes, &error);
    if (texture != NULL) {
      gtk_picture_set_paintable(load->picture, GDK_PAINTABLE(texture));
      g_object_unref(texture);
      /* Cache the raw bytes so later rows reuse them. */
      g_hash_table_insert(image_cache, g_strdup(load->url), g_bytes_ref(bytes));
    } else {
      g_clear_error(&error);
    }
    g_bytes_unref(bytes);
  }

  g_object_unref(load->picture);
  g_free(load->url);
  g_free(load);
}

void
item_image_load_into(GtkPicture *picture, const char *url)
{
  g_return_if_fail(GTK_IS_PICTURE(picture));

  if (url == NULL || url[0] == '\0')
    return;

  ensure_image_session();

  GBytes *cached = g_hash_table_lookup(image_cache, url);
  if (cached != NULL) {
    GError *error = NULL;
    GdkTexture *texture = gdk_texture_new_from_bytes(cached, &error);
    if (texture != NULL) {
      gtk_picture_set_paintable(picture, GDK_PAINTABLE(texture));
      g_object_unref(texture);
    } else {
      g_clear_error(&error);
    }
    return;
  }

  SoupMessage *message = soup_message_new("GET", url);
  if (message == NULL)
    return;

  ImageLoadData *load = g_new0(ImageLoadData, 1);
  load->picture = g_object_ref(picture);
  load->url = g_strdup(url);

  soup_session_send_and_read_async(image_session, message, G_PRIORITY_DEFAULT,
                                   NULL, on_image_loaded, load);
  g_object_unref(message);
}
