# feed. — Sailfish OS client (pure QML, no C++)
TARGET = harbour-feed

CONFIG += sailfishapp

DISTFILES += \
    qml/harbour-feed.qml \
    qml/pages/FeedPage.qml \
    qml/pages/ItemPage.qml \
    qml/pages/SavedPage.qml \
    qml/pages/SubsPage.qml \
    qml/pages/SettingsPage.qml \
    qml/cover/CoverPage.qml \
    qml/js/api.js \
    rpm/harbour-feed.spec \
    rpm/harbour-feed.yaml \
    harbour-feed.desktop

SAILFISHAPP_ICONS = 86x86 108x108 128x128 172x172 256x256
