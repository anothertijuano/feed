#include "config.h"

#include <errno.h>
#include <sys/stat.h>

#define CONFIG_GROUP "feed"

FeedConfig *
feed_config_new(void)
{
  FeedConfig *config = g_new0(FeedConfig, 1);
  config->server = g_strdup("");
  config->token = g_strdup("");
  config->memos_url = g_strdup("");
  config->memos_token = g_strdup("");
  return config;
}

void
feed_config_free(FeedConfig *config)
{
  if (config == NULL)
    return;
  g_free(config->server);
  g_free(config->token);
  g_free(config->memos_url);
  g_free(config->memos_token);
  g_free(config);
}

char *
feed_config_path(void)
{
  return g_build_filename(g_get_user_config_dir(), "feed", "gtk4.conf", NULL);
}

/* Returns the value of a key, or "" when the key is missing. */
static char *
dup_key(GKeyFile *keyfile, const char *key)
{
  char *value = g_key_file_get_string(keyfile, CONFIG_GROUP, key, NULL);
  return value != NULL ? value : g_strdup("");
}

FeedConfig *
feed_config_load(void)
{
  FeedConfig *config = feed_config_new();
  g_autofree char *path = feed_config_path();
  GKeyFile *keyfile = g_key_file_new();
  GError *error = NULL;

  if (!g_key_file_load_from_file(keyfile, path, G_KEY_FILE_NONE, &error)) {
    if (!g_error_matches(error, G_KEY_FILE_ERROR, G_KEY_FILE_ERROR_NOT_FOUND))
      g_warning("failed to load %s: %s", path, error->message);
    g_error_free(error);
    g_key_file_unref(keyfile);
    return config;
  }

  g_free(config->server);
  config->server = dup_key(keyfile, "server");
  g_free(config->token);
  config->token = dup_key(keyfile, "token");
  g_free(config->memos_url);
  config->memos_url = dup_key(keyfile, "memosUrl");
  g_free(config->memos_token);
  config->memos_token = dup_key(keyfile, "memosToken");

  g_key_file_unref(keyfile);
  return config;
}

gboolean
feed_config_save(FeedConfig *config, GError **error)
{
  g_return_val_if_fail(config != NULL, FALSE);

  GKeyFile *keyfile = g_key_file_new();
  g_key_file_set_string(keyfile, CONFIG_GROUP, "server",
                        config->server != NULL ? config->server : "");
  g_key_file_set_string(keyfile, CONFIG_GROUP, "token",
                        config->token != NULL ? config->token : "");
  g_key_file_set_string(keyfile, CONFIG_GROUP, "memosUrl",
                        config->memos_url != NULL ? config->memos_url : "");
  g_key_file_set_string(keyfile, CONFIG_GROUP, "memosToken",
                        config->memos_token != NULL ? config->memos_token : "");

  g_autofree char *path = feed_config_path();
  g_autofree char *dir = g_path_get_dirname(path);

  if (g_mkdir_with_parents(dir, 0700) != 0) {
    g_set_error(error, G_FILE_ERROR, g_file_error_from_errno(errno),
                "cannot create %s: %s", dir, g_strerror(errno));
    g_key_file_unref(keyfile);
    return FALSE;
  }

  gboolean ok = g_key_file_save_to_file(keyfile, path, error);
  g_key_file_unref(keyfile);
  if (!ok)
    return FALSE;

  /* The file holds a token, so keep it private to the user. */
  if (chmod(path, 0600) != 0)
    g_warning("could not chmod %s to 0600: %s", path, g_strerror(errno));

  return TRUE;
}
