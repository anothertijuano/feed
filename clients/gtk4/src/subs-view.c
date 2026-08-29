#include "subs-view.h"

#include "api.h"

typedef struct {
  char *id;
  char *url;
  char *title;
  char *notify;
  gint64 item_count;
} SubRow;

static void
sub_row_free(gpointer data)
{
  SubRow *row = data;

  if (row == NULL)
    return;
  g_free(row->id);
  g_free(row->url);
  g_free(row->title);
  g_free(row->notify);
  g_free(row);
}

struct _SubsView {
  GtkBox parent_instance;
  FeedWindow *window; /* borrowed */
  GCancellable *cancellable;
  GtkListBox *list;
  GtkWidget *status;
  GtkWidget *extras;
  AdwDialog *add_dialog;
  GtkEntry *add_entry;
  guint64 generation;
  gboolean loading;
  gboolean loaded_once;
  gboolean disposed;
};

G_DEFINE_FINAL_TYPE(SubsView, subs_view, GTK_TYPE_BOX)

enum {
  PROP_WINDOW = 1,
  N_PROPS
};

static GParamSpec *props[N_PROPS];

/* ---------- forward declarations ---------- */

static void subs_view_load(SubsView *self);
static void subs_view_reload_internal(SubsView *self);
static void subs_view_add(SubsView *self, const char *url);
static void on_policy_done(FeedApiResponse *response, gpointer data);
static void on_delete_done(FeedApiResponse *response, gpointer data);
static void on_delete_chosen(GObject *source_object, GAsyncResult *result,
                             gpointer data);

/* ---------- helpers ---------- */

static gboolean
subs_view_is_empty(SubsView *self)
{
  return gtk_list_box_get_row_at_index(self->list, 0) == NULL;
}

static void
subs_view_set_status(SubsView *self, const char *text)
{
  if (text == NULL || text[0] == '\0') {
    gtk_widget_set_visible(self->status, FALSE);
    return;
  }
  gtk_label_set_text(GTK_LABEL(self->status), text);
  gtk_widget_set_visible(self->status, TRUE);
}

static FeedApi *
subs_view_get_api(SubsView *self)
{
  return self->window != NULL ? feed_window_get_api(self->window) : NULL;
}

static const char *
next_policy(const char *current)
{
  if (g_strcmp0(current, "default") == 0)
    return "always";
  if (g_strcmp0(current, "always") == 0)
    return "never";
  return "default";
}

static SubRow *
sub_row_from_widget(GtkWidget *widget)
{
  GtkWidget *row = gtk_widget_get_ancestor(widget, GTK_TYPE_LIST_BOX_ROW);
  if (row == NULL)
    return NULL;
  return g_object_get_data(G_OBJECT(row), "sub");
}

/* ---------- closures ---------- */

typedef struct {
  SubsView *view; /* owned ref */
  guint64 generation;
} ViewClosure;

static ViewClosure *
view_closure_new(SubsView *view)
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

typedef struct {
  SubsView *view;    /* owned ref */
  GtkWidget *badge;  /* owned ref */
  char *policy;
} PolicyClosure;

static void
policy_closure_free(gpointer data)
{
  PolicyClosure *closure = data;

  g_object_unref(closure->view);
  g_object_unref(closure->badge);
  g_free(closure->policy);
  g_free(closure);
}

typedef struct {
  SubsView *view;    /* owned ref */
  GtkWidget *row;    /* owned ref */
  char *id;
} DeleteConfirm;

static void
delete_confirm_free(gpointer data)
{
  DeleteConfirm *confirm = data;

  g_object_unref(confirm->view);
  g_object_unref(confirm->row);
  g_free(confirm->id);
  g_free(confirm);
}

/* ---------- list building ---------- */

static void
on_badge_clicked(GtkButton *button, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);
  SubRow *row = sub_row_from_widget(GTK_WIDGET(button));

  if (self->disposed || row == NULL)
    return;

  const char *policy = next_policy(row->notify);

  feed_window_busy_push(self->window);

  PolicyClosure *closure = g_new0(PolicyClosure, 1);
  closure->view = g_object_ref(self);
  closure->badge = g_object_ref(button);
  closure->policy = g_strdup(policy);

  feed_api_set_subscription_notify(subs_view_get_api(self), row->id, policy,
                                   self->cancellable, on_policy_done,
                                   closure, policy_closure_free);
}

static void
on_delete_clicked(GtkButton *button, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);
  GtkWidget *row_widget = gtk_widget_get_ancestor(GTK_WIDGET(button),
                                                  GTK_TYPE_LIST_BOX_ROW);
  SubRow *row = sub_row_from_widget(GTK_WIDGET(button));

  if (self->disposed || row_widget == NULL || row == NULL)
    return;

  AdwMessageDialog *dialog =
    adw_message_dialog_new(GTK_WINDOW(self->window),
                           "Remove subscription?",
                           "The subscription will be removed from the server. "
                           "Already-fetched items stay in your feed.");

  adw_message_dialog_add_responses(dialog,
                                   "cancel", "_Cancel",
                                   "remove", "_Remove",
                                   NULL);
  adw_message_dialog_set_response_appearance(dialog, "remove",
                                             ADW_RESPONSE_DESTRUCTIVE);
  adw_message_dialog_set_default_response(dialog, "cancel");
  adw_message_dialog_set_close_response(dialog, "cancel");

  DeleteConfirm *confirm = g_new0(DeleteConfirm, 1);
  confirm->view = g_object_ref(self);
  confirm->row = g_object_ref(row_widget);
  confirm->id = g_strdup(row->id);

  adw_message_dialog_choose(dialog, NULL, on_delete_chosen, confirm);
}

static void
subs_view_rebuild(SubsView *self, JsonNode *root)
{
  gtk_list_box_remove_all(self->list);

  if (root == NULL || !JSON_NODE_HOLDS_OBJECT(root)) {
    subs_view_set_status(self, "Could not load subscriptions.");
    return;
  }

  JsonArray *array =
    json_object_get_array_member(json_node_get_object(root), "items");
  if (array == NULL) {
    subs_view_set_status(self, "Could not load subscriptions.");
    return;
  }

  guint length = json_array_get_length(array);
  for (guint i = 0; i < length; i++) {
    JsonObject *object = json_array_get_object_element(array, i);
    if (object == NULL)
      continue;

    SubRow *row = g_new0(SubRow, 1);
    row->id = g_strdup(json_object_get_string_member(object, "id"));
    row->url = g_strdup(json_object_get_string_member(object, "url"));
    row->title = g_strdup(json_object_get_string_member(object, "title"));
    row->notify = g_strdup(json_object_get_string_member(object, "notify"));
    row->item_count = json_object_get_int_member(object, "itemCount");
    if (row->notify == NULL || row->notify[0] == '\0') {
      g_free(row->notify);
      row->notify = g_strdup("default");
    }

    const char *display_title =
      row->title != NULL && row->title[0] != '\0'
        ? row->title : (row->url != NULL ? row->url : "Unknown subscription");

    GtkWidget *row_widget = adw_action_row_new();
    g_object_set_data_full(G_OBJECT(row_widget), "sub", row, sub_row_free);
    adw_preferences_row_set_title(ADW_PREFERENCES_ROW(row_widget), display_title);
    if (row->url != NULL && row->url[0] != '\0')
      adw_action_row_set_subtitle(ADW_ACTION_ROW(row_widget), row->url);
    adw_action_row_set_icon_name(ADW_ACTION_ROW(row_widget),
                                 "feed-subscribe-symbolic");

    GtkWidget *suffix = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);

    GtkWidget *badge = gtk_button_new_with_label(row->notify);
    gtk_widget_add_css_class(badge, "pill");
    gtk_widget_set_tooltip_text(badge,
      "Notification policy — click to cycle default / always / never");
    g_signal_connect(badge, "clicked", G_CALLBACK(on_badge_clicked), self);
    gtk_box_append(GTK_BOX(suffix), badge);

    GtkWidget *remove = gtk_button_new_from_icon_name("user-trash-symbolic");
    gtk_widget_add_css_class(remove, "flat");
    gtk_widget_set_tooltip_text(remove, "Remove subscription");
    g_signal_connect(remove, "clicked", G_CALLBACK(on_delete_clicked), self);
    gtk_box_append(GTK_BOX(suffix), remove);

    adw_action_row_add_suffix(ADW_ACTION_ROW(row_widget), suffix);
    gtk_list_box_append(self->list, row_widget);
  }

  if (subs_view_is_empty(self)) {
    subs_view_set_status(self,
      "No subscriptions yet. Click + to add a feed URL.");
  } else {
    subs_view_set_status(self, NULL);
  }
}

/* ---------- loading ---------- */

static void
on_subs_loaded(FeedApiResponse *response, gpointer data)
{
  ViewClosure *closure = data;
  SubsView *self = closure->view;
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
    if (subs_view_is_empty(self))
      subs_view_set_status(self, "Could not load subscriptions.");
    return;
  }

  subs_view_rebuild(self, response->root);
  self->loaded_once = TRUE;
}

static void
subs_view_load(SubsView *self)
{
  if (self->disposed || self->loading)
    return;

  FeedApi *api = subs_view_get_api(self);
  if (api == NULL || feed_api_get_server(api) == NULL) {
    subs_view_set_status(self, "No server configured — open Settings.");
    return;
  }

  self->loading = TRUE;
  subs_view_set_status(self, "Loading…");
  feed_window_busy_push(self->window);

  ViewClosure *closure = view_closure_new(self);
  feed_api_get_subscriptions(api, self->cancellable,
                             on_subs_loaded, closure, view_closure_free);
}

/* ---------- add subscription ---------- */

static void
on_add_done(FeedApiResponse *response, gpointer data)
{
  ViewClosure *closure = data;
  SubsView *self = closure->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
    return;
  }

  feed_window_show_toast(self->window, "Subscription added");
  subs_view_reload_internal(self);
}

static void
subs_view_add(SubsView *self, const char *url)
{
  if (self->disposed)
    return;

  feed_window_busy_push(self->window);

  ViewClosure *closure = view_closure_new(self);
  feed_api_add_subscription(subs_view_get_api(self), url, self->cancellable,
                            on_add_done, closure, view_closure_free);
}

static void
subs_view_submit_entry(SubsView *self)
{
  g_autofree char *url = g_strdup(gtk_editable_get_text(GTK_EDITABLE(self->add_entry)));
  g_strstrip(url);

  if (url[0] == '\0') {
    feed_window_show_toast(self->window, "Enter a feed URL first.");
    return;
  }

  adw_dialog_force_close(self->add_dialog);
  subs_view_add(self, url);
}

static void
on_add_confirm_clicked(GtkButton *button, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  subs_view_submit_entry(self);
}

static void
on_add_entry_activate(GtkEntry *entry, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);

  (void) entry;

  if (self->disposed)
    return;

  subs_view_submit_entry(self);
}

static void
on_add_cancel_clicked(GtkButton *button, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  adw_dialog_force_close(self->add_dialog);
}

static void
on_add_clicked(GtkButton *button, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  gtk_editable_set_text(GTK_EDITABLE(self->add_entry), "");
  adw_dialog_present(self->add_dialog, GTK_WIDGET(self));
  gtk_widget_grab_focus(GTK_WIDGET(self->add_entry));
}

/* ---------- refresh all ---------- */

static void
on_refresh_all_done(FeedApiResponse *response, gpointer data)
{
  ViewClosure *closure = data;
  SubsView *self = closure->view;

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
    feed_window_show_toast(self->window, "Subscriptions refreshed");
  }

  subs_view_reload_internal(self);
}

static void
on_refresh_all_clicked(GtkButton *button, gpointer data)
{
  SubsView *self = FEED_SUBS_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  feed_window_busy_push(self->window);
  ViewClosure *closure = view_closure_new(self);
  feed_api_refresh_all(subs_view_get_api(self), self->cancellable,
                       on_refresh_all_done, closure, view_closure_free);
}

/* ---------- policy / delete completions ---------- */

static void
on_policy_done(FeedApiResponse *response, gpointer data)
{
  PolicyClosure *closure = data;
  SubsView *self = closure->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
  } else {
    SubRow *row = sub_row_from_widget(closure->badge);
    if (row != NULL) {
      g_free(row->notify);
      row->notify = g_strdup(closure->policy);
    }
    gtk_button_set_label(GTK_BUTTON(closure->badge), closure->policy);
  }
}

static void
on_delete_chosen(GObject *source_object, GAsyncResult *result,
                 gpointer data)
{
  DeleteConfirm *confirm = data;
  AdwMessageDialog *dialog = ADW_MESSAGE_DIALOG(source_object);
  const char *response = adw_message_dialog_choose_finish(dialog, result);
  gboolean remove = g_strcmp0(response, "remove") == 0;

  g_object_unref(dialog);

  if (!remove || confirm->view->disposed) {
    delete_confirm_free(confirm);
    return;
  }

  SubsView *self = confirm->view;

  feed_window_busy_push(self->window);
  feed_api_delete_subscription(subs_view_get_api(self), confirm->id,
                               self->cancellable, on_delete_done,
                               confirm, delete_confirm_free);
}

static void
on_delete_done(FeedApiResponse *response, gpointer data)
{
  DeleteConfirm *confirm = data;
  SubsView *self = confirm->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (!response->ok) {
    if (response->error_message != NULL)
      feed_window_show_toast(self->window, response->error_message);
    return;
  }

  gtk_list_box_remove(self->list, confirm->row);
  feed_window_show_toast(self->window, "Subscription removed");

  if (subs_view_is_empty(self)) {
    subs_view_set_status(self,
      "No subscriptions yet. Click + to add a feed URL.");
  }
}

/* ---------- GObject boilerplate ---------- */

static void
subs_view_constructed(GObject *object)
{
  SubsView *self = FEED_SUBS_VIEW(object);

  G_OBJECT_CLASS(subs_view_parent_class)->constructed(object);

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
  gtk_widget_add_css_class(GTK_WIDGET(self->list), "boxed-list");
  gtk_widget_set_margin_top(GTK_WIDGET(self->list), 12);
  gtk_widget_set_margin_bottom(GTK_WIDGET(self->list), 12);
  gtk_widget_set_margin_start(GTK_WIDGET(self->list), 12);
  gtk_widget_set_margin_end(GTK_WIDGET(self->list), 12);
  gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(scroll),
                                GTK_WIDGET(self->list));
  gtk_box_append(GTK_BOX(self), scroll);

  /* Header bar extras: add + refresh-all buttons. */
  self->extras = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);

  GtkWidget *add = gtk_button_new_from_icon_name("list-add-symbolic");
  gtk_widget_set_tooltip_text(add, "Add subscription");
  g_signal_connect(add, "clicked", G_CALLBACK(on_add_clicked), self);
  gtk_box_append(GTK_BOX(self->extras), add);

  GtkWidget *refresh = gtk_button_new_from_icon_name("view-refresh-symbolic");
  gtk_widget_set_tooltip_text(refresh, "Fetch all subscriptions");
  g_signal_connect(refresh, "clicked", G_CALLBACK(on_refresh_all_clicked), self);
  gtk_box_append(GTK_BOX(self->extras), refresh);

  /* Add-subscription dialog. */
  self->add_dialog = adw_dialog_new();
  adw_dialog_set_title(self->add_dialog, "Add Subscription");
  adw_dialog_set_content_width(self->add_dialog, 420);

  GtkWidget *content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
  gtk_widget_set_margin_top(content, 12);
  gtk_widget_set_margin_bottom(content, 12);
  gtk_widget_set_margin_start(content, 12);
  gtk_widget_set_margin_end(content, 12);

  self->add_entry = GTK_ENTRY(gtk_entry_new());
  gtk_entry_set_placeholder_text(self->add_entry, "https://example.com/feed.xml");
  gtk_entry_set_activates_default(self->add_entry, TRUE);
  g_signal_connect(self->add_entry, "activate",
                   G_CALLBACK(on_add_entry_activate), self);
  gtk_box_append(GTK_BOX(content), GTK_WIDGET(self->add_entry));

  GtkWidget *buttons = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
  gtk_widget_set_halign(buttons, GTK_ALIGN_END);

  GtkWidget *cancel = gtk_button_new_with_label("_Cancel");
  gtk_button_set_use_underline(GTK_BUTTON(cancel), TRUE);
  g_signal_connect(cancel, "clicked", G_CALLBACK(on_add_cancel_clicked), self);
  gtk_box_append(GTK_BOX(buttons), cancel);

  GtkWidget *add_btn = gtk_button_new_with_label("_Add");
  gtk_button_set_use_underline(GTK_BUTTON(add_btn), TRUE);
  gtk_widget_add_css_class(add_btn, "suggested-action");
  g_signal_connect(add_btn, "clicked", G_CALLBACK(on_add_confirm_clicked), self);
  gtk_box_append(GTK_BOX(buttons), add_btn);

  gtk_box_append(GTK_BOX(content), buttons);
  adw_dialog_set_child(self->add_dialog, content);
}

static void
subs_view_dispose(GObject *object)
{
  SubsView *self = FEED_SUBS_VIEW(object);

  self->disposed = TRUE;
  if (self->cancellable != NULL) {
    g_cancellable_cancel(self->cancellable);
    g_clear_object(&self->cancellable);
  }
  g_clear_object(&self->add_dialog);

  G_OBJECT_CLASS(subs_view_parent_class)->dispose(object);
}

static void
subs_view_get_property(GObject *object, guint prop_id, GValue *value,
                       GParamSpec *pspec)
{
  SubsView *self = FEED_SUBS_VIEW(object);

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
subs_view_set_property(GObject *object, guint prop_id, const GValue *value,
                       GParamSpec *pspec)
{
  SubsView *self = FEED_SUBS_VIEW(object);

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
subs_view_class_init(SubsViewClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->constructed = subs_view_constructed;
  gobject_class->dispose = subs_view_dispose;
  gobject_class->get_property = subs_view_get_property;
  gobject_class->set_property = subs_view_set_property;

  props[PROP_WINDOW] =
    g_param_spec_object("window", NULL, NULL, FEED_TYPE_WINDOW,
                        G_PARAM_READWRITE | G_PARAM_CONSTRUCT_ONLY |
                          G_PARAM_STATIC_STRINGS);

  g_object_class_install_properties(gobject_class, N_PROPS, props);
}

static void
subs_view_init(SubsView *self)
{
  self->cancellable = g_cancellable_new();
}

/* ---------- public API ---------- */

GtkWidget *
subs_view_new(FeedWindow *window)
{
  g_return_val_if_fail(FEED_IS_WINDOW(window), NULL);

  return g_object_new(FEED_TYPE_SUBS_VIEW,
                      "orientation", GTK_ORIENTATION_VERTICAL,
                      "spacing", 0,
                      "window", window,
                      NULL);
}

GtkWidget *
subs_view_get_extras(SubsView *self)
{
  g_return_val_if_fail(FEED_IS_SUBS_VIEW(self), NULL);
  return self->extras;
}

void
subs_view_on_show(SubsView *self, FeedWindow *window)
{
  g_return_if_fail(FEED_IS_SUBS_VIEW(self));

  (void) window;

  if (self->disposed)
    return;

  if (!self->loaded_once)
    subs_view_load(self);
}

static void
subs_view_reload_internal(SubsView *self)
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

  subs_view_load(self);
}

void
subs_view_reload(SubsView *self)
{
  g_return_if_fail(FEED_IS_SUBS_VIEW(self));

  subs_view_reload_internal(self);
}
