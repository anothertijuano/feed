import QtQuick 2.6
import Sailfish.Silica 1.0
import "../js/api.js" as Api

Page {
    id: feedPage
    property var app: null

    property int offset: 0
    property int total: -1
    property bool loading: false
    property string error: ""

    allowedOrientations: Orientation.All

    function loadMore() {
        if (loading) return;
        if (total >= 0 && offset >= total) return;
        loading = true;
        error = "";
        Api.feed(20, offset, function(err, data) {
            loading = false;
            if (err) {
                error = err;
                return;
            }
            total = data.total || 0;
            var items = data.items || [];
            for (var i = 0; i < items.length; i++) {
                var it = items[i];
                var mediaSrc = (it.media && it.media.length > 0 && it.media[0].src)
                        ? it.media[0].src : "";
                feedModel.append({
                    key: it.id,
                    title: it.title || "",
                    link: it.link || "",
                    source: it.sourceName || "",
                    time: Api.timeAgo(it.fetchedAt || it.publishedAt),
                    snippet: (it.paragraphs && it.paragraphs.length > 0) ? it.paragraphs[0] : "",
                    paragraphs: it.paragraphs || [],
                    media: it.media || [],
                    mediaSrc: mediaSrc,
                    vote: it.vote || 0,
                    saved: !!it.saved
                });
            }
            offset += items.length;
            Api.cachedItems = feedModel.count;
        });
    }

    function refreshAll() {
        error = "";
        Api.refresh(function(err, res) {
            if (err) {
                error = err;
                return;
            }
            var n = (res && res["new"] !== undefined) ? res["new"] : 0;
            feedModel.clear();
            offset = 0;
            total = -1;
            loadMore();
            if (n > 0) {
                error = "";
                refreshNotice.show(n);
            }
        });
    }

    Component.onCompleted: loadMore()

    RemorseItem { id: remorse }

    SilicaListView {
        id: list
        anchors.fill: parent
        model: ListModel { id: feedModel }
        spacing: Theme.paddingMedium

        header: Column {
            width: parent.width
            PageHeader { title: "feed." }
            Label {
                visible: feedPage.error !== ""
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                text: feedPage.error
                color: Theme.errorColor
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeSmall
            }
            Label {
                id: refreshNotice
                visible: false
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                color: Theme.highlightColor
                font.pixelSize: Theme.fontSizeSmall
                function show(n) {
                    text = n + " new item" + (n === 1 ? "" : "s") + " fetched";
                    visible = true;
                }
            }
            BusyIndicator {
                id: headerBusy
                anchors.horizontalCenter: parent.horizontalCenter
                running: feedPage.loading && feedModel.count === 0
                size: BusyIndicatorSize.Medium
            }
        }

        footer: BusyIndicator {
            anchors.horizontalCenter: parent.horizontalCenter
            running: feedPage.loading && feedModel.count > 0
            size: BusyIndicatorSize.Small
        }

        delegate: ListItem {
            id: card
            width: ListView.view.width
            contentHeight: col.height + Theme.paddingLarge * 2

            onClicked: pageStack.push(Qt.resolvedUrl("ItemPage.qml"), {
                app: feedPage.app,
                key: model.key,
                title: model.title,
                link: model.link,
                source: model.source,
                time: model.time,
                paragraphs: model.paragraphs,
                media: model.media,
                vote: model.vote,
                saved: model.saved
            })

            Component.onCompleted: Api.sendSeen(model.key)

            Column {
                id: col
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                spacing: Theme.paddingSmall

                Label {
                    text: model.source + (model.time !== "" ? " · " + model.time : "")
                    color: Theme.secondaryColor
                    font.pixelSize: Theme.fontSizeExtraSmall
                    elide: Text.ElideRight
                    width: parent.width
                }
                Label {
                    text: model.title
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Font.DemiBold
                    wrapMode: Text.Wrap
                    width: parent.width
                    maximumLineCount: 3
                    elide: Text.ElideRight
                }
                Image {
                    visible: model.mediaSrc !== ""
                    width: parent.width
                    height: Theme.itemSizeLarge
                    fillMode: Image.PreserveAspectCrop
                    source: model.mediaSrc
                    asynchronous: true
                    clip: true
                }
                Label {
                    visible: model.snippet !== ""
                    text: model.snippet
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.secondaryColor
                    wrapMode: Text.Wrap
                    width: parent.width
                    maximumLineCount: 2
                    elide: Text.ElideRight
                }
                Row {
                    spacing: Theme.paddingSmall

                    Button {
                        text: "▲"
                        highlighted: model.vote === 1
                        preferredWidth: Theme.itemSizeMedium
                        onClicked: {
                            var idx = index;
                            var key = model.key;
                            var val = (model.vote === 1) ? 0 : 1;
                            Api.interactions(key, "vote", val, function(err, res) {
                                if (!err && res) {
                                    feedModel.setProperty(idx, "vote", res.vote || 0);
                                }
                            });
                        }
                    }
                    Button {
                        text: "▼"
                        preferredWidth: Theme.itemSizeMedium
                        onClicked: {
                            var idx = index;
                            var key = model.key;
                            remorse.execute(card, "Removing…", function() {
                                Api.interactions(key, "vote", -1, function(err) {
                                    if (!err) feedModel.remove(idx);
                                });
                            });
                        }
                    }
                    Button {
                        text: model.saved ? "★" : "☆"
                        highlighted: model.saved
                        preferredWidth: Theme.itemSizeMedium
                        onClicked: {
                            var idx = index;
                            var key = model.key;
                            var target = !model.saved;
                            Api.interactions(key, "save", target, function(err) {
                                if (!err) feedModel.setProperty(idx, "saved", target);
                            });
                        }
                    }
                }
            }
        }

        VerticalScrollDecorator { }

        onAtYEndChanged: {
            if (list.atYEnd) feedPage.loadMore();
        }
    }

    ViewPlaceholder {
        enabled: feedModel.count === 0 && !feedPage.loading
        anchors.fill: list
        text: feedPage.error !== "" ? feedPage.error : "No content yet"
        hintText: feedPage.error !== "" ? "Pull down to refresh" : "Add subscriptions in the web app"
    }

    PullDownMenu {
        MenuItem {
            text: "Refresh"
            onClicked: feedPage.refreshAll()
        }
        MenuItem {
            text: "Subscriptions"
            onClicked: pageStack.push(Qt.resolvedUrl("SubsPage.qml"), { app: feedPage.app })
        }
        MenuItem {
            text: "Saved"
            onClicked: pageStack.push(Qt.resolvedUrl("SavedPage.qml"), { app: feedPage.app })
        }
        MenuItem {
            text: "Settings"
            onClicked: pageStack.push(Qt.resolvedUrl("SettingsPage.qml"), { app: feedPage.app })
        }
    }
}
