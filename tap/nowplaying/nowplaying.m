// libnowplaying: prints the system Now Playing track as JSON.
//
// MediaRemote answers only Apple-signed processes since macOS 15.4, so this
// library is loaded into /usr/bin/perl (see internal/nowplaying) and called
// as a perl XSUB: lampa_nowplaying(perl interpreter, CV) — both ignored.
#import <Foundation/Foundation.h>
#include <dlfcn.h>

typedef void (*GetInfo)(dispatch_queue_t, void (^)(NSDictionary *));

void lampa_nowplaying(void *perl, void *cv) {
    void *mr = dlopen("/System/Library/PrivateFrameworks/MediaRemote.framework/MediaRemote", RTLD_NOW);
    GetInfo get = mr ? (GetInfo)dlsym(mr, "MRMediaRemoteGetNowPlayingInfo") : NULL;
    if (!get) {
        puts("{}");
        return;
    }
    NSDictionary *keys = @{
        @"kMRMediaRemoteNowPlayingInfoTitle": @"Title",
        @"kMRMediaRemoteNowPlayingInfoArtist": @"Artist",
    };
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    get(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^(NSDictionary *info) {
        NSMutableDictionary *out = [NSMutableDictionary dictionary];
        for (NSString *k in keys) {
            id v = info[k];
            if ([v isKindOfClass:NSString.class]) out[keys[k]] = v;
        }
        NSData *d = [NSJSONSerialization dataWithJSONObject:out options:0 error:nil];
        fwrite(d.bytes, 1, d.length, stdout);
        fputs("\n", stdout);
        fflush(stdout);
        dispatch_semaphore_signal(done);
    });
    if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC)) != 0) {
        puts("{}");
    }
}
