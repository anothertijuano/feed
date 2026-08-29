import QtQuick 2.6
import Sailfish.Silica 1.0
import "../js/api.js" as Api

Page {
    id: settingsPage
    property var app: null

    property string statusMsg: ""

    allowedOrientations: Orientation.All

    Component.onCompleted: {
        var cfg = Api.getConfig();
        serverField.text = cfg.server;
        tokenField.text = cfg.token;
    }

    SilicaFlickable {
        anchors.fill: parent
        contentHeight: col.height + Theme.paddingLarge

        Column {
            id: col
            width: parent.width
            spacing: Theme.paddingMedium

            PageHeader { title: "Settings" }

            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                text: "Connect to your feed. server. Tokens are created in the web app (Settings → Access tokens) or with feed -gen-token on the server."
                color: Theme.secondaryColor
                font.pixelSize: Theme.fontSizeExtraSmall
                wrapMode: Text.Wrap
            }

            SectionHeader { text: "Server" }

            TextField {
                id: serverField
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                label: "Server URL"
                placeholderText: "https://feed.example.com"
                inputMethodHints: Qt.ImhUrlCharactersOnly
                EnterKey.onClicked: tokenField.focus = true
            }

            TextField {
                id: tokenField
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                label: "Access token"
                placeholderText: "ft_…"
                echoMode: TextInput.Password
                EnterKey.onClicked: saveAll()
            }

            Button {
                x: Theme.horizontalPageMargin
                text: "Test connection"
                onClicked: {
                    Api.setConfig(serverField.text.trim(), tokenField.text.trim());
                    Api.health(function(err, data) {
                        statusMsg = err ? ("Connection failed: " + err) : "Connected — server is up";
                    });
                }
            }

            Button {
                x: Theme.horizontalPageMargin
                text: "Save"
                onClicked: saveAll()
            }

            Label {
                visible: statusMsg !== ""
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * Theme.horizontalPageMargin
                text: statusMsg
                color: statusMsg.indexOf("failed") >= 0 ? Theme.errorColor : Theme.highlightColor
                font.pixelSize: Theme.fontSizeSmall
                wrapMode: Text.Wrap
            }
        }
    }

    function saveAll() {
        var server = serverField.text.trim();
        var token = tokenField.text.trim();
        if (server === "") {
            statusMsg = "Connection failed: server URL is required";
            return;
        }
        Api.setConfig(server, token);
        if (app) app.saveConfig(server, token);
        statusMsg = "Saved";
    }
}
