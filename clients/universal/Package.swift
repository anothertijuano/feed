// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "Feed",
    platforms: [
        .macOS(.v14)
    ],
    products: [
        .library(name: "FeedCore", targets: ["FeedCore"]),
        .executable(name: "FeedApp", targets: ["FeedApp"]),
    ],
    targets: [
        .target(name: "FeedCore"),
        .executableTarget(
            name: "FeedApp",
            dependencies: ["FeedCore"]
        ),
    ]
)
