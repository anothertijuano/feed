#include "api.h"

#include <string.h>

struct _FeedApi {
  GObject parent_instance;
  SoupSession *session;
  char *server; /* normalized base URL without trailing slash, or NULL */
  char *token;
};

G_DEFINE_FINAL_TYPE(FeedApi, feed_api, G_TYPE_OBJECT)

enum {
  PROP_SERVER = 1,
  PROP_TOKEN,
  N_PROPS
};

static GParamSpec *props[N_PROPS];

/* Normalizes a user-supplied server address: trims whitespace and trailing
 * slashes, adds http:// when no scheme is present, and returns NULL when the
 * result is not a valid absolute URI. */
static char *
normalize_server(const char *server)
{
  if (server == NULL)
    return NULL;

  char *value = g_strdup(server);
  g_strstrip(value);
  while (value[0] != '\0' && value[strlen(value) - 1] == '/')
    value[strlen(value) - 1] = '\0';

  if (strstr(value, "://") == NULL) {
    char *prefixed = g_strconcat("http://", value, NULL);
    g_free(value);
    value = prefixed;
  }

  GUri *uri = g_uri_parse(value, G_URI_FLAGS_NONE, NULL);
  if (uri == NULL) {
    g_free(value);
    return NULL;
  }
  g_uri_unref(uri);
  return value;
}

/* ---------- request plumbing ---------- */

typedef struct {
  SoupSession *session;      /* owned ref */
  SoupMessage *message;      /* owned ref */
  GCancellable *cancellable; /* owned ref, may be NULL */
  FeedApiCallback cb;
  gpointer user_data;
  GDestroyNotify notify;
} RequestData;

static void
feed_api_response_free(FeedApiResponse *response)
{
  g_clear_pointer(&response->error_message, g_free);
  g_clear_pointer(&response->root, json_node_unref);
}

typedef struct {
  FeedApiResponse response;
  FeedApiCallback cb;
  gpointer user_data;
  GDestroyNotify notify;
} ErrorDelivery;

static gboolean
deliver_error_idle(gpointer data)
{
  ErrorDelivery *delivery = data;
  if (delivery->cb != NULL)
    delivery->cb(&delivery->response, delivery->user_data);
  feed_api_response_free(&delivery->response);
  if (delivery->notify != NULL)
    delivery->notify(delivery->user_data);
  g_free(delivery);
  return G_SOURCE_REMOVE;
}

/* Queues an immediate (next main loop iteration) error response. */
static void
deliver_error(const char *message, FeedApiCallback cb, gpointer user_data,
              GDestroyNotify notify)
{
  ErrorDelivery *delivery = g_new0(ErrorDelivery, 1);
  delivery->response.ok = FALSE;
  delivery->response.status = 0;
  delivery->response.error_message = g_strdup(message);
  delivery->cb = cb;
  delivery->user_data = user_data;
  delivery->notify = notify;
  g_idle_add(deliver_error_idle, delivery);
}

static void
on_request_done(GObject *source, GAsyncResult *result, gpointer user_data)
{
  RequestData *request = user_data;
  FeedApiResponse response = { 0 };
  GError *error = NULL;

  (void) source;

  GBytes *body = soup_session_send_and_read_finish(request->session, result, &error);
  if (body == NULL) {
    if (!g_error_matches(error, G_IO_ERROR, G_IO_ERROR_CANCELLED))
      response.error_message = g_strdup(error->message);
    response.status = soup_message_get_status(request->message);
    g_error_free(error);
  } else {
    response.status = soup_message_get_status(request->message);

    gsize length = 0;
    const char *data = g_bytes_get_data(body, &length);
    if (data != NULL && length > 0) {
      JsonParser *parser = json_parser_new();
      if (json_parser_load_from_data(parser, data, (gssize) length, NULL)) {
        JsonNode *root = json_parser_get_root(parser);
        if (root != NULL)
          response.root = json_node_ref(root);
      }
      g_object_unref(parser);
    }

    if (response.status < 200 || response.status >= 300) {
      if (response.root != NULL && JSON_NODE_HOLDS_OBJECT(response.root)) {
        JsonObject *object = json_node_get_object(response.root);
        const char *message = json_object_get_string_member(object, "error");
        if (message != NULL)
          response.error_message = g_strdup(message);
      }
      if (response.error_message == NULL)
        response.error_message = g_strdup_printf("Server returned HTTP %u",
                                                 response.status);
      g_clear_pointer(&response.root, json_node_unref);
    } else {
      response.ok = TRUE;
    }

    g_bytes_unref(body);
  }

  if (request->cb != NULL)
    request->cb(&response, request->user_data);

  feed_api_response_free(&response);
  g_object_unref(request->session);
  g_object_unref(request->message);
  if (request->cancellable != NULL)
    g_object_unref(request->cancellable);
  if (request->notify != NULL)
    request->notify(request->user_data);
  g_free(request);
}

static void
feed_api_request_full(FeedApi *self, const char *method, const char *path,
                      JsonNode *body, gboolean with_auth,
                      GCancellable *cancellable,
                      FeedApiCallback cb, gpointer user_data,
                      GDestroyNotify notify)
{
  g_return_if_fail(FEED_IS_API(self));
  g_return_if_fail(method != NULL);
  g_return_if_fail(path != NULL);

  if (self->server == NULL || self->server[0] == '\0') {
    deliver_error("No server configured", cb, user_data, notify);
    return;
  }

  g_autofree char *url = g_strconcat(self->server, path, NULL);
  SoupMessage *message = soup_message_new(method, url);
  if (message == NULL) {
    deliver_error("Invalid request URL", cb, user_data, notify);
    return;
  }

  if (with_auth && self->token != NULL && self->token[0] != '\0') {
    g_autofree char *auth = g_strconcat("Bearer ", self->token, NULL);
    soup_message_headers_append(soup_message_get_request_headers(message),
                                "Authorization", auth);
  }

  if (body != NULL) {
    g_autofree char *text = json_to_string(body, FALSE);
    if (text != NULL) {
      GBytes *bytes = g_bytes_new(text, strlen(text));
      soup_message_set_request_body_from_bytes(message, "application/json", bytes);
      g_bytes_unref(bytes);
    }
  }

  RequestData *request = g_new0(RequestData, 1);
  request->session = g_object_ref(self->session);
  request->message = g_object_ref(message);
  request->cancellable = cancellable != NULL ? g_object_ref(cancellable) : NULL;
  request->cb = cb;
  request->user_data = user_data;
  request->notify = notify;

  soup_session_send_and_read_async(self->session, message, G_PRIORITY_DEFAULT,
                                   cancellable, on_request_done, request);
  g_object_unref(message);
}

/* ---------- GObject boilerplate ---------- */

static void
feed_api_dispose(GObject *object)
{
  FeedApi *self = FEED_API(object);

  if (self->session != NULL) {
    soup_session_abort(self->session);
    g_clear_object(&self->session);
  }

  G_OBJECT_CLASS(feed_api_parent_class)->dispose(object);
}

static void
feed_api_finalize(GObject *object)
{
  FeedApi *self = FEED_API(object);

  g_free(self->server);
  g_free(self->token);

  G_OBJECT_CLASS(feed_api_parent_class)->finalize(object);
}

static void
feed_api_get_property(GObject *object, guint prop_id, GValue *value,
                      GParamSpec *pspec)
{
  FeedApi *self = FEED_API(object);

  switch (prop_id) {
  case PROP_SERVER:
    g_value_set_string(value, self->server);
    break;
  case PROP_TOKEN:
    g_value_set_string(value, self->token);
    break;
  default:
    G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    break;
  }
}

static void
feed_api_set_property(GObject *object, guint prop_id, const GValue *value,
                      GParamSpec *pspec)
{
  FeedApi *self = FEED_API(object);

  switch (prop_id) {
  case PROP_SERVER:
    feed_api_set_server(self, g_value_get_string(value));
    break;
  case PROP_TOKEN:
    feed_api_set_token(self, g_value_get_string(value));
    break;
  default:
    G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    break;
  }
}

static void
feed_api_class_init(FeedApiClass *klass)
{
  GObjectClass *gobject_class = G_OBJECT_CLASS(klass);

  gobject_class->dispose = feed_api_dispose;
  gobject_class->finalize = feed_api_finalize;
  gobject_class->get_property = feed_api_get_property;
  gobject_class->set_property = feed_api_set_property;

  props[PROP_SERVER] =
    g_param_spec_string("server", NULL, NULL, NULL,
                        G_PARAM_READWRITE | G_PARAM_CONSTRUCT | G_PARAM_STATIC_STRINGS);
  props[PROP_TOKEN] =
    g_param_spec_string("token", NULL, NULL, NULL,
                        G_PARAM_READWRITE | G_PARAM_CONSTRUCT | G_PARAM_STATIC_STRINGS);

  g_object_class_install_properties(gobject_class, N_PROPS, props);
}

static void
feed_api_init(FeedApi *self)
{
  self->session = soup_session_new();
  g_object_set(self->session, "user-agent", "feed-gtk4/0.1", NULL);
}

/* ---------- public API ---------- */

FeedApi *
feed_api_new(const char *server, const char *token)
{
  return g_object_new(FEED_TYPE_API, "server", server, "token", token, NULL);
}

void
feed_api_set_server(FeedApi *self, const char *server)
{
  g_return_if_fail(FEED_IS_API(self));

  char *normalized = normalize_server(server);
  g_free(self->server);
  self->server = normalized;
}

void
feed_api_set_token(FeedApi *self, const char *token)
{
  g_return_if_fail(FEED_IS_API(self));

  g_free(self->token);
  self->token = g_strdup(token != NULL ? token : "");
}

const char *
feed_api_get_server(FeedApi *self)
{
  g_return_val_if_fail(FEED_IS_API(self), NULL);
  return self->server;
}

const char *
feed_api_get_token(FeedApi *self)
{
  g_return_val_if_fail(FEED_IS_API(self), NULL);
  return self->token;
}

void
feed_api_get_feed(FeedApi *self, gint64 limit, gint64 offset,
                  GCancellable *cancellable,
                  FeedApiCallback cb, gpointer user_data,
                  GDestroyNotify notify)
{
  g_autofree char *path =
    g_strdup_printf("/api/feed?limit=%" G_GINT64_FORMAT "&offset=%" G_GINT64_FORMAT,
                    limit, offset);
  feed_api_request_full(self, "GET", path, NULL, TRUE, cancellable,
                        cb, user_data, notify);
}

void
feed_api_get_saved(FeedApi *self, GCancellable *cancellable,
                   FeedApiCallback cb, gpointer user_data,
                   GDestroyNotify notify)
{
  feed_api_request_full(self, "GET", "/api/saved", NULL, TRUE, cancellable,
                        cb, user_data, notify);
}

static void
feed_api_interaction(FeedApi *self, JsonObject *payload,
                     GCancellable *cancellable,
                     FeedApiCallback cb, gpointer user_data,
                     GDestroyNotify notify)
{
  JsonNode *node = json_node_new(JSON_NODE_OBJECT);
  json_node_take_object(node, payload);
  feed_api_request_full(self, "POST", "/api/interactions", node, TRUE,
                        cancellable, cb, user_data, notify);
  json_node_unref(node);
}

void
feed_api_vote(FeedApi *self, const char *key, gint64 value,
              GCancellable *cancellable,
              FeedApiCallback cb, gpointer user_data,
              GDestroyNotify notify)
{
  JsonObject *payload = json_object_new();
  json_object_set_string_member(payload, "key", key != NULL ? key : "");
  json_object_set_string_member(payload, "kind", "vote");
  json_object_set_int_member(payload, "value", value);
  feed_api_interaction(self, payload, cancellable, cb, user_data, notify);
}

void
feed_api_save(FeedApi *self, const char *key, gboolean saved,
              GCancellable *cancellable,
              FeedApiCallback cb, gpointer user_data,
              GDestroyNotify notify)
{
  JsonObject *payload = json_object_new();
  json_object_set_string_member(payload, "key", key != NULL ? key : "");
  json_object_set_string_member(payload, "kind", "save");
  json_object_set_boolean_member(payload, "value", saved);
  feed_api_interaction(self, payload, cancellable, cb, user_data, notify);
}

void
feed_api_seen(FeedApi *self, const char *key,
              GCancellable *cancellable,
              FeedApiCallback cb, gpointer user_data,
              GDestroyNotify notify)
{
  JsonObject *payload = json_object_new();
  json_object_set_string_member(payload, "key", key != NULL ? key : "");
  json_object_set_string_member(payload, "kind", "seen");
  json_object_set_boolean_member(payload, "value", TRUE);
  feed_api_interaction(self, payload, cancellable, cb, user_data, notify);
}

void
feed_api_get_subscriptions(FeedApi *self, GCancellable *cancellable,
                           FeedApiCallback cb, gpointer user_data,
                           GDestroyNotify notify)
{
  feed_api_request_full(self, "GET", "/api/subscriptions", NULL, TRUE,
                        cancellable, cb, user_data, notify);
}

void
feed_api_add_subscription(FeedApi *self, const char *url,
                          GCancellable *cancellable,
                          FeedApiCallback cb, gpointer user_data,
                          GDestroyNotify notify)
{
  JsonObject *payload = json_object_new();
  json_object_set_string_member(payload, "url", url != NULL ? url : "");
  JsonNode *node = json_node_new(JSON_NODE_OBJECT);
  json_node_take_object(node, payload);
  feed_api_request_full(self, "POST", "/api/subscriptions", node, TRUE,
                        cancellable, cb, user_data, notify);
  json_node_unref(node);
}

void
feed_api_delete_subscription(FeedApi *self, const char *id,
                             GCancellable *cancellable,
                             FeedApiCallback cb, gpointer user_data,
                             GDestroyNotify notify)
{
  g_autofree char *escaped = g_uri_escape_string(id != NULL ? id : "",
                                                 NULL, FALSE);
  g_autofree char *path = g_strdup_printf("/api/subscriptions/%s",
                                          escaped != NULL ? escaped : "");
  feed_api_request_full(self, "DELETE", path, NULL, TRUE,
                        cancellable, cb, user_data, notify);
}

void
feed_api_set_subscription_notify(FeedApi *self, const char *id,
                                 const char *policy,
                                 GCancellable *cancellable,
                                 FeedApiCallback cb, gpointer user_data,
                                 GDestroyNotify notify)
{
  g_autofree char *escaped = g_uri_escape_string(id != NULL ? id : "",
                                                 NULL, FALSE);
  g_autofree char *path = g_strdup_printf("/api/subscriptions/%s",
                                          escaped != NULL ? escaped : "");
  JsonObject *payload = json_object_new();
  json_object_set_string_member(payload, "notify", policy != NULL ? policy : "");
  JsonNode *node = json_node_new(JSON_NODE_OBJECT);
  json_node_take_object(node, payload);
  feed_api_request_full(self, "POST", path, node, TRUE,
                        cancellable, cb, user_data, notify);
  json_node_unref(node);
}

void
feed_api_refresh_all(FeedApi *self, GCancellable *cancellable,
                     FeedApiCallback cb, gpointer user_data,
                     GDestroyNotify notify)
{
  feed_api_request_full(self, "POST", "/api/refresh", NULL, TRUE,
                        cancellable, cb, user_data, notify);
}

void
feed_api_get_settings(FeedApi *self, GCancellable *cancellable,
                      FeedApiCallback cb, gpointer user_data,
                      GDestroyNotify notify)
{
  feed_api_request_full(self, "GET", "/api/settings", NULL, TRUE,
                        cancellable, cb, user_data, notify);
}

void
feed_api_post_settings(FeedApi *self, const char *memos_url,
                       const char *memos_token,
                       GCancellable *cancellable,
                       FeedApiCallback cb, gpointer user_data,
                       GDestroyNotify notify)
{
  JsonObject *payload = json_object_new();
  json_object_set_string_member(payload, "memosUrl",
                                memos_url != NULL ? memos_url : "");
  json_object_set_string_member(payload, "memosToken",
                                memos_token != NULL ? memos_token : "");
  JsonNode *node = json_node_new(JSON_NODE_OBJECT);
  json_node_take_object(node, payload);
  feed_api_request_full(self, "POST", "/api/settings", node, TRUE,
                        cancellable, cb, user_data, notify);
  json_node_unref(node);
}

void
feed_api_get_health(FeedApi *self, GCancellable *cancellable,
                    FeedApiCallback cb, gpointer user_data,
                    GDestroyNotify notify)
{
  feed_api_request_full(self, "GET", "/api/health", NULL, FALSE,
                        cancellable, cb, user_data, notify);
}
