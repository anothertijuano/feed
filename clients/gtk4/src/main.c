#include <adwaita.h>
#include <gdk/gdk.h>

#include "config.h"
#include "feed-view.h"
#include "feed-window.h"
#include "saved-view.h"
#include "settings-view.h"
#include "subs-view.h"

#define APP_ID "com.anothertijuano.feed.Gtk4"

typedef struct {
  FeedView *feed;
  SavedView *saved;
  SubsView *subs;
} ViewRefs;

static void
load_custom_css(void)
{
  GtkCssProvider *provider = gtk_css_provider_new();
  gtk_css_provider_load_from_string(provider, ".feed-thumb { border-radius: 8px; }");
  gtk_style_context_add_provider_for_display(
    gdk_display_get_default(), GTK_STYLE_PROVIDER(provider),
    GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
  g_object_unref(provider);
}

static void
on_settings_saved(SettingsView *view, gpointer data)
{
  ViewRefs *refs = data;

  (void) view;

  feed_view_reload(refs->feed);
  saved_view_reload(refs->saved);
  subs_view_reload(refs->subs);
}

static void
on_activate(GApplication *app, gpointer data)
{
  (void) data;

  load_custom_css();

  FeedConfig *config = feed_config_load();
  FeedWindow *window = feed_window_new(ADW_APPLICATION(app), config);

  GtkWidget *feed_widget = feed_view_new(window);
  GtkWidget *saved_widget = saved_view_new(window);
  GtkWidget *subs_widget = subs_view_new(window);
  GtkWidget *settings_widget = settings_view_new(window);

  FeedView *feed = FEED_VIEW(feed_widget);
  SavedView *saved = FEED_SAVED_VIEW(saved_widget);
  SubsView *subs = FEED_SUBS_VIEW(subs_widget);
  SettingsView *settings = FEED_SETTINGS_VIEW(settings_widget);

  feed_window_add_view(window, "feed", "Feed", "view-list-symbolic",
                       feed_widget, feed_view_get_extras(feed),
                       (FeedViewShowFunc) feed_view_on_show);
  feed_window_add_view(window, "saved", "Saved", "starred-symbolic",
                       saved_widget, saved_view_get_extras(saved),
                       (FeedViewShowFunc) saved_view_on_show);
  feed_window_add_view(window, "subs", "Subscriptions", "feed-subscribe-symbolic",
                       subs_widget, subs_view_get_extras(subs),
                       (FeedViewShowFunc) subs_view_on_show);
  feed_window_add_view(window, "settings", "Settings", "emblem-system-symbolic",
                       settings_widget, NULL, NULL);

  ViewRefs *refs = g_new0(ViewRefs, 1);
  refs->feed = feed;
  refs->saved = saved;
  refs->subs = subs;
  g_object_set_data_full(G_OBJECT(window), "view-refs", refs, g_free);
  g_signal_connect(settings_widget, "saved", G_CALLBACK(on_settings_saved), refs);

  gtk_window_present(GTK_WINDOW(window));
}

int
main(int argc, char **argv)
{
  AdwApplication *app = adw_application_new(APP_ID, G_APPLICATION_DEFAULT_FLAGS);
  g_signal_connect(app, "activate", G_CALLBACK(on_activate), NULL);
  int status = g_application_run(G_APPLICATION(app), argc, argv);
  g_object_unref(app);
  return status;
}
