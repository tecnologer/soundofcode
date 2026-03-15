// keyboard_darwin_tap.c — CGEventTap global keyboard capture for macOS.
// Only compiled on darwin (filename suffix convention).

#include "_cgo_export.h"
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

static CFMachPortRef gTap     = NULL;
static CFRunLoopRef  gRunLoop = NULL;

static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    (void)proxy; (void)refcon;
    // Re-enable the tap if the OS disabled it (e.g. took too long).
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (gTap) CGEventTapEnable(gTap, true);
        return event;
    }
    CGKeyCode keyCode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
    goKeyCallback((GoUint16)keyCode);
    return event;
}

// createEventTap creates the tap but does NOT start the run loop.
// Returns 1 on success, 0 if the tap could not be created (permission denied).
int createEventTap(void) {
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown);
    gTap = CGEventTapCreate(
        kCGHIDEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        tapCallback,
        NULL
    );
    return gTap != NULL ? 1 : 0;
}

// runEventTap attaches the tap to the current thread's run loop and blocks.
// Call from a dedicated goroutine/thread.
void runEventTap(void) {
    if (!gTap) return;
    gRunLoop = CFRunLoopGetCurrent();
    CFRunLoopSourceRef src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, gTap, 0);
    CFRunLoopAddSource(gRunLoop, src, kCFRunLoopCommonModes);
    CFRelease(src);
    CGEventTapEnable(gTap, true);
    CFRunLoopRun(); // blocks until stopEventTap() calls CFRunLoopStop
}

// stopEventTap stops the run loop and releases resources.
void stopEventTap(void) {
    if (gRunLoop) {
        CFRunLoopStop(gRunLoop);
        gRunLoop = NULL;
    }
    if (gTap) {
        CGEventTapEnable(gTap, false);
        CFRelease(gTap);
        gTap = NULL;
    }
}
