/*
 * api.h - asynchronous HTTP client for the feed server.
 *
 * All requests run on the main context via libsoup's async callbacks; nothing
 * here ever blocks the UI thread. A request always completes exactly once,
 * asynchronously, by invoking `cb` with a FeedApiResponse that is only valid
 * for the duration of the callback (copy what you need out of it).
 */

#ifndef FEED_API_H
#define FEED_API_H

#include <gio/gio.h>
#include <json-glib/json-glib.h>
#include <libsoup/soup.h>

G_BEGIN_DECLS

#define FEED_TYPE_API (feed_api_get_type())
G_DECLARE_FINAL_TYPE(FeedApi, feed_api, FEED, API, GObject)

typedef struct {
  gboolean ok;           /* HTTP status was 2xx and the body was received */
  guint status;          /* HTTP status; 0 when the transport failed */
  char *error_message;   /* server "error" field or transport error; NULL if none */
  JsonNode *root;        /* parsed response body; NULL when absent */
} FeedApiResponse;

typedef void (*FeedApiCallback)(FeedApiResponse *response, gpointer user_data);

FeedApi *feed_api_new(const char *server, const char *token);

void feed_api_set_server(FeedApi *self, const char *server);
void feed_api_set_token(FeedApi *self, const char *token);
const char *feed_api_get_server(FeedApi *self);
const char *feed_api_get_token(FeedApi *self);

/* Feed listing. */
void feed_api_get_feed(FeedApi *self, gint64 limit, gint64 offset,
                       GCancellable *cancellable,
                       FeedApiCallback cb, gpointer user_data,
                       GDestroyNotify notify);
void feed_api_get_saved(FeedApi *self, GCancellable *cancellable,
                        FeedApiCallback cb, gpointer user_data,
                        GDestroyNotify notify);

/* Interactions. */
void feed_api_vote(FeedApi *self, const char *key, gint64 value,
                   GCancellable *cancellable,
                   FeedApiCallback cb, gpointer user_data,
                   GDestroyNotify notify);
void feed_api_save(FeedApi *self, const char *key, gboolean saved,
                   GCancellable *cancellable,
                   FeedApiCallback cb, gpointer user_data,
                   GDestroyNotify notify);
void feed_api_seen(FeedApi *self, const char *key,
                   GCancellable *cancellable,
                   FeedApiCallback cb, gpointer user_data,
                   GDestroyNotify notify);

/* Subscriptions. */
void feed_api_get_subscriptions(FeedApi *self, GCancellable *cancellable,
                                FeedApiCallback cb, gpointer user_data,
                                GDestroyNotify notify);
void feed_api_add_subscription(FeedApi *self, const char *url,
                               GCancellable *cancellable,
                               FeedApiCallback cb, gpointer user_data,
                               GDestroyNotify notify);
void feed_api_delete_subscription(FeedApi *self, const char *id,
                                  GCancellable *cancellable,
                                  FeedApiCallback cb, gpointer user_data,
                                  GDestroyNotify notify);
void feed_api_set_subscription_notify(FeedApi *self, const char *id,
                                      const char *policy,
                                      GCancellable *cancellable,
                                      FeedApiCallback cb, gpointer user_data,
                                      GDestroyNotify notify);
void feed_api_refresh_all(FeedApi *self, GCancellable *cancellable,
                          FeedApiCallback cb, gpointer user_data,
                          GDestroyNotify notify);

/* Settings and health. */
void feed_api_get_settings(FeedApi *self, GCancellable *cancellable,
                           FeedApiCallback cb, gpointer user_data,
                           GDestroyNotify notify);
void feed_api_post_settings(FeedApi *self, const char *memos_url,
                            const char *memos_token,
                            GCancellable *cancellable,
                            FeedApiCallback cb, gpointer user_data,
                            GDestroyNotify notify);
void feed_api_get_health(FeedApi *self, GCancellable *cancellable,
                         FeedApiCallback cb, gpointer user_data,
                         GDestroyNotify notify);

G_END_DECLS

#endif /* FEED_API_H */
