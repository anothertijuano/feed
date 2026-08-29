#include "feed-view.h"

#include "api.h"
#include "item-common.h"

#define FEED_PAGE_SIZE 25

struct _FeedView {
  GtkBox parent_instance;
  FeedWindow *window; /* borrowed */
  GCancellable *cancellable;
  GtkListBox *list;
  GtkScrolledWindow *scroll;
  GtkWidget *status;
  GtkWidget *extras;
  gint64 offset;
  gint64 total;
  guint64 generation;
  gboolean loading;
  gboolean exhausted;
  gboolean loaded_once;
  gboolean disposed;
};

G_DEFINE_FINAL_TYPE(FeedView, feed_view, GTK_TYPE_BOX)

enum {
  PROP_WINDOW = 1,
  N_PROPS
};

static GParamSpec *props[N_PROPS];

/* ---------- forward declarations ---------- */

static void feed_view_load_next_page(FeedView *self);
static void feed_view_confirm_downvote(FeedView *self, FeedCard *card);
static void feed_view_send_vote(FeedView *self, FeedCard *card, gint64 value);
static void feed_view_reload_internal(FeedView *self);
static void feed_view_schedule_fill(FeedView *self);
static void on_card_vote(FeedCard *card, gint64 value, gpointer data);
static void on_card_save(FeedCard *card, gboolean saved, gpointer data);
static void on_card_open(FeedCard *card, gpointer data);

/* ---------- helpers ---------- */

static gboolean
feed_view_is_empty(FeedView *self)
{
  return gtk_list_box_get_row_at_index(self->list, 0) == NULL;
}

static void
feed_view_set_status(FeedView *self, const char *text)
{
  if (text == NULL || text[0] == '\0') {
    gtk_widget_set_visible(self->status, FALSE);
    return;
  }
  gtk_label_set_text(GTK_LABEL(self->status), text);
  gtk_widget_set_visible(self->status, TRUE);
}

static FeedApi *
feed_view_get_api(FeedView *self)
{
  return self->window != NULL ? feed_window_get_api(self->window) : NULL;
}

/* ---------- closures ---------- */

typedef struct {
  FeedView *view; /* owned ref */
  guint64 generation;
} ViewClosure;

static ViewClosure *
view_closure_new(FeedView *view)
{
  ViewClosure *closure = g_new0(ViewClosure, 1);
  closure->view = g_object_ref(view);
  closure->generation = view->generation;
  return closure;
}

static void
view_closure_free(gpointer data)
{
  ViewClosure *closure = data;

  g_object_unref(closure->view);
  g_free(closure);
}

/* ---------- paging ---------- */

static void
on_feed_loaded(FeedApiResponse *response, gpointer data)
{
  ViewClosure *closure = data;
  FeedView *self = closure->view;
  gboolean current =
    !self->disposed && self->generation == closure->generation;

  if (!self->disposed)
    feed_window_busy_pop(self->window);
  if (!current)
    return;

  self->loading = FALSE;

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
    if (feed_view_is_empty(self))
      feed_view_set_status(self, "Could not load the feed.");
    return;
  }

  gint64 total = -1;
  GPtrArray *items = feed_items_from_json(response->root, &total);
  if (items == NULL) {
    feed_window_show_toast(self->window, "The server sent an unexpected response.");
    return;
  }

  if (total >= 0)
    self->total = total;

  FeedApi *api = feed_view_get_api(self);
  for (guint i = 0; i < items->len; i++) {
    FeedItem *item = g_ptr_array_steal_index(items, i);
    GtkWidget *card = feed_card_new(item, TRUE);

    g_signal_connect(card, "vote", G_CALLBACK(on_card_vote), self);
    g_signal_connect(card, "save", G_CALLBACK(on_card_save), self);
    g_signal_connect(card, "open", G_CALLBACK(on_card_open), self);

    gtk_list_box_append(self->list, card);

    /* First render: mark the item as seen, fire-and-forget. */
    feed_api_seen(api, item->id, NULL, NULL, NULL, NULL);
  }
  guint count = items->len;
  g_ptr_array_unref(items);

  self->offset += (gint64) count;
  if (self->total >= 0 && self->offset >= self->total)
    self->exhausted = TRUE;

  if (count > 0) {
    feed_view_set_status(self, NULL);
  } else if (feed_view_is_empty(self)) {
    feed_view_set_status(self,
      "Your feed is empty. Add a subscription on the server, then refresh.");
  }

  self->loaded_once = TRUE;
  feed_view_schedule_fill(self);
}

static void
feed_view_load_next_page(FeedView *self)
{
  if (self->disposed || self->loading || self->exhausted)
    return;

  FeedApi *api = feed_view_get_api(self);
  if (api == NULL || feed_api_get_server(api) == NULL) {
    feed_view_set_status(self, "No server configured — open Settings.");
    return;
  }

  self->loading = TRUE;
  if (self->offset == 0)
    feed_view_set_status(self, "Loading…");

  feed_window_busy_push(self->window);

  ViewClosure *closure = view_closure_new(self);
  feed_api_get_feed(api, FEED_PAGE_SIZE, self->offset, self->cancellable,
                    on_feed_loaded, closure, view_closure_free);
}

static void
feed_view_maybe_fill(FeedView *self)
{
  if (self->disposed || self->loading || self->exhausted)
    return;
  if (!gtk_widget_get_mapped(GTK_WIDGET(self)))
    return;

  GtkAdjustment *vadj = gtk_scrolled_window_get_vadjustment(self->scroll);
  if (gtk_adjustment_get_upper(vadj) <=
      gtk_adjustment_get_page_size(vadj) + 1.0) {
    feed_view_load_next_page(self);
  }
}

static gboolean
fill_idle(gpointer data)
{
  FeedView *self = FEED_VIEW(data);

  feed_view_maybe_fill(self);
  g_object_unref(self);
  return G_SOURCE_REMOVE;
}

static void
feed_view_schedule_fill(FeedView *self)
{
  if (self->disposed)
    return;
  g_idle_add(fill_idle, g_object_ref(self));
}

static void
on_edge_reached(GtkScrolledWindow *scroll, GtkPositionType position,
                gpointer data)
{
  FeedView *self = FEED_VIEW(data);

  (void) scroll;

  if (position == GTK_POS_BOTTOM)
    feed_view_load_next_page(self);
}

static void
on_vadjustment_changed(GtkAdjustment *adjustment, gpointer data)
{
  FeedView *self = FEED_VIEW(data);
  double value = gtk_adjustment_get_value(adjustment);
  double upper = gtk_adjustment_get_upper(adjustment);
  double page_size = gtk_adjustment_get_page_size(adjustment);

  if (upper - (value + page_size) < 300.0)
    feed_view_load_next_page(self);
}

/* ---------- refresh ---------- */

static void
on_refresh_done(FeedApiResponse *response, gpointer data)
{
  ViewClosure *closure = data;
  FeedView *self = closure->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
    return;
  }

  gint64 count = -1;
  if (response->root != NULL && JSON_NODE_HOLDS_OBJECT(response->root)) {
    JsonObject *object = json_node_get_object(response->root);
    if (json_object_has_member(object, "new"))
      count = json_object_get_int_member(object, "new");
  }

  if (count > 0) {
    g_autofree char *message =
      g_strdup_printf("%" G_GINT64_FORMAT " new item%s fetched",
                      count, count == 1 ? "" : "s");
    feed_window_show_toast(self->window, message);
  } else {
    feed_window_show_toast(self->window, "Feed refreshed");
  }

  feed_view_reload_internal(self);
}

static void
on_refresh_clicked(GtkButton *button, gpointer data)
{
  FeedView *self = FEED_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  feed_window_busy_push(self->window);
  ViewClosure *closure = view_closure_new(self);
  feed_api_refresh_all(feed_view_get_api(self), self->cancellable,
                       on_refresh_done, closure, view_closure_free);
}

/* ---------- voting ---------- */

typedef struct {
  FeedView *view; /* owned ref */
  FeedCard *card; /* owned ref */
} VoteClosure;

static void
vote_closure_free(gpointer data)
{
  VoteClosure *closure = data;

  g_object_unref(closure->view);
  g_object_unref(closure->card);
  g_free(closure);
}

static void
on_vote_done(FeedApiResponse *response, gpointer data)
{
  VoteClosure *closure = data;
  FeedView *self = closure->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
    feed_card_set_vote(closure->card,
                       feed_card_get_item(closure->card)->vote);
    return;
  }

  gint64 vote = feed_card_get_item(closure->card)->vote;
  if (response->root != NULL && JSON_NODE_HOLDS_OBJECT(response->root)) {
    JsonObject *object = json_node_get_object(response->root);
    if (json_object_has_member(object, "vote"))
      vote = json_object_get_int_member(object, "vote");
  }

  feed_card_set_vote(closure->card, vote);

  if (vote == -1) {
    /* The server removed the item permanently; drop the row. */
    gtk_list_box_remove(self->list, GTK_WIDGET(closure->card));
    if (self->offset > 0)
      self->offset--;
    if (self->total > 0)
      self->total--;
    if (feed_view_is_empty(self))
      feed_view_set_status(self, "Your feed is empty.");
  }
}

static void
feed_view_send_vote(FeedView *self, FeedCard *card, gint64 value)
{
  feed_window_busy_push(self->window);

  VoteClosure *closure = g_new0(VoteClosure, 1);
  closure->view = g_object_ref(self);
  closure->card = g_object_ref(card);

  feed_api_vote(feed_view_get_api(self), feed_card_get_item(card)->id, value,
                self->cancellable, on_vote_done, closure, vote_closure_free);
}

static void
on_card_vote(FeedCard *card, gint64 value, gpointer data)
{
  FeedView *self = FEED_VIEW(data);

  if (self->disposed)
    return;

  if (value == -1)
    feed_view_confirm_downvote(self, card);
  else
    feed_view_send_vote(self, card, value);
}

static void
on_downvote_chosen(AdwMessageDialog *dialog, GAsyncResult *result,
                   gpointer data)
{
  VoteClosure *confirm = data;
  const char *response = adw_message_dialog_choose_finish(dialog, result);
  gboolean remove = g_strcmp0(response, "remove") == 0;

  g_object_unref(dialog);

  if (!confirm->view->disposed) {
    if (remove) {
      feed_view_send_vote(confirm->view, confirm->card, -1);
    } else {
      /* Revert the optimistic toggle. */
      feed_card_set_vote(confirm->card,
                         feed_card_get_item(confirm->card)->vote);
    }
  }

  vote_closure_free(confirm);
}

static void
feed_view_confirm_downvote(FeedView *self, FeedCard *card)
{
  AdwMessageDialog *dialog =
    adw_message_dialog_new(GTK_WIDGET(self->window),
                           "Remove this item from the feed?",
                           "Downvoting deletes the item from the server "
                           "permanently. This cannot be undone.");

  adw_message_dialog_add_responses(dialog,
                                   "cancel", "_Cancel",
                                   "remove", "_Remove",
                                   NULL);
  adw_message_dialog_set_response_appearance(dialog, "remove",
                                             ADW_RESPONSE_DESTRUCTIVE);
  adw_message_dialog_set_default_response(dialog, "cancel");
  adw_message_dialog_set_close_response(dialog, "cancel");

  VoteClosure *confirm = g_new0(VoteClosure, 1);
  confirm->view = g_object_ref(self);
  confirm->card = g_object_ref(card);

  adw_message_dialog_choose(dialog, NULL, on_downvote_chosen, confirm);
}

/* ---------- saving ---------- */

typedef struct {
  FeedView *view; /* owned ref */
  FeedCard *card; /* owned ref */
} SaveClosure;

static void
save_closure_free(gpointer data)
{
  SaveClosure *closure = data;

  g_object_unref(closure->view);
  g_object_unref(closure->card);
  g_free(closure);
}

static void
on_save_done(FeedApiResponse *response, gpointer data)
{
  SaveClosure *closure = data;
  FeedView *self = closure->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
    feed_card_set_saved(closure->card,
                        feed_card_get_item(closure->card)->saved);
    return;
  }

  gboolean saved = feed_card_get_item(closure->card)->saved;
  if (response->root != NULL && JSON_NODE_HOLDS_OBJECT(response->root)) {
    JsonObject *object = json_node_get_object(response->root);
    if (json_object_has_member(object, "saved"))
      saved = json_object_get_boolean_member(object, "saved");
  }

  feed_card_set_saved(closure->card, saved);
}

static void
on_card_save(FeedCard *card, gboolean saved, gpointer data)
{
  FeedView *self = FEED_VIEW(data);

  if (self->disposed)
    return;

  feed_window_busy_push(self->window);

  SaveClosure *closure = g_new0(SaveClosure, 1);
  closure->view = g_object_ref(self);
  closure->card = g_object_ref(card);

  feed_api_save(feed_view_get_api(self), feed_card_get_item(card)->id, saved,
                self->cancellable, on_save_done, closure, save_closure_free);
}

static void
on_card_open(FeedCard *card, gpointer data)
{
  FeedView *self = FEED_VIEW(data);
  FeedItem *item = feed_card_get_item(card);

  if (self->disposed)
    return;

  if (item->link != NULL && item->link[0] != '\0')
    gtk_show_uri(GTK_WINDOW(self->window), item->link, GDK_CURRENT_TIME);
}

/* ---------- GObject boilerplate ---------- */

static void
feed_view_constructed(GObject *object)
{
  FeedView *self = FEED_VIEW(object);

  G_OBJECT_CLASS(feed_view_parent_class)->constructed(object);

  self->status = gtk_label_new("Loading…");
  gtk_label_set_wrap(GTK_LABEL(self->status), TRUE);
  gtk_label_set_xalign(GTK_LABEL(self->status), 0.0f);
  gtk_widget_add_css_class(self->status, "dim-label");
  gtk_widget_set_margin_top(self->status, 24);
  gtk_widget_set_margin_start(self->status, 12);
  gtk_widget_set_margin_end(self->status, 12);
  gtk_box_append(GTK_BOX(self), self->status);

  self->scroll = GTK_SCROLLED_WINDOW(gtk_scrolled_window_new());
  gtk_widget_set_hexpand(GTK_WIDGET(self->scroll), TRUE);
  gtk_widget_set_vexpand(GTK_WIDGET(self->scroll), TRUE);
  gtk_scrolled_window_set_policy(self->scroll, GTK_POLICY_NEVER,
                                 GTK_POLICY_AUTOMATIC);

  self->list = GTK_LIST_BOX(gtk_list_box_new());
  gtk_list_box_set_selection_mode(self->list, GTK_SELECTION_NONE);
  gtk_list_box_set_show_separators(self->list, FALSE);
  gtk_scrolled_window_set_child(self->scroll, GTK_WIDGET(self->list));
  gtk_box_append(GTK_BOX(self), GTK_WIDGET(self->scroll));

  g_signal_connect(self->scroll, "edge-reached",
                   G_CALLBACK(on_edge_reached), self);
  GtkAdjustment *vadjustment =
    gtk_scrolled_window_get_vadjustment(self->scroll);
  g_signal_connect(vadjustment, "value-changed",
                   G_CALLBACK(on_vadjustment_changed), self);

  /* Header bar extras: refresh button. */
  self->extras = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
  GtkWidget *refresh = gtk_button_new_from_icon_name("view-refresh-symbolic");
  gtk_widget_set_tooltip_text(refresh, "Fetch new items and refresh the feed");
  g_signal_connect(refresh, "clicked", G_CALLBACK(on_refresh_clicked), self);
  gtk_box_append(GTK_BOX(self->extras), refresh);
}

static void
feed_view_dispose(GObject *object)
{
  FeedView *self = FEED_VIEW(object);

  self->disposed = TRUE;
  if (self->cancellable != NULL) {
    g_cancellable_cancel(self->cancellable);
    g_clear_object(&self->cancellable);
  }

  G_OBJECT_CLASS(feed_view_parent_class)->dispose(object);
}

static void
feed_view_get_property(GObject *object, guint prop_id, GValue *value,
                       GParamSpec *pspec)
{
  FeedView *self = FEED_VIEW(object);

  switch (prop_id) {
  case PROP_WINDOW:
    g_value_set_object(value, self->window);
    break;
  default:
    G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    break;
  }
}

static void
feed_view_set_property(GObject *object, guint prop_id, const GValue *value,
                       GParamSpec *pspec)
{
  FeedView *self = FEED_VIEW(object);

  switch (prop_id) {
  case PROP_WINDOW:
    self->window = g_value_get_object(value); /* borrowed */
    break;
  default:
    G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    break;
  }
}

static void
feed_view_class_init(FeedViewClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->constructed = feed_view_constructed;
  gobject_class->dispose = feed_view_dispose;
  gobject_class->get_property = feed_view_get_property;
  gobject_class->set_property = feed_view_set_property;

  props[PROP_WINDOW] =
    g_param_spec_object("window", NULL, NULL, FEED_TYPE_WINDOW,
                        G_PARAM_READWRITE | G_PARAM_CONSTRUCT_ONLY |
                          G_PARAM_STATIC_STRINGS);

  g_object_class_install_properties(gobject_class, N_PROPS, props);
}

static void
feed_view_init(FeedView *self)
{
  self->cancellable = g_cancellable_new();
  self->total = -1;
}

/* ---------- public API ---------- */

GtkWidget *
feed_view_new(FeedWindow *window)
{
  g_return_val_if_fail(FEED_IS_WINDOW(window), NULL);

  return g_object_new(FEED_TYPE_VIEW,
                      "orientation", GTK_ORIENTATION_VERTICAL,
                      "spacing", 0,
                      "window", window,
                      NULL);
}

GtkWidget *
feed_view_get_extras(FeedView *self)
{
  g_return_val_if_fail(FEED_IS_VIEW(self), NULL);
  return self->extras;
}

void
feed_view_on_show(FeedView *self, FeedWindow *window)
{
  g_return_if_fail(FEED_IS_VIEW(self));

  (void) window;

  if (self->disposed)
    return;

  if (!self->loaded_once)
    feed_view_load_next_page(self);
  else
    feed_view_schedule_fill(self);
}

static void
feed_view_reload_internal(FeedView *self)
{
  if (self->disposed)
    return;

  self->generation++;
  if (self->cancellable != NULL)
    g_cancellable_cancel(self->cancellable);
  g_clear_object(&self->cancellable);
  self->cancellable = g_cancellable_new();

  gtk_list_box_remove_all(self->list);
  self->offset = 0;
  self->total = -1;
  self->exhausted = FALSE;
  self->loading = FALSE;

  feed_view_load_next_page(self);
}

void
feed_view_reload(FeedView *self)
{
  g_return_if_fail(FEED_IS_VIEW(self));

  feed_view_reload_internal(self);
}
