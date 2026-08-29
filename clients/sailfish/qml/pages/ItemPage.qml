import QtQuick 2.6
import Sailfish.Silica 1.0
import "../js/api.js" as Api

Page {
    id: itemPage
    property var app: null

    property string key: ""
    property string title: ""
    property string link: ""
    property string source: ""
    property string time: ""
    property var paragraphs: []
    property var media: []
    property int vote: 0
    property bool saved: false

    allowedOrientations: Orientation.All

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: col.height + Theme.paddingLarge

        Column {
            id: col
            width: parent.width
            spacing: Theme.paddingMedium

            PageHeader {
                title: itemPage.source !== "" ? itemPage.source : "feed."
            }

            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                text: itemPage.title
                font.pixelSize: Theme.fontSizeLarge
                font.weight: Font.Bold
                wrapMode: Text.Wrap
            }

            Label {
                x: Theme.horizontalPageMargin
                visible: itemPage.time !== ""
                text: itemPage.time
                color: Theme.secondaryColor
                font.pixelSize: Theme.fontSizeExtraSmall
            }

            Repeater {
                model: itemPage.media
                delegate: Image {
                    x: Theme.horizontalPageMargin
                    width: col.width - 2 * Theme.horizontalPageMargin
                    height: Theme.itemSizeLarge
                    fillMode: Image.PreserveAspectFit
                    source: modelData.src || ""
                    asynchronous: true
                    clip: true
                }
            }

            Repeater {
                model: itemPage.paragraphs
                delegate: Label {
                    x: Theme.horizontalPageMargin
                    width: col.width - 2 * Theme.horizontalPageMargin
                    text: modelData
                    font.pixelSize: Theme.fontSizeSmall
                    wrapMode: Text.Wrap
                    color: Theme.primaryColor
                }
            }

            Row {
                x: Theme.horizontalPageMargin
                spacing: Theme.paddingSmall

                Button {
                    text: "▲"
                    highlighted: itemPage.vote === 1
                    preferredWidth: Theme.itemSizeMedium
                    onClicked: {
                        var val = (itemPage.vote === 1) ? 0 : 1;
                        Api.interactions(itemPage.key, "vote", val, function(err, res) {
                            if (!err && res) itemPage.vote = res.vote || 0;
                        });
                    }
                }
                Button {
                    id: downBtn
                    text: "▼"
                    preferredWidth: Theme.itemSizeMedium
                    onClicked: remorsePopup.execute(downBtn, "Removing…", function() {
                        Api.interactions(itemPage.key, "vote", -1, function(err) {
                            if (!err) pageStack.pop();
                        });
                    })
                }
                Button {
                    text: itemPage.saved ? "★" : "☆"
                    highlighted: itemPage.saved
                    preferredWidth: Theme.itemSizeMedium
                    onClicked: {
                        var target = !itemPage.saved;
                        Api.interactions(itemPage.key, "save", target, function(err) {
                            if (!err) itemPage.saved = target;
                        });
                    }
                }
            }

            Button {
                visible: itemPage.link !== ""
                x: Theme.horizontalPageMargin
                text: "Open original"
                onClicked: Qt.openUrlExternally(itemPage.link)
            }
        }
    }

    RemorseItem { id: remorsePopup }

    VerticalScrollDecorator { }
}
