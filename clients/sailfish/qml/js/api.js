// feed. — Sailfish OS client: shared API helper.
//
// This is a .pragma library, so all pages share the same state
// (server/token config, seen-item dedupe, cached counts).
.pragma library

var server = "";
var token = "";

var seenSent = {};
var cachedItems = 0;

function setConfig(s, t) {
    server = s ? s : "";
    token = t ? t : "";
}

function getConfig() {
    return { server: server, token: token };
}

function baseUrl() {
    var s = server;
    if (s.charAt(s.length - 1) === "/") {
        s = s.slice(0, -1);
    }
    return s;
}

// request() issues a JSON API call. cb is called as cb(error, data, status);
// on success error is null.
function request(method, path, body, cb) {
    var xhr = new XMLHttpRequest();
    xhr.open(method, baseUrl() + path);
    xhr.setRequestHeader("Content-Type", "application/json");
    if (token !== "") {
        xhr.setRequestHeader("Authorization", "Bearer " + token);
    }
    xhr.timeout = 15000;
    xhr.onreadystatechange = function() {
        if (xhr.readyState !== XMLHttpRequest.DONE) return;
        var status = xhr.status;
        var data = null;
        try {
            data = JSON.parse(xhr.responseText || "null");
        } catch (e) {
            data = null;
        }
        if (status >= 200 && status < 300) {
            cb(null, data, status);
        } else {
            var msg = (data && data.error) ? data.error : ("HTTP " + status);
            cb(msg, data, status);
        }
    };
    xhr.onerror = function() {
        cb("Network error", null, 0);
    };
    xhr.ontimeout = function() {
        cb("Request timed out", null, 0);
    };
    if (body === null || body === undefined) {
        xhr.send();
    } else {
        xhr.send(JSON.stringify(body));
    }
}

function get(path, cb) { request("GET", path, null, cb); }
function post(path, body, cb) { request("POST", path, body, cb); }
function del(path, cb) { request("DELETE", path, null, cb); }

/* ---------- endpoint helpers ---------- */

function health(cb) { get("/api/health", cb); }

function feed(limit, offset, cb) {
    get("/api/feed?limit=" + limit + "&offset=" + offset, cb);
}

function saved(cb) { get("/api/saved", cb); }

function interactions(key, kind, value, cb) {
    post("/api/interactions", { key: key, kind: kind, value: value },
         cb ? cb : function() {});
}

function sendSeen(key) {
    if (seenSent[key]) return;
    seenSent[key] = true;
    interactions(key, "seen", true);
}

function subscriptions(cb) { get("/api/subscriptions", cb); }

function addSubscription(url, cb) {
    post("/api/subscriptions", { url: url }, cb);
}

function deleteSubscription(id, cb) {
    del("/api/subscriptions/" + encodeURIComponent(id), cb);
}

function setNotify(id, policy, cb) {
    post("/api/subscriptions/" + encodeURIComponent(id), { notify: policy }, cb);
}

function refresh(cb) { post("/api/refresh", {}, cb); }

function settings(cb) { get("/api/settings", cb); }

function saveSettings(memosUrl, memosToken, cb) {
    post("/api/settings", { memosUrl: memosUrl, memosToken: memosToken }, cb);
}

/* ---------- helpers ---------- */

function timeAgo(iso) {
    if (!iso) return "";
    var t = Date.parse(iso);
    if (isNaN(t)) return "";
    var diff = (Date.now() - t) / 1000;
    if (diff < 60) return "just now";
    if (diff < 3600) return Math.floor(diff / 60) + "m ago";
    if (diff < 86400) return Math.floor(diff / 3600) + "h ago";
    if (diff < 604800) return Math.floor(diff / 86400) + "d ago";
    return new Date(t).toLocaleDateString();
}
