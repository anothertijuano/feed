import QtQuick 2.6
import Sailfish.Silica 1.0
import "../js/api.js" as Api

Page {
    id: subsPage
    property var app: null

    property bool loading: false
    property string error: ""

    allowedOrientations: Orientation.All

    function load() {
        loading = true;
        error = "";
        Api.subscriptions(function(err, data) {
            loading = false;
            if (err) {
                error = err;
                return;
            }
            subsModel.clear();
            var items = data.items || [];
            for (var i = 0; i < items.length; i++) {
                var s = items[i];
                subsModel.append({
                    key: s.id,
                    title: s.title || "",
                    url: s.url || "",
                    notify: s.notify || "default",
                    count: s.itemCount !== undefined ? s.itemCount : -1
                });
            }
        });
    }

    Component.onCompleted: load()

    RemorseItem { id: remorse }

    SilicaListView {
        id: list
        anchors.fill: parent
        model: ListModel { id: subsModel }
        spacing: Theme.paddingMedium

        header: Column {
            width: parent.width
            PageHeader { title: "Subscriptions" }
            Label {
                visible: subsPage.error !== ""
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                text: subsPage.error
                color: Theme.errorColor
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeSmall
            }
            BusyIndicator {
                anchors.horizontalCenter: parent.horizontalCenter
                running: subsPage.loading
                size: BusyIndicatorSize.Small
            }
        }

        delegate: ListItem {
            id: subItem
            width: ListView.view.width
            contentHeight: col.height + Theme.paddingLarge * 2

            menu: ContextMenu {
                MenuItem {
                    text: "Notify: always"
                    onClicked: subsPage.setNotify(model.key, "always")
                }
                MenuItem {
                    text: "Notify: default"
                    onClicked: subsPage.setNotify(model.key, "default")
                }
                MenuItem {
                    text: "Notify: never"
                    onClicked: subsPage.setNotify(model.key, "never")
                }
                MenuItem {
                    text: "Remove"
                    onClicked: remorse.execute(subItem, "Removing…", function() {
                        Api.deleteSubscription(model.key, function(err) {
                            if (!err) subsModel.remove(index);
                        });
                    })
                }
            }

            Column {
                id: col
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                spacing: Theme.paddingSmall

                Label {
                    text: model.title
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Font.DemiBold
                    elide: Text.ElideRight
                    width: parent.width
                }
                Label {
                    text: model.url
                    color: Theme.secondaryColor
                    font.pixelSize: Theme.fontSizeExtraSmall
                    elide: Text.ElideMiddle
                    width: parent.width
                }
                Label {
                    text: subsPage.notifyLabel(model.notify)
                         + (model.count >= 0 ? " · " + model.count + " items" : "")
                    color: model.notify === "never" ? Theme.errorColor
                         : (model.notify === "always" ? Theme.highlightColor : Theme.secondaryColor)
                    font.pixelSize: Theme.fontSizeExtraSmall
                }
            }
        }

        VerticalScrollDecorator { }
    }

    ViewPlaceholder {
        enabled: subsModel.count === 0 && !subsPage.loading
        text: "No subscriptions"
        hintText: "Pull down and tap Add feed"
    }

    function notifyLabel(policy) {
        if (policy === "always") return "notify: always";
        if (policy === "never") return "notify: never";
        return "notify: default";
    }

    function setNotify(key, policy) {
        Api.setNotify(key, policy, function(err) {
            if (err) {
                error = err;
                return;
            }
            for (var i = 0; i < subsModel.count; i++) {
                if (subsModel.get(i).key === key) {
                    subsModel.setProperty(i, "notify", policy);
                    break;
                }
            }
        });
    }

    PullDownMenu {
        MenuItem {
            text: "Add feed"
            onClicked: {
                urlField.text = "";
                addDialog.open();
            }
        }
        MenuItem {
            text: "Refresh all"
            onClicked: {
                Api.refresh(function(err, res) {
                    if (err) {
                        error = err;
                    } else {
                        subsPage.load();
                    }
                });
            }
        }
    }

    Dialog {
        id: addDialog

        acceptDestination: subsPage
        canAccept: urlField.text.trim() !== ""

        Column {
            width: parent.width

            DialogHeader {
                title: "Add feed"
                acceptText: "Add"
            }

            TextField {
                id: urlField
                width: parent.width
                label: "Feed URL"
                placeholderText: "https://example.com/rss.xml"
                inputMethodHints: Qt.ImhUrlCharactersOnly
                EnterKey.onClicked: addDialog.accept()
            }
        }

        onAccepted: {
            var url = urlField.text.trim();
            Api.addSubscription(url, function(err) {
                if (err) {
                    subsPage.error = err;
                } else {
                    subsPage.error = "";
                    subsPage.load();
                }
            });
        }
    }
}
