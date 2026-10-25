//go:build desktop && cgo && darwin

// macOS window shim of the desktop shell (see cocoa_darwin.go for the
// design). Public AppKit/WebKit API only; deployment target macOS 13.
// Everything here runs on the main thread: the Go side calls these
// functions from webview_go Dispatch callbacks or from Open (main thread).

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

#include "cocoa_darwin.h"
#include "_cgo_export.h"

static NSWindow *gWindow;
static WKWebView *gWebView;
static id gNavDelegate;
static id gMenuTarget;
static const void *kDownloadPathKey = &kDownloadPathKey;

// ---- menu --------------------------------------------------------------------

@interface ArenileMenuTarget : NSObject
- (void)menuAction:(id)sender;
@end

@implementation ArenileMenuTarget
- (void)menuAction:(id)sender {
	NSString *action = [sender representedObject];
	if ([action isKindOfClass:[NSString class]]) {
		arenileShellMenuAction((char *)[action UTF8String]);
	}
}
@end

static NSMenu *buildMainMenu(const char *json) {
	NSData *data = [NSData dataWithBytes:json length:strlen(json)];
	NSArray *menus = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
	if (![menus isKindOfClass:[NSArray class]]) {
		return nil;
	}
	NSMenu *bar = [[NSMenu alloc] initWithTitle:@""];
	for (NSDictionary *m in menus) {
		NSString *role = m[@"role"];
		NSString *title = m[@"title"] ?: @"";
		NSMenu *sub = [[NSMenu alloc] initWithTitle:title];
		NSMenuItem *top = [[NSMenuItem alloc] initWithTitle:title action:nil keyEquivalent:@""];
		top.submenu = sub;
		[bar addItem:top];
		for (NSDictionary *e in m[@"entries"]) {
			if ([e[@"separator"] boolValue]) {
				[sub addItem:[NSMenuItem separatorItem]];
				continue;
			}
			NSString *label = e[@"label"] ?: @"";
			NSString *key = e[@"key"] ?: @"";
			NSString *selector = e[@"selector"];
			NSMenuItem *it;
			if (selector.length > 0) {
				// Native: sent to the first responder (the focused WKWebView
				// field for the edit actions, the window, NSApp).
				it = [[NSMenuItem alloc] initWithTitle:label action:NSSelectorFromString(selector) keyEquivalent:key];
				it.target = nil;
			} else {
				it = [[NSMenuItem alloc] initWithTitle:label action:@selector(menuAction:) keyEquivalent:key];
				it.target = gMenuTarget;
				it.representedObject = e[@"action"];
			}
			NSEventModifierFlags mods = key.length > 0 ? NSEventModifierFlagCommand : 0;
			if ([e[@"shift"] boolValue]) mods |= NSEventModifierFlagShift;
			if ([e[@"option"] boolValue]) mods |= NSEventModifierFlagOption;
			if ([e[@"control"] boolValue]) mods |= NSEventModifierFlagControl;
			it.keyEquivalentModifierMask = mods;
			[sub addItem:it];
		}
		if ([role isEqualToString:@"window"]) {
			[NSApp setWindowsMenu:sub];
		}
	}
	return bar;
}

// ---- added methods (webview_go's runtime classes) ------------------------------

// NSWindowDelegate windowShouldClose: the window is hidden (or the app
// quits, decided in Go) but never destroyed by the close button.
// The methods are added to webview_go's runtime classes, shared by every
// later webview window of the process (e.g. the remote pairing form): with
// no shell controller attached they keep the default behaviour.
static BOOL windowShouldClose(id self, SEL _cmd, id sender) {
	return arenileShellWindowShouldClose() ? NO : YES;
}

// NSApplicationDelegate: a click on the Dock icon shows the hidden window.
static BOOL applicationShouldHandleReopen(id self, SEL _cmd, id app, BOOL visible) {
	if (!visible) {
		(void)arenileShellReopen();
	}
	return YES;
}

// NSApplicationDelegate: Dock "Esci", logout, `osascript quit`: the
// launcher quits in order (exit backup, LAN stop) and the process exits.
// The quit is cancelled and run by the launcher (seconds): at logout macOS
// reports that Arenile interrupted it, and the next attempt succeeds.
static NSUInteger applicationShouldTerminate(id self, SEL _cmd, id app) {
	return arenileShellQuitRequested() ? NSTerminateCancel : NSTerminateNow;
}

// Link classes of desktop.ClassifyLink (arenileShellClassifyLink).
enum {
	kLinkOther = 0,
	kLinkPanelPage = 1,
	kLinkPanelResource = 2,
	kLinkLeaves = 3,
};

// classifyLink asks the Go rule (desktop.ClassifyLink, the single source
// of truth shared with the injected script) how url relates to the panel
// shown in webView. With no web page shown yet (first load) nothing is
// classified: the controller's own navigation proceeds.
static int classifyLink(WKWebView *webView, NSURL *url) {
	NSURL *current = webView.URL;
	if (url == nil || current == nil ||
	    !([current.scheme isEqualToString:@"http"] || [current.scheme isEqualToString:@"https"])) {
		return kLinkOther;
	}
	return arenileShellClassifyLink((char *)[current.absoluteString UTF8String], (char *)[url.absoluteString UTF8String]);
}

// WKUIDelegate: target=_blank and window.open (the injected script
// already handles most of them; this is the native net). A link that
// leaves the panel goes to the default browser (policy in Go); a panel
// page opens in place; an API URL of the panel (a server document) is saved as
// a download instead of replacing the panel with a page the operator
// cannot leave.
static WKWebView *createWebView(id self, SEL _cmd, WKWebView *webView, WKWebViewConfiguration *cfg,
                                WKNavigationAction *action, WKWindowFeatures *features) {
	NSURL *url = action.request.URL;
	if (url == nil) {
		return nil;
	}
	switch (classifyLink(webView, url)) {
	case kLinkPanelPage:
		[webView loadRequest:action.request];
		break;
	case kLinkPanelResource:
		[webView startDownloadUsingRequest:action.request
		                 completionHandler:^(WKDownload *download) {
			                 download.delegate = (id<WKDownloadDelegate>)gNavDelegate;
		                 }];
		break;
	default:
		// Leaves the panel, or not a web URL: Go refuses what is not a
		// web link (AllowExternal).
		arenileShellOpenExternal((char *)[url.absoluteString UTF8String]);
		break;
	}
	return nil;
}

static void addMethod(id object, SEL sel, IMP imp, const char *types) {
	if (object == nil) {
		return;
	}
	Class cls = object_getClass(object);
	if (!class_addMethod(cls, sel, imp, types)) {
		class_replaceMethod(cls, sel, imp, types);
	}
}

// ---- downloads ------------------------------------------------------------------

@interface ArenileNavigationDelegate : NSObject <WKNavigationDelegate, WKDownloadDelegate>
@end

@implementation ArenileNavigationDelegate

- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)action
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	// <a download> (blob: and data: URLs included) becomes a download.
	if (action.shouldPerformDownload) {
		decisionHandler(WKNavigationActionPolicyDownload);
		return;
	}
	// The window shows the panel only: a top-level navigation that leaves
	// it (another origin, or a same-origin page outside /admin such as the
	// public booking page; redirect, location.href, form post) goes to the
	// default browser (policy in Go) instead of loading with the window
	// bindings and no way back.
	if (action.targetFrame == nil || action.targetFrame.isMainFrame) {
		if (classifyLink(webView, action.request.URL) == kLinkLeaves) {
			arenileShellOpenExternal((char *)[action.request.URL.absoluteString UTF8String]);
			decisionHandler(WKNavigationActionPolicyCancel);
			return;
		}
	}
	decisionHandler(WKNavigationActionPolicyAllow);
}

- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationResponse:(WKNavigationResponse *)response
                      decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
	BOOL attachment = NO;
	if ([response.response isKindOfClass:[NSHTTPURLResponse class]]) {
		NSString *cd = [(NSHTTPURLResponse *)response.response valueForHTTPHeaderField:@"Content-Disposition"];
		attachment = cd != nil && [[cd lowercaseString] hasPrefix:@"attachment"];
	}
	// A top-level API response (an export or a PDF opened by URL) would
	// replace the panel with a page the operator cannot leave: saved instead.
	BOOL resource = response.isForMainFrame && classifyLink(webView, response.response.URL) == kLinkPanelResource;
	if (attachment || resource || !response.canShowMIMEType) {
		decisionHandler(WKNavigationResponsePolicyDownload);
		return;
	}
	decisionHandler(WKNavigationResponsePolicyAllow);
}

- (void)webView:(WKWebView *)webView navigationAction:(WKNavigationAction *)action didBecomeDownload:(WKDownload *)download {
	download.delegate = self;
}

- (void)webView:(WKWebView *)webView
    navigationResponse:(WKNavigationResponse *)response
     didBecomeDownload:(WKDownload *)download {
	download.delegate = self;
}

- (void)download:(WKDownload *)download
    decideDestinationUsingResponse:(NSURLResponse *)response
                 suggestedFilename:(NSString *)suggestedFilename
                 completionHandler:(void (^)(NSURL *))completionHandler {
	char *path = arenileShellDownloadPath((char *)[suggestedFilename UTF8String]);
	if (path == NULL) {
		completionHandler(nil);
		return;
	}
	NSString *p = [NSString stringWithUTF8String:path];
	free(path);
	objc_setAssociatedObject(download, kDownloadPathKey, p, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	completionHandler([NSURL fileURLWithPath:p]);
}

- (void)downloadDidFinish:(WKDownload *)download {
	NSString *p = objc_getAssociatedObject(download, kDownloadPathKey);
	if (p != nil) {
		// Bounce the Downloads stack in the Dock, as Safari does.
		[[NSDistributedNotificationCenter defaultCenter] postNotificationName:@"com.apple.DownloadFileFinished"
		                                                               object:p];
		arenileShellDownloadDone((char *)[p UTF8String], 0);
	}
}

- (void)download:(WKDownload *)download didFailWithError:(NSError *)error resumeData:(NSData *)resumeData {
	NSString *p = objc_getAssociatedObject(download, kDownloadPathKey);
	arenileShellDownloadDone((char *)[(p ?: @"download") UTF8String], 1);
}

@end

// ---- C interface ------------------------------------------------------------------

void arenile_shell_install(void *nswindow, const char *menu_json) {
	gWindow = (__bridge NSWindow *)nswindow;
	id content = gWindow.contentView;
	if ([content isKindOfClass:[WKWebView class]]) {
		gWebView = (WKWebView *)content;
	}
	gMenuTarget = [ArenileMenuTarget new];
	NSMenu *bar = buildMainMenu(menu_json);
	if (bar != nil) {
		[NSApp setMainMenu:bar];
	}
	addMethod(gWindow.delegate, @selector(windowShouldClose:), (IMP)windowShouldClose, "c@:@");
	addMethod(NSApp.delegate, @selector(applicationShouldHandleReopen:hasVisibleWindows:),
	          (IMP)applicationShouldHandleReopen, "c@:@c");
	addMethod(NSApp.delegate, @selector(applicationShouldTerminate:), (IMP)applicationShouldTerminate, "Q@:@");
	gWindow.releasedWhenClosed = NO;
	gWindow.collectionBehavior |= NSWindowCollectionBehaviorFullScreenPrimary;
	gWindow.tabbingMode = NSWindowTabbingModeDisallowed;
	if (gWebView != nil) {
		addMethod(gWebView.UIDelegate, @selector(webView:createWebViewWithConfiguration:forNavigationAction:windowFeatures:),
		          (IMP)createWebView, "@@:@@@@");
		gNavDelegate = [ArenileNavigationDelegate new];
		gWebView.navigationDelegate = gNavDelegate;
		gWebView.allowsMagnification = YES;
		gWebView.allowsBackForwardNavigationGestures = NO;
	}
	[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
}

void arenile_shell_uninstall(void) {
	if (gWebView != nil) {
		gWebView.navigationDelegate = nil;
	}
	gNavDelegate = nil;
	gWebView = nil;
	gWindow = nil;
}

void arenile_shell_show(void) {
	if (gWindow == nil) {
		return;
	}
	if (@available(macOS 14.0, *)) {
		[NSApp activate];
	} else {
		[NSApp activateIgnoringOtherApps:YES];
	}
	if (gWindow.miniaturized) {
		[gWindow deminiaturize:nil];
	}
	[gWindow makeKeyAndOrderFront:nil];
}

void arenile_shell_hide(void) {
	if (gWindow == nil) {
		return;
	}
	if (gWindow.styleMask & NSWindowStyleMaskFullScreen) {
		// orderOut of a full-screen window leaves an empty Space behind.
		[gWindow toggleFullScreen:nil];
	}
	[gWindow orderOut:nil];
}

void arenile_shell_set_zoom(double level) {
	if (gWebView != nil && level > 0) {
		gWebView.pageZoom = level;
	}
}

void arenile_shell_print(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gWebView == nil || gWindow == nil) {
			return;
		}
		NSPrintInfo *info = [[NSPrintInfo sharedPrintInfo] copy];
		info.horizontalPagination = NSPrintingPaginationModeFit;
		info.verticalPagination = NSPrintingPaginationModeAutomatic;
		info.verticallyCentered = NO;
		NSPrintOperation *op = [gWebView printOperationWithPrintInfo:info];
		op.showsPrintPanel = YES;
		op.showsProgressPanel = YES;
		// Without a frame the WKWebView print view renders blank pages.
		op.view.frame = gWebView.bounds;
		[op runOperationModalForWindow:gWindow delegate:nil didRunSelector:nil contextInfo:nil];
	});
}

int arenile_shell_get_frame(double *x, double *y, double *w, double *h, int *zoomed) {
	if (gWindow == nil) {
		return 0;
	}
	if (gWindow.styleMask & NSWindowStyleMaskFullScreen) {
		return 0; // keep the last windowed geometry
	}
	NSRect f = gWindow.frame;
	*x = f.origin.x;
	*y = f.origin.y;
	*w = f.size.width;
	*h = f.size.height;
	*zoomed = gWindow.zoomed ? 1 : 0;
	return 1;
}

void arenile_shell_set_frame(double x, double y, double w, double h) {
	if (gWindow == nil) {
		return;
	}
	NSRect f = NSMakeRect(x, y, w, h);
	// Only on a screen that still exists (a disconnected external monitor
	// would leave the window out of reach): otherwise keep the centred one.
	for (NSScreen *s in [NSScreen screens]) {
		NSRect v = s.visibleFrame;
		NSRect title = NSMakeRect(f.origin.x, NSMaxY(f) - 32, f.size.width, 32);
		if (NSIntersectsRect(v, title)) {
			[gWindow setFrame:f display:YES];
			return;
		}
	}
	[gWindow setContentSize:NSMakeSize(w, h)];
	[gWindow center];
}

void arenile_shell_set_zoomed(void) {
	if (gWindow != nil && !gWindow.zoomed) {
		[gWindow zoom:nil];
	}
}

void arenile_shell_set_accessory(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	});
}
