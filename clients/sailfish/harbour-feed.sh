#!/bin/sh
# feed. launcher — runs the pure-QML app from its installed location.
exec sailfish-qml /usr/share/harbour-feed/qml/harbour-feed.qml >>/tmp/harbour-feed.log 2>&1
