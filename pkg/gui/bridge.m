#import <Cocoa/Cocoa.h>
#import <ServiceManagement/ServiceManagement.h>
#import <CoreFoundation/CoreFoundation.h>
#import <SystemConfiguration/SystemConfiguration.h>
#include <stdint.h>
#include "bridge.h"

char* NSStringToCString(NSString* str) {
    if (str == nil) {
        return NULL;
    }
    const char* utf8String = [str UTF8String];
    if (utf8String == NULL) {
        return NULL;
    }
    char* result = (char*)malloc(strlen(utf8String) + 1);
    strcpy(result, utf8String);
    return result;
}

ProxyConfig* GetSystemProxyConfig(void) {
    ProxyConfig* config = (ProxyConfig*)malloc(sizeof(ProxyConfig));
    memset(config, 0, sizeof(ProxyConfig));
    
    CFDictionaryRef proxyDict = SCDynamicStoreCopyProxies(NULL);
    if (proxyDict == NULL) {
        return config;
    }
    
    NSDictionary* proxies = (__bridge NSDictionary*)proxyDict;
    
    // HTTP Proxy
    NSNumber* httpEnable = proxies[(__bridge NSString*)kSCPropNetProxiesHTTPEnable];
    if (httpEnable && [httpEnable boolValue]) {
        config->http_enabled = 1;
        
        NSString* httpHost = proxies[(__bridge NSString*)kSCPropNetProxiesHTTPProxy];
        if (httpHost) {
            config->http_host = NSStringToCString(httpHost);
        }
        
        NSNumber* httpPort = proxies[(__bridge NSString*)kSCPropNetProxiesHTTPPort];
        if (httpPort) {
            config->http_port = [httpPort intValue];
        }
    }
    
    // HTTPS Proxy
    NSNumber* httpsEnable = proxies[(__bridge NSString*)kSCPropNetProxiesHTTPSEnable];
    if (httpsEnable && [httpsEnable boolValue]) {
        config->https_enabled = 1;
        
        NSString* httpsHost = proxies[(__bridge NSString*)kSCPropNetProxiesHTTPSProxy];
        if (httpsHost) {
            config->https_host = NSStringToCString(httpsHost);
        }
        
        NSNumber* httpsPort = proxies[(__bridge NSString*)kSCPropNetProxiesHTTPSPort];
        if (httpsPort) {
            config->https_port = [httpsPort intValue];
        }
    }
    
    // SOCKS Proxy
    NSNumber* socksEnable = proxies[(__bridge NSString*)kSCPropNetProxiesSOCKSEnable];
    if (socksEnable && [socksEnable boolValue]) {
        config->socks_enabled = 1;
        
        NSString* socksHost = proxies[(__bridge NSString*)kSCPropNetProxiesSOCKSProxy];
        if (socksHost) {
            config->socks_host = NSStringToCString(socksHost);
        }
        
        NSNumber* socksPort = proxies[(__bridge NSString*)kSCPropNetProxiesSOCKSPort];
        if (socksPort) {
            config->socks_port = [socksPort intValue];
        }
    }
    
    CFRelease(proxyDict);
    
    return config;
}

void FreeProxyConfig(ProxyConfig* config) {
    if (config == NULL) {
        return;
    }
    
    if (config->http_host != NULL) {
        free(config->http_host);
    }
    if (config->https_host != NULL) {
        free(config->https_host);
    }
    if (config->socks_host != NULL) {
        free(config->socks_host);
    }
    
    free(config);
}

// The time interval in seconds for the menu update timer.
static const NSTimeInterval kMenuUpdateTimerInterval = 1.0;

// Callbacks exported from Go
extern void battMenuWillOpen(uintptr_t handle);
extern void battMenuDidClose(uintptr_t handle);
extern void battMenuTimerFired(uintptr_t handle);

@interface BattMenuObserver : NSObject
@property(nonatomic, assign) uintptr_t handle;
@property(nonatomic, strong) NSTimer *timer;
- (instancetype)initWithHandle:(uintptr_t)handle;
- (void)menuWillOpen:(NSNotification *)note;
- (void)menuDidClose:(NSNotification *)note;
- (void)timerTick:(NSTimer *)timer;
@end

@implementation BattMenuObserver
- (instancetype)initWithHandle:(uintptr_t)handle {
    if ((self = [super init])) {
        _handle = handle;
    }
    return self;
}
- (void)menuWillOpen:(NSNotification *)note {
    battMenuWillOpen(_handle);
    // Start a selector-based timer and add it to common modes so it fires during menu tracking.
    self.timer = [NSTimer timerWithTimeInterval:kMenuUpdateTimerInterval
                                         target:self
                                       selector:@selector(timerTick:)
                                       userInfo:nil
                                        repeats:YES];
    [[NSRunLoop mainRunLoop] addTimer:self.timer forMode:NSRunLoopCommonModes];
}
- (void)menuDidClose:(NSNotification *)note {
    if (self.timer) {
        [self.timer invalidate];
        self.timer = nil;
    }
    battMenuDidClose(_handle);
}
- (void)timerTick:(NSTimer *)timer {
    battMenuTimerFired(_handle);
}
@end

void *batt_attachMenuObserver(uintptr_t menuPtr, uintptr_t handle) {
    NSMenu *menu = (NSMenu *)menuPtr;
    BattMenuObserver *obs = [[BattMenuObserver alloc] initWithHandle:handle];
    NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
    [center addObserver:obs selector:@selector(menuWillOpen:) name:NSMenuDidBeginTrackingNotification object:menu];
    [center addObserver:obs selector:@selector(menuDidClose:) name:NSMenuDidEndTrackingNotification object:menu];
    return (void *)CFBridgingRetain(obs);
}

void batt_releaseMenuObserver(void *obsPtr) {
    if (obsPtr == NULL) return;
    BattMenuObserver *obs = (BattMenuObserver *)obsPtr;
    if (obs.timer) {
        [obs.timer invalidate];
        obs.timer = nil;
    }
    [[NSNotificationCenter defaultCenter] removeObserver:obs];
    CFRelease(obsPtr);
}

bool registerAppWithSMAppService(void) {
    if (@available(macOS 13.0, *)) {
        NSError *error = nil;
        SMAppService *service = [SMAppService mainAppService];
        BOOL success = [service registerAndReturnError:&error];
        if (!success && error) {
            NSLog(@"Failed to register login item: %@", error);
            return false;
        }
        return success;
    } else {
        NSLog(@"SMAppService not available on this macOS version");
        return false;
    }
}

bool unregisterAppWithSMAppService(void) {
    if (@available(macOS 13.0, *)) {
        NSError *error = nil;
        SMAppService *service = [SMAppService mainAppService];
        BOOL success = [service unregisterAndReturnError:&error];
        if (!success && error) {
            NSLog(@"Failed to unregister login item: %@", error);
            return false;
        }
        return success;
    } else {
        NSLog(@"SMAppService not available on this macOS version");
        return false;
    }
}

bool isRegisteredWithSMAppService(void) {
    if (@available(macOS 13.0, *)) {
        SMAppService *service = [SMAppService mainAppService];
        return [service status] == SMAppServiceStatusEnabled;
    }
    return false;
}
