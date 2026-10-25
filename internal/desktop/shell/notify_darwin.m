//go:build desktop && cgo && darwin

// macOS notification shim (see notify_darwin.go). Public UserNotifications
// API only; deployment target macOS 13. Notifications posted through
// UNUserNotificationCenter carry the name and icon of Arenile.app, are
// shown even while the app is frontmost and bring the panel back when
// clicked; the target page travels in the userInfo of the notification.

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

#include "notify_darwin.h"
#include "_cgo_export.h"

static NSString *const kTargetKey = @"target";

// ---- delegate -------------------------------------------------------------------

@interface ArenileNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation ArenileNotificationDelegate

// Without this macOS drops the notifications of the frontmost app: the
// operator at the panel would miss "Backup completato" or a new version.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
	completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList |
	                  UNNotificationPresentationOptionSound);
}

// A click (default action) shows the panel at the page of the
// notification (Go decides: window or browser, see SetNotificationHandler).
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))completionHandler {
	if ([response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
		id target = response.notification.request.content.userInfo[kTargetKey];
		NSString *t = [target isKindOfClass:[NSString class]] ? target : @"";
		arenileNotifyClicked((char *)[t UTF8String]);
	}
	completionHandler();
}

@end

// ---- center -----------------------------------------------------------------------

// center returns the notification center with the delegate installed, or
// nil when the process is not an app bundle: currentNotificationCenter
// raises an exception without a bundle identifier (development binary).
static UNUserNotificationCenter *center(void) {
	static UNUserNotificationCenter *c;
	static id delegate;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		if ([[NSBundle mainBundle] bundleIdentifier] == nil) {
			return;
		}
		@try {
			c = [UNUserNotificationCenter currentNotificationCenter];
			delegate = [ArenileNotificationDelegate new];
			c.delegate = delegate;
		} @catch (NSException *e) {
			c = nil;
		}
	});
	return c;
}

static void add(UNUserNotificationCenter *c, UNMutableNotificationContent *content, uintptr_t fallback) {
	UNNotificationRequest *req = [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
	                                                                  content:content
	                                                                  trigger:nil];
	[c addNotificationRequest:req
	    withCompletionHandler:^(NSError *error) {
		    if (error != nil) {
			    arenileNotifyFallback(fallback);
		    } else {
			    arenileNotifyDone(fallback);
		    }
	    }];
}

// ---- C interface ------------------------------------------------------------------

// arenile_notify_init installs the delegate at start-up (before NSApp
// runs): a click on a notification left in Notification Center after
// Arenile quit launches the app and is delivered to the delegate only if
// it exists by then. Nothing happens outside an app bundle.
void arenile_notify_init(void) { (void)center(); }

// arenile_notify_post returns 0 when there is no app bundle (the caller
// uses osascript); otherwise 1, and exactly one of arenileNotifyDone and
// arenileNotifyFallback is called later. The authorization (alert and
// sound) is asked lazily, the first time a notification is posted. If the
// user refuses or later turns notifications off in System Settings, the
// notification is dropped: going around that choice with osascript would
// ignore an explicit decision of the user.
int arenile_notify_post(const char *title, const char *body, const char *target, uintptr_t fallback) {
	UNUserNotificationCenter *c = center();
	if (c == nil) {
		return 0;
	}
	UNMutableNotificationContent *content = [UNMutableNotificationContent new];
	content.title = [NSString stringWithUTF8String:title] ?: @"Arenile";
	content.body = [NSString stringWithUTF8String:body] ?: @"";
	content.sound = [UNNotificationSound defaultSound];
	NSString *t = [NSString stringWithUTF8String:target];
	if (t.length > 0) {
		content.userInfo = @{kTargetKey : t};
	}
	[c getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
		switch (settings.authorizationStatus) {
		case UNAuthorizationStatusAuthorized:
		case UNAuthorizationStatusProvisional:
			add(c, content, fallback);
			break;
		case UNAuthorizationStatusNotDetermined: {
			[c requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
			                 completionHandler:^(BOOL granted, NSError *error) {
				                 if (granted) {
					                 add(c, content, fallback);
				                 } else {
					                 arenileNotifyDone(fallback);
				                 }
			                 }];
			break;
		}
		default: // denied (or a future status): respect the user's choice
			arenileNotifyDone(fallback);
			break;
		}
	}];
	return 1;
}
