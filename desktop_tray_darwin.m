#import <Cocoa/Cocoa.h>

extern void DRPTrayToggleServer(void);
extern void DRPTrayShowMainWindow(void);
extern void DRPTrayQuit(void);

@interface DRPStatusItemController : NSObject
@property(nonatomic, strong) NSStatusItem *statusItem;
@property(nonatomic, strong) NSMenuItem *serverStatusItem;
@property(nonatomic, strong) NSMenuItem *serverToggleItem;
@end

@implementation DRPStatusItemController

- (instancetype)initWithIconData:(NSData *)iconData {
    self = [super init];
    if (self == nil) {
        return nil;
    }

    self.statusItem = [[NSStatusBar systemStatusBar]
        statusItemWithLength:NSSquareStatusItemLength];
    self.statusItem.button.toolTip = @"DKST RetroProxy";
    self.statusItem.button.accessibilityLabel = @"DKST RetroProxy";

    if (iconData.length > 0) {
        NSImage *image = [[NSImage alloc] initWithData:iconData];
        if (image != nil) {
            image.size = NSMakeSize(18.0, 18.0);
            image.template = YES;
            self.statusItem.button.image = image;
            self.statusItem.button.imagePosition = NSImageOnly;
        }
    }
    if (self.statusItem.button.image == nil) {
        self.statusItem.button.title = @"RP";
    }

    NSMenu *menu = [[NSMenu alloc] initWithTitle:@"DKST RetroProxy"];
    menu.autoenablesItems = NO;

    self.serverStatusItem = [[NSMenuItem alloc]
        initWithTitle:@"Server Status: Stopped" action:nil keyEquivalent:@""];
    self.serverStatusItem.enabled = NO;
    [menu addItem:self.serverStatusItem];

    self.serverToggleItem = [[NSMenuItem alloc]
        initWithTitle:@"Start Server"
               action:@selector(toggleServer:)
        keyEquivalent:@""];
    self.serverToggleItem.target = self;
    [menu addItem:self.serverToggleItem];

    NSMenuItem *showItem = [[NSMenuItem alloc]
        initWithTitle:@"Show Main Window"
               action:@selector(showMainWindow:)
        keyEquivalent:@""];
    showItem.target = self;
    [menu addItem:showItem];

    [menu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *quitItem = [[NSMenuItem alloc]
        initWithTitle:@"Quit" action:@selector(quit:) keyEquivalent:@""];
    quitItem.target = self;
    [menu addItem:quitItem];

    // Assigning NSStatusItem.menu lets AppKit perform native menu tracking on
    // left click. No Wails click callback or synthetic popup is involved.
    self.statusItem.menu = menu;
    return self;
}

- (void)toggleServer:(id)sender {
    (void)sender;
    DRPTrayToggleServer();
}

- (void)showMainWindow:(id)sender {
    (void)sender;
    DRPTrayShowMainWindow();
}

- (void)quit:(id)sender {
    (void)sender;
    DRPTrayQuit();
}

@end

static DRPStatusItemController *DRPStatusController = nil;
static NSString *DRPPendingStatusTitle = @"Server Status: Stopped";
static NSString *DRPPendingToggleTitle = @"Start Server";

static void DRPOnMainThread(dispatch_block_t block, BOOL wait) {
    if ([NSThread isMainThread]) {
        block();
        return;
    }
    if (wait) {
        dispatch_sync(dispatch_get_main_queue(), block);
    } else {
        dispatch_async(dispatch_get_main_queue(), block);
    }
}

void DRPInitStatusItem(const unsigned char *iconBytes, int iconLength) {
    NSData *iconData = nil;
    if (iconBytes != NULL && iconLength > 0) {
        iconData = [NSData dataWithBytes:iconBytes length:(NSUInteger)iconLength];
    }
    DRPOnMainThread(^{
        if (DRPStatusController != nil) {
            [[NSStatusBar systemStatusBar]
                removeStatusItem:DRPStatusController.statusItem];
        }
        DRPStatusController = [[DRPStatusItemController alloc]
            initWithIconData:iconData];
        DRPStatusController.serverStatusItem.title = DRPPendingStatusTitle;
        DRPStatusController.serverToggleItem.title = DRPPendingToggleTitle;
    }, YES);
}

void DRPUpdateStatusItem(const char *statusTitle, const char *toggleTitle) {
    NSString *status = statusTitle == NULL
        ? @"Server Status: Stopped"
        : [NSString stringWithUTF8String:statusTitle];
    NSString *toggle = toggleTitle == NULL
        ? @"Start Server"
        : [NSString stringWithUTF8String:toggleTitle];
    DRPPendingStatusTitle = [status copy];
    DRPPendingToggleTitle = [toggle copy];
    DRPOnMainThread(^{
        DRPStatusController.serverStatusItem.title = DRPPendingStatusTitle;
        DRPStatusController.serverToggleItem.title = DRPPendingToggleTitle;
    }, NO);
}

void DRPRemoveStatusItem(void) {
    DRPOnMainThread(^{
        if (DRPStatusController != nil) {
            [[NSStatusBar systemStatusBar]
                removeStatusItem:DRPStatusController.statusItem];
            DRPStatusController = nil;
        }
    }, YES);
}
