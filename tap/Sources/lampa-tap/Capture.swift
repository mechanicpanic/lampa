import CoreAudio
import Darwin
import Foundation

final class Capture {
    private var tapID = AudioObjectID(kAudioObjectUnknown)
    private var aggID = AudioObjectID(kAudioObjectUnknown)
    private var procID: AudioDeviceIOProcID?
    private let queue = DispatchQueue(label: "lampa-tap.write")
    private var stopped = false

    func start(pid: pid_t?) throws {
        let desc: CATapDescription
        if let pid {
            desc = CATapDescription(stereoMixdownOfProcesses: [try processObject(pid: pid)])
        } else {
            desc = CATapDescription(stereoGlobalTapButExcludeProcesses: [])
        }
        desc.uuid = UUID()
        desc.name = "lampa-tap"
        desc.isPrivate = true
        desc.muteBehavior = .unmuted
        try check(AudioHardwareCreateProcessTap(desc, &tapID), "create process tap")

        let asbd = try getValue(tapID, kAudioTapPropertyFormat, AudioStreamBasicDescription())
        guard asbd.mFormatID == kAudioFormatLinearPCM,
              asbd.mFormatFlags & kAudioFormatFlagIsFloat != 0,
              asbd.mBitsPerChannel == 32 else {
            throw TapError(description: "unexpected tap format \(asbd)")
        }
        let nonInterleaved = asbd.mFormatFlags & kAudioFormatFlagIsNonInterleaved != 0

        let output = try getValue(systemObject, kAudioHardwarePropertyDefaultSystemOutputDevice, AudioDeviceID(0))
        let outputUID = try getString(output, kAudioDevicePropertyDeviceUID)
        let dict: [String: Any] = [
            kAudioAggregateDeviceNameKey: "lampa-tap",
            kAudioAggregateDeviceUIDKey: UUID().uuidString,
            kAudioAggregateDeviceMainSubDeviceKey: outputUID,
            kAudioAggregateDeviceIsPrivateKey: true,
            kAudioAggregateDeviceIsStackedKey: false,
            kAudioAggregateDeviceTapAutoStartKey: true,
            kAudioAggregateDeviceSubDeviceListKey: [[kAudioSubDeviceUIDKey: outputUID]],
            kAudioAggregateDeviceTapListKey: [[
                kAudioSubTapDriftCompensationKey: true,
                kAudioSubTapUIDKey: desc.uuid.uuidString,
            ]],
        ]
        try check(AudioHardwareCreateAggregateDevice(dict as CFDictionary, &aggID), "create aggregate device")

        var rate = UInt32(asbd.mSampleRate).littleEndian
        writeAll(Data(bytes: &rate, count: 4))

        try check(AudioDeviceCreateIOProcIDWithBlock(&procID, aggID, nil) { [weak self] _, input, _, _, _ in
            self?.handle(input, nonInterleaved: nonInterleaved)
        }, "create IO proc")
        try check(AudioDeviceStart(aggID, procID), "start aggregate device")

        // Output device changed (AirPods, HDMI…): exit so the Go side restarts us on the new device.
        var addr = address(kAudioHardwarePropertyDefaultSystemOutputDevice)
        AudioObjectAddPropertyListenerBlock(systemObject, &addr, queue) { [weak self] _, _ in
            fputs("lampa-tap: output device changed, exiting\n", stderr)
            self?.stopAndExit(0)
        }
    }

    private func handle(_ input: UnsafePointer<AudioBufferList>, nonInterleaved: Bool) {
        let abl = UnsafeMutableAudioBufferListPointer(UnsafeMutablePointer(mutating: input))
        guard let first = abl.first, first.mData != nil else { return }
        var mono: [Float]
        if nonInterleaved {
            let frames = Int(first.mDataByteSize) / 4
            mono = [Float](repeating: 0, count: frames)
            for buf in abl {
                guard let p = buf.mData?.assumingMemoryBound(to: Float.self) else { continue }
                for i in 0..<frames { mono[i] += p[i] }
            }
            let k = Float(abl.count)
            for i in 0..<frames { mono[i] /= k }
        } else {
            let ch = max(1, Int(first.mNumberChannels))
            let frames = Int(first.mDataByteSize) / 4 / ch
            let p = first.mData!.assumingMemoryBound(to: Float.self)
            mono = [Float](repeating: 0, count: frames)
            for i in 0..<frames {
                var s: Float = 0
                for c in 0..<ch { s += p[i * ch + c] }
                mono[i] = s / Float(ch)
            }
        }
        let data = mono.withUnsafeBufferPointer { Data(buffer: $0) }
        queue.async { [weak self] in self?.writeAll(data) }
    }

    private func writeAll(_ data: Data) {
        data.withUnsafeBytes { raw in
            var off = 0
            while off < raw.count {
                let n = Darwin.write(1, raw.baseAddress! + off, raw.count - off)
                if n < 0 && errno == EINTR { continue }
                if n <= 0 { stopAndExit(0) } // reader went away
                off += n
            }
        }
    }

    func stop() {
        guard !stopped else { return }
        stopped = true
        if let procID {
            AudioDeviceStop(aggID, procID)
            AudioDeviceDestroyIOProcID(aggID, procID)
        }
        if aggID != kAudioObjectUnknown { AudioHardwareDestroyAggregateDevice(aggID) }
        if tapID != kAudioObjectUnknown { AudioHardwareDestroyProcessTap(tapID) }
    }

    func stopAndExit(_ code: Int32) -> Never {
        stop()
        exit(code)
    }
}
