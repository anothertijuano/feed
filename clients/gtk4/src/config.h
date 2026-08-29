/*
 * config.h - persistent configuration for the feed GTK4 client.
 *
 * Settings live in ~/.config/feed/gtk4.conf (a GKeyFile) and are written
 * with permissions 0600 because the file contains an API token.
 */

#ifndef FEED_CONFIG_H
#define FEED_CONFIG_H

#include <glib.h>

G_BEGIN_DECLS

typedef struct {
  char *server;
  char *token;
  char *memos_url;
  char *memos_token;
} FeedConfig;

FeedConfig *feed_config_new(void);
FeedConfig *feed_config_load(void);
gboolean feed_config_save(FeedConfig *config, GError **error);
void feed_config_free(FeedConfig *config);

/* Returns the absolute path of the config file (newly allocated). */
char *feed_config_path(void);

G_END_DECLS

#endif /* FEED_CONFIG_H */
