#ifndef BATT_BRIDGE_H
#define BATT_BRIDGE_H

#include <stdint.h>
#include <stdbool.h>

// Proxy configuration struct shared between Objective-C and Go (cgo)
typedef struct {
    char* http_host;
    int http_port;
    int http_enabled;
    char* https_host;
    int https_port;
    int https_enabled;
    char* socks_host;
    int socks_port;
    int socks_enabled;
} ProxyConfig;

// Proxy related functions
ProxyConfig* GetSystemProxyConfig(void);
void FreeProxyConfig(ProxyConfig* config);

// Login item (SMAppService) related functions
bool registerAppWithSMAppService(void);
bool unregisterAppWithSMAppService(void);
bool isRegisteredWithSMAppService(void);

// Menu observer (NSMenu tracking) bridge
void* batt_attachMenuObserver(uintptr_t menuPtr, uintptr_t handle);
void batt_releaseMenuObserver(void* obsPtr);

#endif // BATT_BRIDGE_H
