import AppKit
import CoreAudio
import Foundation

struct CAError: Error, CustomStringConvertible {
    let what: String
    let status: OSStatus
    var description: String { "\(what) failed (OSStatus \(status))" }
}

struct TapError: Error, CustomStringConvertible {
    let description: String
}

func check(_ status: OSStatus, _ what: String) throws {
    if status != noErr { throw CAError(what: what, status: status) }
}

func address(_ selector: AudioObjectPropertySelector) -> AudioObjectPropertyAddress {
    AudioObjectPropertyAddress(mSelector: selector,
                               mScope: kAudioObjectPropertyScopeGlobal,
                               mElement: kAudioObjectPropertyElementMain)
}

let systemObject = AudioObjectID(kAudioObjectSystemObject)

func getValue<T: BitwiseCopyable>(_ obj: AudioObjectID, _ selector: AudioObjectPropertySelector, _ initial: T) throws -> T {
    var addr = address(selector)
    var size = UInt32(MemoryLayout<T>.size)
    var value = initial
    try check(AudioObjectGetPropertyData(obj, &addr, 0, nil, &size, &value), "get property \(selector)")
    return value
}

func getString(_ obj: AudioObjectID, _ selector: AudioObjectPropertySelector) throws -> String {
    var addr = address(selector)
    var size = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
    var value: Unmanaged<CFString>?
    try check(AudioObjectGetPropertyData(obj, &addr, 0, nil, &size, &value), "get string \(selector)")
    return value?.takeRetainedValue() as String? ?? ""
}

func getObjectList(_ obj: AudioObjectID, _ selector: AudioObjectPropertySelector) throws -> [AudioObjectID] {
    var addr = address(selector)
    var size: UInt32 = 0
    try check(AudioObjectGetPropertyDataSize(obj, &addr, 0, nil, &size), "get size \(selector)")
    var list = [AudioObjectID](repeating: 0, count: Int(size) / MemoryLayout<AudioObjectID>.size)
    try check(AudioObjectGetPropertyData(obj, &addr, 0, nil, &size, &list), "get list \(selector)")
    return list
}

func processObject(pid: pid_t) throws -> AudioObjectID {
    var addr = address(kAudioHardwarePropertyTranslatePIDToProcessObject)
    var qualifier = pid
    var obj = AudioObjectID(kAudioObjectUnknown)
    var size = UInt32(MemoryLayout<AudioObjectID>.size)
    try check(AudioObjectGetPropertyData(systemObject, &addr, UInt32(MemoryLayout<pid_t>.size),
                                         &qualifier, &size, &obj), "translate pid \(pid)")
    if obj == kAudioObjectUnknown { throw TapError(description: "pid \(pid) has no audio process object") }
    return obj
}

struct AppInfo: Encodable {
    let pid: Int32
    let name: String
    let bundleID: String
}

func listAudioApps() throws -> [AppInfo] {
    var apps: [AppInfo] = []
    for p in try getObjectList(systemObject, kAudioHardwarePropertyProcessObjectList) {
        let running = (try? getValue(p, kAudioProcessPropertyIsRunningOutput, UInt32(0))) ?? 0
        guard running != 0 else { continue }
        let pid = try getValue(p, kAudioProcessPropertyPID, pid_t(0))
        let bundle = (try? getString(p, kAudioProcessPropertyBundleID)) ?? ""
        let name = NSRunningApplication(processIdentifier: pid)?.localizedName
            ?? executableName(pid) ?? "pid \(pid)"
        apps.append(AppInfo(pid: pid, name: name, bundleID: bundle))
    }
    return apps
}

// executableName is the fallback for processes without an app bundle (afplay, CLIs).
func executableName(_ pid: pid_t) -> String? {
    var buf = [CChar](repeating: 0, count: 4 * Int(MAXPATHLEN))
    guard proc_pidpath(pid, &buf, UInt32(buf.count)) > 0 else { return nil }
    return URL(fileURLWithPath: String(cString: buf)).lastPathComponent
}
