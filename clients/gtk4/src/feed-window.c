#include "feed-window.h"

struct _FeedWindow {
  AdwApplicationWindow parent_instance;
  FeedConfig *config; /* owned */
  FeedApi *api;       /* owned */
  AdwToastOverlay *toast_overlay;
  AdwHeaderBar *header_bar;
  AdwWindowTitle *title_widget;
  GtkSpinner *spinner;
  GtkBox *extra_box;   /* host for per-view header bar widgets */
  GtkStack *stack;
  GtkListBox *sidebar;
  GHashTable *views;   /* name -> ViewRecord* (records own their name/title) */
  guint busy;
};

typedef struct {
  char *name;
  char *title;
  GtkWidget *widget;  /* borrowed */
  GtkWidget *extras;  /* borrowed */
  FeedViewShowFunc on_show;
} ViewRecord;

G_DEFINE_FINAL_TYPE(FeedWindow, feed_window, ADW_TYPE_APPLICATION_WINDOW)

enum {
  PROP_CONFIG = 1,
  N_PROPS
};

static GParamSpec *props[N_PROPS];

static void
view_record_free(gpointer data)
{
  ViewRecord *record = data;

  if (record == NULL)
    return;
  g_free(record->name);
  g_free(record->title);
  g_free(record);
}

static void
on_row_activated(GtkListBox *box, GtkListBoxRow *row, gpointer data)
{
  FeedWindow *self = FEED_WINDOW(data);
  const char *name = g_object_get_data(G_OBJECT(row), "view-name");

  (void) box;

  if (name == NULL)
    return;

  ViewRecord *record = g_hash_table_lookup(self->views, name);
  if (record == NULL)
    return;

  gtk_stack_set_visible_child_name(self->stack, name);
  adw_window_title_set_title(self->title_widget, record->title);
  gtk_window_set_title(GTK_WINDOW(self), record->title);

  GHashTableIter iter;
  gpointer value;
  g_hash_table_iter_init(&iter, self->views);
  while (g_hash_table_iter_next(&iter, NULL, &value)) {
    ViewRecord *entry = value;
    if (entry->extras != NULL)
      gtk_widget_set_visible(entry->extras, entry == record);
  }

  if (record->on_show != NULL)
    record->on_show(record->widget, self);
}

static void
switch_to(FeedWindow *self, const char *name)
{
  if (g_hash_table_lookup(self->views, name) == NULL)
    return;

  /* Find and select the matching sidebar row without relying on ordering. */
  GtkListBoxRow *selected = NULL;
  GtkWidget *child = gtk_widget_get_first_child(GTK_WIDGET(self->sidebar));
  for (; child != NULL; child = gtk_widget_get_next_sibling(child)) {
    const char *row_name = g_object_get_data(G_OBJECT(child), "view-name");
    if (g_strcmp0(row_name, name) == 0) {
      selected = GTK_LIST_BOX_ROW(child);
      break;
    }
  }
  if (selected != NULL)
    gtk_list_box_select_row(self->sidebar, selected);

  on_row_activated(self->sidebar, selected, self);
}

static void
feed_window_constructed(GObject *object)
{
  FeedWindow *self = FEED_WINDOW(object);

  G_OBJECT_CLASS(feed_window_parent_class)->constructed(object);

  self->api = feed_api_new(self->config != NULL ? self->config->server : NULL,
                           self->config != NULL ? self->config->token : NULL);

  gtk_window_set_default_size(GTK_WINDOW(self), 1080, 760);
  gtk_window_set_title(GTK_WINDOW(self), "Feed");
  gtk_window_set_icon_name(GTK_WINDOW(self), "com.anothertijuano.feed.Gtk4");

  /* Header bar. */
  self->header_bar = ADW_HEADER_BAR(adw_header_bar_new());
  self->title_widget = ADW_WINDOW_TITLE(adw_window_title_new("Feed", NULL));
  adw_header_bar_set_title_widget(self->header_bar, GTK_WIDGET(self->title_widget));

  self->spinner = GTK_SPINNER(gtk_spinner_new());
  gtk_spinner_set_spinning(self->spinner, TRUE);
  gtk_widget_set_visible(GTK_WIDGET(self->spinner), FALSE);
  gtk_widget_set_margin_end(GTK_WIDGET(self->spinner), 6);
  adw_header_bar_pack_end(self->header_bar, GTK_WIDGET(self->spinner));

  self->extra_box = GTK_BOX(gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0));
  adw_header_bar_pack_end(self->header_bar, GTK_WIDGET(self->extra_box));

  /* Sidebar. */
  self->sidebar = GTK_LIST_BOX(gtk_list_box_new());
  gtk_list_box_set_selection_mode(self->sidebar, GTK_SELECTION_SINGLE);
  gtk_widget_add_css_class(GTK_WIDGET(self->sidebar), "navigation-sidebar");
  g_signal_connect(self->sidebar, "row-activated",
                   G_CALLBACK(on_row_activated), self);
  AdwNavigationPage *sidebar_page =
    adw_navigation_page_new(GTK_WIDGET(self->sidebar), "Sections");

  /* Content. */
  self->stack = GTK_STACK(gtk_stack_new());
  gtk_stack_set_transition_type(self->stack, GTK_STACK_TRANSITION_TYPE_CROSSFADE);
  AdwNavigationPage *content_page =
    adw_navigation_page_new(GTK_WIDGET(self->stack), "Content");

  GtkWidget *split = adw_navigation_split_view_new();
  adw_navigation_split_view_set_sidebar(ADW_NAVIGATION_SPLIT_VIEW(split),
                                        sidebar_page);
  adw_navigation_split_view_set_content(ADW_NAVIGATION_SPLIT_VIEW(split),
                                        content_page);
  adw_navigation_split_view_set_show_content(ADW_NAVIGATION_SPLIT_VIEW(split),
                                             TRUE);
  adw_navigation_split_view_set_collapsed(ADW_NAVIGATION_SPLIT_VIEW(split),
                                          FALSE);
  adw_navigation_split_view_set_min_sidebar_width(ADW_NAVIGATION_SPLIT_VIEW(split),
                                                  180.0);
  adw_navigation_split_view_set_max_sidebar_width(ADW_NAVIGATION_SPLIT_VIEW(split),
                                                  280.0);

  GtkWidget *toolbar = adw_toolbar_view_new();
  adw_toolbar_view_add_top_bar(ADW_TOOLBAR_VIEW(toolbar),
                               GTK_WIDGET(self->header_bar));
  adw_toolbar_view_set_content(ADW_TOOLBAR_VIEW(toolbar), split);

  self->toast_overlay = ADW_TOAST_OVERLAY(adw_toast_overlay_new());
  adw_toast_overlay_set_child(self->toast_overlay, toolbar);

  adw_application_window_set_content(ADW_APPLICATION_WINDOW(self),
                                     GTK_WIDGET(self->toast_overlay));
}

static void
feed_window_dispose(GObject *object)
{
  FeedWindow *self = FEED_WINDOW(object);

  g_clear_object(&self->api);
  g_clear_pointer(&self->views, g_hash_table_unref);

  G_OBJECT_CLASS(feed_window_parent_class)->dispose(object);
}

static void
feed_window_finalize(GObject *object)
{
  FeedWindow *self = FEED_WINDOW(object);

  feed_config_free(self->config);
  self->config = NULL;

  G_OBJECT_CLASS(feed_window_parent_class)->finalize(object);
}

static void
feed_window_get_property(GObject *object, guint prop_id, GValue *value,
                         GParamSpec *pspec)
{
  FeedWindow *self = FEED_WINDOW(object);

  switch (prop_id) {
  case PROP_CONFIG:
    g_value_set_pointer(value, self->config);
    break;
  default:
    G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    break;
  }
}

static void
feed_window_set_property(GObject *object, guint prop_id, const GValue *value,
                         GParamSpec *pspec)
{
  FeedWindow *self = FEED_WINDOW(object);

  switch (prop_id) {
  case PROP_CONFIG:
    self->config = g_value_get_pointer(value); /* takes ownership */
    break;
  default:
    G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    break;
  }
}

static void
feed_window_class_init(FeedWindowClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->constructed = feed_window_constructed;
  gobject_class->dispose = feed_window_dispose;
  gobject_class->finalize = feed_window_finalize;
  gobject_class->get_property = feed_window_get_property;
  gobject_class->set_property = feed_window_set_property;

  props[PROP_CONFIG] =
    g_param_spec_pointer("config", NULL, NULL,
                         G_PARAM_WRITABLE | G_PARAM_CONSTRUCT_ONLY |
                           G_PARAM_STATIC_STRINGS);

  g_object_class_install_properties(gobject_class, N_PROPS, props);
}

static void
feed_window_init(FeedWindow *self)
{
  self->views = g_hash_table_new_full(g_str_hash, g_str_equal, NULL,
                                      view_record_free);
}

FeedWindow *
feed_window_new(AdwApplication *app, FeedConfig *config)
{
  g_return_val_if_fail(ADW_IS_APPLICATION(app), NULL);
  g_return_val_if_fail(config != NULL, NULL);

  return g_object_new(FEED_TYPE_WINDOW,
                      "application", app,
                      "config", config,
                      NULL);
}

FeedApi *
feed_window_get_api(FeedWindow *self)
{
  g_return_val_if_fail(FEED_IS_WINDOW(self), NULL);
  return self->api;
}

FeedConfig *
feed_window_get_config(FeedWindow *self)
{
  g_return_val_if_fail(FEED_IS_WINDOW(self), NULL);
  return self->config;
}

void
feed_window_busy_push(FeedWindow *self)
{
  g_return_if_fail(FEED_IS_WINDOW(self));

  self->busy++;
  gtk_widget_set_visible(GTK_WIDGET(self->spinner), self->busy > 0);
}

void
feed_window_busy_pop(FeedWindow *self)
{
  g_return_if_fail(FEED_IS_WINDOW(self));

  if (self->busy > 0)
    self->busy--;
  gtk_widget_set_visible(GTK_WIDGET(self->spinner), self->busy > 0);
}

void
feed_window_show_toast(FeedWindow *self, const char *message)
{
  g_return_if_fail(FEED_IS_WINDOW(self));

  if (message == NULL || message[0] == '\0')
    return;

  AdwToast *toast = adw_toast_new(message);
  adw_toast_set_timeout(toast, 4);
  adw_toast_overlay_add_toast(self->toast_overlay, toast);
}

void
feed_window_add_view(FeedWindow *self, const char *name,
                     const char *title, const char *icon_name,
                     GtkWidget *view, GtkWidget *extras,
                     FeedViewShowFunc on_show)
{
  g_return_if_fail(FEED_IS_WINDOW(self));
  g_return_if_fail(name != NULL);
  g_return_if_fail(title != NULL);
  g_return_if_fail(GTK_IS_WIDGET(view));

  gtk_stack_add_named(self->stack, view, name);

  GtkWidget *row = gtk_list_box_row_new();
  GtkWidget *row_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
  GtkWidget *icon = gtk_image_new_from_icon_name(icon_name);
  GtkWidget *label = gtk_label_new(title);
  gtk_label_set_xalign(GTK_LABEL(label), 0.0f);
  gtk_widget_set_hexpand(label, TRUE);
  gtk_box_append(GTK_BOX(row_box), icon);
  gtk_box_append(GTK_BOX(row_box), label);
  gtk_list_box_row_set_child(GTK_LIST_BOX_ROW(row), row_box);
  g_object_set_data_full(G_OBJECT(row), "view-name", g_strdup(name), g_free);
  gtk_list_box_append(self->sidebar, row);

  if (extras != NULL)
    gtk_box_append(self->extra_box, extras);

  ViewRecord *record = g_new0(ViewRecord, 1);
  record->name = g_strdup(name);
  record->title = g_strdup(title);
  record->widget = view;
  record->extras = extras;
  record->on_show = on_show;
  g_hash_table_insert(self->views, record->name, record);

  if (g_hash_table_size(self->views) == 1)
    switch_to(self, name);
}
