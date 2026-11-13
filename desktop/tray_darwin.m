#include <dispatch/dispatch.h>

extern void peregrineTrayStart(void);

// Creates the menu-bar item on the main thread, after Wails has started NSApp.
void peregrineRunTrayOnMain(void) {
  dispatch_async(dispatch_get_main_queue(), ^{
    peregrineTrayStart();
  });
}
