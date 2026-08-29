import QtQuick 2.6
import Sailfish.Silica 1.0
import "../js/api.js" as Api

Page {
    id: savedPage
    property var app: null

    property bool loading: false
    property string error: ""

    allowedOrientations: Orientation.All

    function load() {
        loading = true;
        error = "";
        Api.saved(function(err, data) {
            loading = false;
            if (err) {
                error = err;
                return;
            }
            savedModel.clear();
            var items = data.items || [];
            for (var i = 0; i < items.length; i++) {
                var it = items[i];
                savedModel.append({
                    key: it.id,
                    title: it.title || "",
                    link: it.link || "",
                    source: it.sourceName || "",
                    time: Api.timeAgo(it.fetchedAt || it.publishedAt),
                    paragraphs: it.paragraphs || [],
                    media: it.media || [],
                    saved: true
                });
            }
        });
    }

    Component.onCompleted: load()

    SilicaListView {
        id: list
        anchors.fill: parent
        model: ListModel { id: savedModel }
        spacing: Theme.paddingMedium

        header: Column {
            width: parent.width
            PageHeader { title: "Saved" }
            Label {
                visible: savedPage.error !== ""
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                text: savedPage.error
                color: Theme.errorColor
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeSmall
            }
            BusyIndicator {
                anchors.horizontalCenter: parent.horizontalCenter
                running: savedPage.loading
                size: BusyIndicatorSize.Small
            }
        }

        delegate: ListItem {
            width: ListView.view.width
            contentHeight: col.height + Theme.paddingLarge * 2

            onClicked: pageStack.push(Qt.resolvedUrl("ItemPage.qml"), {
                app: savedPage.app,
                key: model.key,
                title: model.title,
                link: model.link,
                source: model.source,
                time: model.time,
                paragraphs: model.paragraphs,
                media: model.media,
                vote: 0,
                saved: true
            })

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
                Label {
                    text: "★ saved"
                    color: Theme.highlightColor
                    font.pixelSize: Theme.fontSizeExtraSmall
                }
            }
        }

        VerticalScrollDecorator { }
    }

    ViewPlaceholder {
        enabled: savedModel.count === 0 && !savedPage.loading
        text: "Nothing saved yet"
        hintText: "Tap the ☆ on any item to save it"
    }
}
