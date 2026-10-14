import Darwin
import Foundation

func fail(_ msg: String, code: Int32 = 1) -> Never {
    fputs("lampa-tap: \(msg)\n", stderr)
    exit(code)
}

var signalSources: [DispatchSourceSignal] = []

let args = Array(CommandLine.arguments.dropFirst())
switch args.first {
case "list":
    do {
        let data = try JSONEncoder().encode(try listAudioApps())
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data("\n".utf8))
    } catch {
        fail("\(error)")
    }
case "capture":
    var pid: pid_t?
    if let i = args.firstIndex(of: "--pid") {
        guard i + 1 < args.count, let v = Int32(args[i + 1]) else { fail("--pid needs a number", code: 2) }
        pid = v
    }
    signal(SIGPIPE, SIG_IGN)
    let cap = Capture()
    for sig in [SIGINT, SIGTERM] {
        signal(sig, SIG_IGN)
        let src = DispatchSource.makeSignalSource(signal: sig, queue: .main)
        src.setEventHandler { cap.stopAndExit(0) }
        src.resume()
        signalSources.append(src)
    }
    do {
        try cap.start(pid: pid)
    } catch {
        cap.stop()
        if error is CAError {
            fail("\(error). If this is a permission error, allow audio recording for your terminal in System Settings → Privacy & Security → Screen & System Audio Recording.")
        }
        fail("\(error)")
    }
    dispatchMain()
default:
    fail("usage: lampa-tap capture [--pid N] | list", code: 2)
}
