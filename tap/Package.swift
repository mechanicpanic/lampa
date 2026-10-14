// swift-tools-version:5.10
import Foundation
import PackageDescription

// Embed Info.plist so TCC can show NSAudioCaptureUsageDescription for a bare CLI.
let plist = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent()
    .appendingPathComponent("Sources/lampa-tap/Info.plist").path

let package = Package(
    name: "lampa-tap",
    platforms: [.macOS("14.4")],
    targets: [
        .executableTarget(
            name: "lampa-tap",
            path: "Sources/lampa-tap",
            exclude: ["Info.plist"],
            linkerSettings: [.unsafeFlags([
                "-Xlinker", "-sectcreate", "-Xlinker", "__TEXT",
                "-Xlinker", "__info_plist", "-Xlinker", plist,
            ])]
        ),
    ]
)
