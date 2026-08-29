#include "saved-view.h"

#include "api.h"
#include "item-common.h"

struct _SavedView {
  GtkBox parent_instance;
  FeedWindow *window; /* borrowed */
  GCancellable *cancellable;
  GtkListBox *list;
  GtkWidget *status;
  GtkWidget *extras;
  guint64 generation;
  gboolean loading;
  gboolean loaded_once;
  gboolean disposed;
};

G_DEFINE_FINAL_TYPE(SavedView, saved_view, GTK_TYPE_BOX)

enum {
  PROP_WINDOW = 1,
  N_PROPS
};

static GParamSpec *props[N_PROPS];

/* ---------- forward declarations ---------- */

static void saved_view_load(SavedView *self);
static void saved_view_reload_internal(SavedView *self);
static void on_card_save(FeedCard *card, gboolean saved, gpointer data);
static void on_card_open(FeedCard *card, gpointer data);

/* ---------- helpers ---------- */

static gboolean
saved_view_is_empty(SavedView *self)
{
  return gtk_list_box_get_row_at_index(self->list, 0) == NULL;
}

static void
saved_view_set_status(SavedView *self, const char *text)
{
  if (text == NULL || text[0] == '\0') {
    gtk_widget_set_visible(self->status, FALSE);
    return;
  }
  gtk_label_set_text(GTK_LABEL(self->status), text);
  gtk_widget_set_visible(self->status, TRUE);
}

static FeedApi *
saved_view_get_api(SavedView *self)
{
  return self->window != NULL ? feed_window_get_api(self->window) : NULL;
}

/* ---------- closures ---------- */

typedef struct {
  SavedView *view; /* owned ref */
  guint64 generation;
} ViewClosure;

static ViewClosure *
view_closure_new(SavedView *view)
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

/* ---------- loading ---------- */

static void
on_saved_loaded(FeedApiResponse *response, gpointer data)
{
  ViewClosure *closure = data;
  SavedView *self = closure->view;
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
    if (saved_view_is_empty(self))
      saved_view_set_status(self, "Could not load saved items.");
    return;
  }

  GPtrArray *items = feed_items_from_json(response->root, NULL);
  if (items == NULL) {
    feed_window_show_toast(self->window, "The server sent an unexpected response.");
    return;
  }

  gtk_list_box_remove_all(self->list);

  for (guint i = 0; i < items->len; i++) {
    FeedItem *item = g_ptr_array_steal_index(items, i);
    GtkWidget *card = feed_card_new(item, FALSE);

    g_signal_connect(card, "save", G_CALLBACK(on_card_save), self);
    g_signal_connect(card, "open", G_CALLBACK(on_card_open), self);

    gtk_list_box_append(self->list, card);
  }
  g_ptr_array_unref(items);

  if (saved_view_is_empty(self)) {
    saved_view_set_status(self,
      "No saved items yet. Star items in the feed to keep them here.");
  } else {
    saved_view_set_status(self, NULL);
  }

  self->loaded_once = TRUE;
}

static void
saved_view_load(SavedView *self)
{
  if (self->disposed || self->loading)
    return;

  FeedApi *api = saved_view_get_api(self);
  if (api == NULL || feed_api_get_server(api) == NULL) {
    saved_view_set_status(self, "No server configured — open Settings.");
    return;
  }

  self->loading = TRUE;
  saved_view_set_status(self, "Loading…");
  feed_window_busy_push(self->window);

  ViewClosure *closure = view_closure_new(self);
  feed_api_get_saved(api, self->cancellable,
                     on_saved_loaded, closure, view_closure_free);
}

/* ---------- refresh ---------- */

static void
on_refresh_clicked(GtkButton *button, gpointer data)
{
  SavedView *self = FEED_SAVED_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  saved_view_reload_internal(self);
}

/* ---------- interactions ---------- */

typedef struct {
  SavedView *view; /* owned ref */
  FeedCard *card;  /* owned ref */
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
  SavedView *self = closure->view;

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

  if (!saved) {
    /* Un-saved: drop the row. */
    gtk_list_box_remove(self->list, GTK_WIDGET(closure->card));
    if (saved_view_is_empty(self)) {
      saved_view_set_status(self,
        "No saved items yet. Star items in the feed to keep them here.");
    }
  } else {
    feed_card_set_saved(closure->card, TRUE);
  }
}

static void
on_card_save(FeedCard *card, gboolean saved, gpointer data)
{
  SavedView *self = FEED_SAVED_VIEW(data);

  if (self->disposed)
    return;

  feed_window_busy_push(self->window);

  SaveClosure *closure = g_new0(SaveClosure, 1);
  closure->view = g_object_ref(self);
  closure->card = g_object_ref(card);

  feed_api_save(saved_view_get_api(self), feed_card_get_item(card)->id, saved,
                self->cancellable, on_save_done, closure, save_closure_free);
}

static void
on_card_open(FeedCard *card, gpointer data)
{
  SavedView *self = FEED_SAVED_VIEW(data);
  FeedItem *item = feed_card_get_item(card);

  if (self->disposed)
    return;

  if (item->link != NULL && item->link[0] != '\0')
    gtk_show_uri(GTK_WINDOW(self->window), item->link, GDK_CURRENT_TIME);
}

/* ---------- GObject boilerplate ---------- */

static void
saved_view_constructed(GObject *object)
{
  SavedView *self = FEED_SAVED_VIEW(object);

  G_OBJECT_CLASS(saved_view_parent_class)->constructed(object);

  self->status = gtk_label_new("Loading…");
  gtk_label_set_wrap(GTK_LABEL(self->status), TRUE);
  gtk_label_set_xalign(GTK_LABEL(self->status), 0.0f);
  gtk_widget_add_css_class(self->status, "dim-label");
  gtk_widget_set_margin_top(self->status, 24);
  gtk_widget_set_margin_start(self->status, 12);
  gtk_widget_set_margin_end(self->status, 12);
  gtk_box_append(GTK_BOX(self), self->status);

  GtkWidget *scroll = gtk_scrolled_window_new();
  gtk_widget_set_hexpand(scroll, TRUE);
  gtk_widget_set_vexpand(scroll, TRUE);
  gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scroll), GTK_POLICY_NEVER,
                                 GTK_POLICY_AUTOMATIC);

  self->list = GTK_LIST_BOX(gtk_list_box_new());
  gtk_list_box_set_selection_mode(self->list, GTK_SELECTION_NONE);
  gtk_list_box_set_show_separators(self->list, FALSE);
  gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(scroll),
                                GTK_WIDGET(self->list));
  gtk_box_append(GTK_BOX(self), scroll);

  /* Header bar extras: refresh button. */
  self->extras = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
  GtkWidget *refresh = gtk_button_new_from_icon_name("view-refresh-symbolic");
  gtk_widget_set_tooltip_text(refresh, "Refresh saved items");
  g_signal_connect(refresh, "clicked", G_CALLBACK(on_refresh_clicked), self);
  gtk_box_append(GTK_BOX(self->extras), refresh);
}

static void
saved_view_dispose(GObject *object)
{
  SavedView *self = FEED_SAVED_VIEW(object);

  self->disposed = TRUE;
  if (self->cancellable != NULL) {
    g_cancellable_cancel(self->cancellable);
    g_clear_object(&self->cancellable);
  }

  G_OBJECT_CLASS(saved_view_parent_class)->dispose(object);
}

static void
saved_view_get_property(GObject *object, guint prop_id, GValue *value,
                        GParamSpec *pspec)
{
  SavedView *self = FEED_SAVED_VIEW(object);

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
saved_view_set_property(GObject *object, guint prop_id, const GValue *value,
                        GParamSpec *pspec)
{
  SavedView *self = FEED_SAVED_VIEW(object);

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
saved_view_class_init(SavedViewClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->constructed = saved_view_constructed;
  gobject_class->dispose = saved_view_dispose;
  gobject_class->get_property = saved_view_get_property;
  gobject_class->set_property = saved_view_set_property;

  props[PROP_WINDOW] =
    g_param_spec_object("window", NULL, NULL, FEED_TYPE_WINDOW,
                        G_PARAM_READWRITE | G_PARAM_CONSTRUCT_ONLY |
                          G_PARAM_STATIC_STRINGS);

  g_object_class_install_properties(gobject_class, N_PROPS, props);
}

static void
saved_view_init(SavedView *self)
{
  self->cancellable = g_cancellable_new();
}

/* ---------- public API ---------- */

GtkWidget *
saved_view_new(FeedWindow *window)
{
  g_return_val_if_fail(FEED_IS_WINDOW(window), NULL);

  return g_object_new(FEED_TYPE_SAVED_VIEW,
                      "orientation", GTK_ORIENTATION_VERTICAL,
                      "spacing", 0,
                      "window", window,
                      NULL);
}

GtkWidget *
saved_view_get_extras(SavedView *self)
{
  g_return_val_if_fail(FEED_IS_SAVED_VIEW(self), NULL);
  return self->extras;
}

void
saved_view_on_show(SavedView *self, FeedWindow *window)
{
  g_return_if_fail(FEED_IS_SAVED_VIEW(self));

  (void) window;

  if (self->disposed)
    return;

  if (!self->loaded_once)
    saved_view_load(self);
}

static void
saved_view_reload_internal(SavedView *self)
{
  if (self->disposed)
    return;

  self->generation++;
  if (self->cancellable != NULL)
    g_cancellable_cancel(self->cancellable);
  g_clear_object(&self->cancellable);
  self->cancellable = g_cancellable_new();

  gtk_list_box_remove_all(self->list);
  self->loading = FALSE;

  saved_view_load(self);
}

void
saved_view_reload(SavedView *self)
{
  g_return_if_fail(FEED_IS_SAVED_VIEW(self));

  saved_view_reload_internal(self);
}
