import QtQuick 2.6
import Sailfish.Silica 1.0
import Qt.labs.settings 1.0
import "js/api.js" as Api
import "pages"
import "cover"

ApplicationWindow {
    id: app

    allowedOrientations: Orientation.All
    _defaultPageOrientations: Orientation.All

    initialPage: Component { FeedPage { app: app } }
    cover: Component { CoverPage { } }

    // Configuration lives here (persisted) and is mirrored into the
    // shared api.js library so every page sees the same connection.
    Settings {
        id: cfg
        category: "harbour-feed"
        property string server: ""
        property string token: ""
    }

    Component.onCompleted: Api.setConfig(cfg.server, cfg.token)

    function saveConfig(server, token) {
        cfg.server = server;
        cfg.token = token;
        Api.setConfig(server, token);
    }
}
