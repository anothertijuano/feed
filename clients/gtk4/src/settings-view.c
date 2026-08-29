#include "settings-view.h"

#include "api.h"

struct _SettingsView {
  GtkBox parent_instance;
  FeedWindow *window; /* borrowed */
  GCancellable *cancellable;
  AdwEntryRow *server_row;
  AdwPasswordEntryRow *token_row;
  AdwEntryRow *memos_url_row;
  AdwPasswordEntryRow *memos_token_row;
  gboolean disposed;
};

G_DEFINE_FINAL_TYPE(SettingsView, settings_view, GTK_TYPE_BOX)

enum {
  SIGNAL_SAVED,
  N_SIGNALS
};

static guint signals[N_SIGNALS];

enum {
  PROP_WINDOW = 1,
  N_PROPS
};

static GParamSpec *props[N_PROPS];

/* ---------- connection test ---------- */

typedef struct {
  SettingsView *view; /* owned ref */
  FeedApi *api;       /* owned */
  gboolean health_ok;
  gboolean settings_ok;
  char *health_error;
  char *settings_error;
  guint pending;
} TestData;

static void
test_data_complete(TestData *test)
{
  SettingsView *self = test->view;

  if (!self->disposed) {
    feed_window_busy_pop(self->window);

    if (test->health_ok && test->settings_ok) {
      feed_window_show_toast(self->window,
                             "Connection OK — server reachable and token accepted.");
    } else if (!test->health_ok) {
      feed_window_show_toast(self->window,
        test->health_error != NULL ? test->health_error
                                   : "Could not reach the server.");
    } else {
      feed_window_show_toast(self->window,
        test->settings_error != NULL ? test->settings_error
                                     : "Server reachable, but the token was rejected.");
    }
  }

  g_object_unref(test->api);
  g_object_unref(test->view);
  g_free(test->health_error);
  g_free(test->settings_error);
  g_free(test);
}

static void
on_test_health(FeedApiResponse *response, gpointer data)
{
  TestData *test = data;

  test->health_ok = response->ok;
  if (!response->ok && response->error_message != NULL)
    test->health_error = g_strdup(response->error_message);

  test->pending--;
  if (test->pending == 0)
    test_data_complete(test);
}

static void
on_test_settings(FeedApiResponse *response, gpointer data)
{
  TestData *test = data;

  test->settings_ok = response->ok;
  if (!response->ok && response->error_message != NULL)
    test->settings_error = g_strdup(response->error_message);

  test->pending--;
  if (test->pending == 0)
    test_data_complete(test);
}

static void
on_test_clicked(GtkButton *button, gpointer data)
{
  SettingsView *self = FEED_SETTINGS_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  g_autofree char *server = NULL;
  g_autofree char *token = NULL;
  g_object_get(self->server_row, "text", &server, NULL);
  g_object_get(self->token_row, "text", &token, NULL);

  FeedApi *api = feed_api_new(server, token);
  if (feed_api_get_server(api) == NULL) {
    feed_window_show_toast(self->window, "Enter a valid server URL first.");
    g_object_unref(api);
    return;
  }

  TestData *test = g_new0(TestData, 1);
  test->view = g_object_ref(self);
  test->api = api;
  test->pending = 2;

  feed_window_busy_push(self->window);
  feed_api_get_health(api, self->cancellable, on_test_health, test, NULL);
  feed_api_get_settings(api, self->cancellable, on_test_settings, test, NULL);
}

/* ---------- save ---------- */

typedef struct {
  SettingsView *view; /* owned ref */
} SaveClosure;

static void
save_closure_free(gpointer data)
{
  SaveClosure *closure = data;

  g_object_unref(closure->view);
  g_free(closure);
}

static void
on_post_settings_done(FeedApiResponse *response, gpointer data)
{
  SaveClosure *closure = data;
  SettingsView *self = closure->view;

  if (self->disposed)
    return;

  feed_window_busy_pop(self->window);

  if (response->ok) {
    feed_window_show_toast(self->window, "Settings saved.");
  } else {
    g_autofree char *message =
      g_strdup_printf("Saved locally, but the server could not be updated: %s",
                      response->error_message != NULL
                        ? response->error_message : "unknown error");
    feed_window_show_toast(self->window, message);
  }
}

static void
on_save_clicked(GtkButton *button, gpointer data)
{
  SettingsView *self = FEED_SETTINGS_VIEW(data);

  (void) button;

  if (self->disposed)
    return;

  char *server = NULL;
  char *token = NULL;
  char *memos_url = NULL;
  char *memos_token = NULL;
  g_object_get(self->server_row, "text", &server, NULL);
  g_object_get(self->token_row, "text", &token, NULL);
  g_object_get(self->memos_url_row, "text", &memos_url, NULL);
  g_object_get(self->memos_token_row, "text", &memos_token, NULL);
  g_strstrip(server);
  g_strstrip(memos_url);

  if (server[0] == '\0') {
    feed_window_show_toast(self->window, "Server URL is required.");
    g_free(server);
    g_free(token);
    g_free(memos_url);
    g_free(memos_token);
    return;
  }

  FeedConfig *config = feed_window_get_config(self->window);
  g_free(config->server);
  config->server = server;
  g_free(config->token);
  config->token = token;
  g_free(config->memos_url);
  config->memos_url = memos_url;
  g_free(config->memos_token);
  config->memos_token = memos_token;

  GError *error = NULL;
  if (!feed_config_save(config, &error)) {
    g_autofree char *message =
      g_strdup_printf("Could not save settings: %s", error->message);
    feed_window_show_toast(self->window, message);
    g_error_free(error);
    return;
  }

  FeedApi *api = feed_window_get_api(self->window);
  feed_api_set_server(api, config->server);
  feed_api_set_token(api, config->token);

  g_signal_emit(self, signals[SIGNAL_SAVED], 0);

  /* Best-effort: push the memos settings to the server too. */
  feed_window_busy_push(self->window);

  SaveClosure *closure = g_new0(SaveClosure, 1);
  closure->view = g_object_ref(self);

  feed_api_post_settings(api, config->memos_url, config->memos_token,
                         self->cancellable, on_post_settings_done,
                         closure, save_closure_free);
}

/* ---------- GObject boilerplate ---------- */

static void
settings_view_constructed(GObject *object)
{
  SettingsView *self = FEED_SETTINGS_VIEW(object);

  G_OBJECT_CLASS(settings_view_parent_class)->constructed(object);

  FeedConfig *config = feed_window_get_config(self->window);

  GtkWidget *page = adw_preferences_page_new();

  /* Server group. */
  GtkWidget *server_group = adw_preferences_group_new();
  adw_preferences_group_set_title(ADW_PREFERENCES_GROUP(server_group),
                                  "Server");
  adw_preferences_group_set_description(ADW_PREFERENCES_GROUP(server_group),
    "Connection settings for your self-hosted feed server.");

  self->server_row = ADW_ENTRY_ROW(adw_entry_row_new());
  adw_preferences_row_set_title(ADW_PREFERENCES_ROW(self->server_row), "Server URL");
  g_object_set(self->server_row, "text", config->server, NULL);
  adw_preferences_group_add(ADW_PREFERENCES_GROUP(server_group),
                            GTK_WIDGET(self->server_row));

  self->token_row = ADW_PASSWORD_ENTRY_ROW(adw_password_entry_row_new());
  adw_preferences_row_set_title(ADW_PREFERENCES_ROW(self->token_row), "API Token");
  g_object_set(self->token_row, "text", config->token, NULL);
  adw_preferences_group_add(ADW_PREFERENCES_GROUP(server_group),
                            GTK_WIDGET(self->token_row));

  GtkWidget *test_row = adw_action_row_new();
  adw_preferences_row_set_title(ADW_PREFERENCES_ROW(test_row), "Connection");
  adw_action_row_set_subtitle(ADW_ACTION_ROW(test_row),
    "Check that the server is reachable and the token is valid.");
  GtkWidget *test_button = gtk_button_new_with_label("Test");
  gtk_widget_add_css_class(test_button, "pill");
  g_signal_connect(test_button, "clicked", G_CALLBACK(on_test_clicked), self);
  adw_action_row_add_suffix(ADW_ACTION_ROW(test_row), test_button);
  adw_preferences_group_add(ADW_PREFERENCES_GROUP(server_group), test_row);

  adw_preferences_page_add(ADW_PREFERENCES_PAGE(page),
                           ADW_PREFERENCES_GROUP(server_group));

  /* Memos group. */
  GtkWidget *memos_group = adw_preferences_group_new();
  adw_preferences_group_set_title(ADW_PREFERENCES_GROUP(memos_group), "Memos");
  adw_preferences_group_set_description(ADW_PREFERENCES_GROUP(memos_group),
    "Optional integration with a Memos instance. Saved locally and pushed to "
    "the server.");

  self->memos_url_row = ADW_ENTRY_ROW(adw_entry_row_new());
  adw_preferences_row_set_title(ADW_PREFERENCES_ROW(self->memos_url_row), "Memos URL");
  g_object_set(self->memos_url_row, "text", config->memos_url, NULL);
  adw_preferences_group_add(ADW_PREFERENCES_GROUP(memos_group),
                            GTK_WIDGET(self->memos_url_row));

  self->memos_token_row = ADW_PASSWORD_ENTRY_ROW(adw_password_entry_row_new());
  adw_preferences_row_set_title(ADW_PREFERENCES_ROW(self->memos_token_row), "Memos Token");
  g_object_set(self->memos_token_row, "text", config->memos_token, NULL);
  adw_preferences_group_add(ADW_PREFERENCES_GROUP(memos_group),
                            GTK_WIDGET(self->memos_token_row));

  adw_preferences_page_add(ADW_PREFERENCES_PAGE(page),
                           ADW_PREFERENCES_GROUP(memos_group));

  /* Save group. */
  GtkWidget *save_group = adw_preferences_group_new();
  adw_preferences_group_set_title(ADW_PREFERENCES_GROUP(save_group), "Save");
  g_autofree char *config_note =
    g_strdup_printf("Settings are stored in %s/feed/gtk4.conf (mode 0600).",
                    g_get_user_config_dir());
  adw_preferences_group_set_description(ADW_PREFERENCES_GROUP(save_group),
                                        config_note);

  GtkWidget *save_row = adw_action_row_new();
  adw_preferences_row_set_title(ADW_PREFERENCES_ROW(save_row), "Save Settings");
  GtkWidget *save_button = gtk_button_new_with_label("Save");
  gtk_widget_add_css_class(save_button, "suggested-action");
  g_signal_connect(save_button, "clicked", G_CALLBACK(on_save_clicked), self);
  adw_action_row_add_suffix(ADW_ACTION_ROW(save_row), save_button);
  adw_preferences_group_add(ADW_PREFERENCES_GROUP(save_group), save_row);

  adw_preferences_page_add(ADW_PREFERENCES_PAGE(page),
                           ADW_PREFERENCES_GROUP(save_group));

  GtkWidget *scroll = gtk_scrolled_window_new();
  gtk_widget_set_hexpand(scroll, TRUE);
  gtk_widget_set_vexpand(scroll, TRUE);
  gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scroll), GTK_POLICY_NEVER,
                                 GTK_POLICY_AUTOMATIC);
  gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(scroll), page);
  gtk_box_append(GTK_BOX(self), scroll);
}

static void
settings_view_dispose(GObject *object)
{
  SettingsView *self = FEED_SETTINGS_VIEW(object);

  self->disposed = TRUE;
  if (self->cancellable != NULL) {
    g_cancellable_cancel(self->cancellable);
    g_clear_object(&self->cancellable);
  }

  G_OBJECT_CLASS(settings_view_parent_class)->dispose(object);
}

static void
settings_view_get_property(GObject *object, guint prop_id, GValue *value,
                           GParamSpec *pspec)
{
  SettingsView *self = FEED_SETTINGS_VIEW(object);

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
settings_view_set_property(GObject *object, guint prop_id, const GValue *value,
                           GParamSpec *pspec)
{
  SettingsView *self = FEED_SETTINGS_VIEW(object);

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
settings_view_class_init(SettingsViewClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->constructed = settings_view_constructed;
  gobject_class->dispose = settings_view_dispose;
  gobject_class->get_property = settings_view_get_property;
  gobject_class->set_property = settings_view_set_property;

  signals[SIGNAL_SAVED] =
    g_signal_new("saved", FEED_TYPE_SETTINGS_VIEW, G_SIGNAL_RUN_LAST, 0,
                 NULL, NULL, NULL, G_TYPE_NONE, 0);

  props[PROP_WINDOW] =
    g_param_spec_object("window", NULL, NULL, FEED_TYPE_WINDOW,
                        G_PARAM_READWRITE | G_PARAM_CONSTRUCT_ONLY |
                          G_PARAM_STATIC_STRINGS);

  g_object_class_install_properties(gobject_class, N_PROPS, props);
}

static void
settings_view_init(SettingsView *self)
{
  self->cancellable = g_cancellable_new();
}

/* ---------- public API ---------- */

GtkWidget *
settings_view_new(FeedWindow *window)
{
  g_return_val_if_fail(FEED_IS_WINDOW(window), NULL);

  return g_object_new(FEED_TYPE_SETTINGS_VIEW,
                      "orientation", GTK_ORIENTATION_VERTICAL,
                      "spacing", 0,
                      "window", window,
                      NULL);
}
