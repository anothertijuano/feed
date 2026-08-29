import QtQuick 2.6
import Sailfish.Silica 1.0
import Nemo.Configuration 1.0
import "js/api.js" as Api
import "pages"
import "cover"

ApplicationWindow {
    id: app

    allowedOrientations: Orientation.All
    _defaultPageOrientations: Orientation.All

    initialPage: Component { FeedPage { app: app } }
    cover: Component { CoverPage { } }

    // Configuration lives here (persisted via Nemo.Configuration, the
    // standard Sailfish settings backend) and is mirrored into the
    // shared api.js library so every page sees the same connection.
    ConfigurationGroup {
        id: cfg
        path: "/apps/harbour-feed"
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
