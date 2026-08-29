import QtQuick 2.6
import Sailfish.Silica 1.0
import "../js/api.js" as Api

CoverBackground {
    Label {
        anchors.centerIn: parent
        text: "feed."
        color: Theme.highlightColor
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Font.Bold
    }

    Label {
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Theme.paddingLarge
        visible: Api.cachedItems > 0
        text: Api.cachedItems + " items cached"
        color: Theme.secondaryColor
        font.pixelSize: Theme.fontSizeExtraSmall
    }
}
