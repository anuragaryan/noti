// system_capturer.m - ScreenCaptureKit audio capture implementation for macOS
#import "system_capturer.h"
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreMedia/CoreMedia.h>
#import <AudioToolbox/AudioToolbox.h>

// ScreenCaptureKit delegate for handling audio output
API_AVAILABLE(macos(13.0))
@interface SystemAudioCapturerDelegate : NSObject <SCStreamDelegate, SCStreamOutput>
@property (nonatomic, assign) AudioDataCallback audioCallback;
@property (nonatomic, strong) SCStream *stream;
@property (nonatomic, strong) SCContentFilter *filter;
@property (nonatomic, assign) BOOL isCapturing;
@property (nonatomic, assign) int targetSampleRate;
@property (nonatomic, assign) int targetChannels;
@property (nonatomic, strong) dispatch_queue_t audioQueue;
@end

@implementation SystemAudioCapturerDelegate

- (instancetype)init {
    self = [super init];
    if (self) {
        _audioQueue = dispatch_queue_create("com.noti.systemaudio", DISPATCH_QUEUE_SERIAL);
    }
    return self;
}

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer 
        ofType:(SCStreamOutputType)type {
    if (@available(macOS 13.0, *)) {
    } else {
        return;
    }

    // Only process audio samples
    if (type != SCStreamOutputTypeAudio) {
        return;
    }
    
    if (!self.audioCallback) {
        return;
    }
    
    // Get the audio buffer list from the sample buffer
    CMBlockBufferRef blockBuffer = CMSampleBufferGetDataBuffer(sampleBuffer);
    if (!blockBuffer) {
        NSLog(@"[SystemAudio] No block buffer in sample buffer");
        return;
    }
    
    // Get audio format description
    CMFormatDescriptionRef formatDesc = CMSampleBufferGetFormatDescription(sampleBuffer);
    if (!formatDesc) {
        NSLog(@"[SystemAudio] No format description");
        return;
    }
    
    const AudioStreamBasicDescription *asbd = CMAudioFormatDescriptionGetStreamBasicDescription(formatDesc);
    if (!asbd) {
        NSLog(@"[SystemAudio] No audio stream basic description");
        return;
    }
    
    // Get data pointer and length
    size_t totalLength = 0;
    char *dataPointer = NULL;
    OSStatus status = CMBlockBufferGetDataPointer(blockBuffer, 0, NULL, &totalLength, &dataPointer);
    
    if (status != kCMBlockBufferNoErr || !dataPointer || totalLength == 0) {
        NSLog(@"[SystemAudio] Failed to get data pointer: %d", (int)status);
        return;
    }
    
    // ScreenCaptureKit provides audio in float32 interleaved format.
    // totalLength is in bytes; divide by sizeof(float) to get total float values,
    // then divide by channel count to get the number of frames.
    int channelCount = (int)asbd->mChannelsPerFrame;
    if (channelCount <= 0) {
        NSLog(@"[SystemAudio] Invalid channel count: %d, skipping buffer", channelCount);
        return;
    }
    int totalSamples = (int)(totalLength / sizeof(float));
    if (totalSamples % channelCount != 0) {
        NSLog(@"[SystemAudio] Buffer size %d not evenly divisible by channel count %d, truncating",
              totalSamples, channelCount);
    }
    int frameCount = totalSamples / channelCount;

    // If stereo and we need mono, mix down to a mono buffer
    if (channelCount == 2 && self.targetChannels == 1) {
        float *stereoData = (float *)dataPointer;
        float *monoData = (float *)malloc(frameCount * sizeof(float));

        for (int i = 0; i < frameCount; i++) {
            monoData[i] = (stereoData[i * 2] + stereoData[i * 2 + 1]) / 2.0f;
        }

        // callback receives frameCount frames, 1 channel
        self.audioCallback(monoData, frameCount, (int)asbd->mSampleRate, 1);
        free(monoData);
    } else {
        // Pass interleaved buffer as-is; callback receives frameCount frames
        self.audioCallback((float *)dataPointer, frameCount,
                          (int)asbd->mSampleRate, channelCount);
    }
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
    if (error) {
        NSLog(@"[SystemAudio] Stream stopped with error: %@", error);
    } else {
        NSLog(@"[SystemAudio] Stream stopped normally");
    }
    self.isCapturing = NO;
}

@end

// Global instance
static SystemAudioCapturerDelegate *g_capturer API_AVAILABLE(macos(13.0)) = nil;

int SystemAudioCapturer_Initialize(void) {
    if (@available(macOS 13.0, *)) {
        if (!g_capturer) {
            g_capturer = [[SystemAudioCapturerDelegate alloc] init];
            NSLog(@"[SystemAudio] Capturer initialized");
        }
        return 0;
    }
    NSLog(@"[SystemAudio] System audio capture requires macOS 13.0 or later");
    return -1; // Not supported on this macOS version
}

SCPermissionStatus SystemAudioCapturer_CheckPermission(void) {
    if (@available(macOS 13.0, *)) {
        // ScreenCaptureKit uses screen recording permission
        // Check via CGPreflightScreenCaptureAccess (available in macOS 10.15+)
        if (CGPreflightScreenCaptureAccess()) {
            return SCPermissionStatusGranted;
        }
        return SCPermissionStatusDenied;
    }
    return SCPermissionStatusUnknown;
}

void SystemAudioCapturer_RequestPermission(void) {
    if (@available(macOS 10.15, *)) {
        // This will prompt the user for screen recording permission
        // and open System Preferences if needed
        CGRequestScreenCaptureAccess();
        NSLog(@"[SystemAudio] Permission requested");
    }
}

int SystemAudioCapturer_Start(int sampleRate, int channels, AudioDataCallback callback) {
    if (@available(macOS 13.0, *)) {
        if (!g_capturer) {
            NSLog(@"[SystemAudio] Capturer not initialized");
            return -1;
        }
        
        if (g_capturer.isCapturing) {
            NSLog(@"[SystemAudio] Already capturing");
            return -2;
        }
        
        g_capturer.audioCallback = callback;
        g_capturer.targetSampleRate = sampleRate;
        g_capturer.targetChannels = channels;
        
        // Get shareable content and create filter in one synchronous operation
        // This avoids ARC issues with objects being released across thread boundaries
        __block NSError *contentError = nil;
        __block SCContentFilter *capturedFilter = nil;
        __block CGDirectDisplayID displayID = 0;
        dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
        
        NSLog(@"[SystemAudio] Getting shareable content...");
        
        [SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable shareableContent, NSError * _Nullable error) {
            if (error) {
                contentError = error;
                NSLog(@"[SystemAudio] Error getting shareable content: %@", error);
                dispatch_semaphore_signal(semaphore);
                return;
            }
            
            if (!shareableContent) {
                NSLog(@"[SystemAudio] Shareable content is nil");
                dispatch_semaphore_signal(semaphore);
                return;
            }
            
            NSLog(@"[SystemAudio] Got shareable content: %lu displays, %lu windows, %lu apps",
                  (unsigned long)shareableContent.displays.count,
                  (unsigned long)shareableContent.windows.count,
                  (unsigned long)shareableContent.applications.count);
            
            // Get the display and create filter while still in the callback
            // where shareableContent and its objects are guaranteed to be valid
            if (shareableContent.displays.count > 0) {
                SCDisplay *display = shareableContent.displays.firstObject;
                if (display) {
                    displayID = display.displayID;
                    NSLog(@"[SystemAudio] Creating filter for display ID: %u", displayID);
                    
                    // Create the filter HERE while display is still valid
                    capturedFilter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
                    NSLog(@"[SystemAudio] Filter created: %@", capturedFilter);
                }
            } else {
                NSLog(@"[SystemAudio] No displays available");
            }
            
            dispatch_semaphore_signal(semaphore);
        }];
        
        // Wait for completion with timeout (30 seconds)
        dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC);
        long waitResult = dispatch_semaphore_wait(semaphore, timeout);
        
        if (waitResult != 0) {
            NSLog(@"[SystemAudio] Timeout waiting for shareable content");
            return -3;
        }
        
        if (contentError) {
            NSLog(@"[SystemAudio] Failed to get shareable content: %@", contentError);
            return -3;
        }
        
        if (!capturedFilter) {
            NSLog(@"[SystemAudio] No filter created - no display available");
            return -4;
        }
        
        NSLog(@"[SystemAudio] Using filter for display ID: %u", displayID);
        
        // Use the filter that was created in the callback
        g_capturer.filter = capturedFilter;
        
        // Configure stream for audio capture
        SCStreamConfiguration *config = [[SCStreamConfiguration alloc] init];
        
        // Enable audio capture
        config.capturesAudio = YES;
        config.excludesCurrentProcessAudio = YES;  // Don't capture our own app's audio
        
        // Set audio parameters
        config.sampleRate = sampleRate;
        config.channelCount = channels;
        
        // Minimize video capture since we only need audio
        // Set to smallest possible size
        config.width = 2;
        config.height = 2;
        config.minimumFrameInterval = CMTimeMake(1, 1);  // 1 FPS minimum
        config.showsCursor = NO;
        
        NSLog(@"[SystemAudio] Creating stream with sample rate: %d, channels: %d", sampleRate, channels);
        
        // Create the stream
        g_capturer.stream = [[SCStream alloc] initWithFilter:g_capturer.filter 
                                               configuration:config 
                                                    delegate:g_capturer];
        
        // Add audio output
        NSError *addOutputError = nil;
        BOOL added = [g_capturer.stream addStreamOutput:g_capturer 
                                                   type:SCStreamOutputTypeAudio 
                                     sampleHandlerQueue:g_capturer.audioQueue 
                                                  error:&addOutputError];
        
        if (!added || addOutputError) {
            NSLog(@"[SystemAudio] Failed to add stream output: %@", addOutputError);
            g_capturer.stream = nil;
            g_capturer.filter = nil;
            return -5;
        }
        
        // Start capture
        dispatch_semaphore_t startSemaphore = dispatch_semaphore_create(0);
        __block NSError *startError = nil;
        
        NSLog(@"[SystemAudio] Starting capture...");
        
        [g_capturer.stream startCaptureWithCompletionHandler:^(NSError *error) {
            startError = error;
            dispatch_semaphore_signal(startSemaphore);
        }];
        
        dispatch_semaphore_wait(startSemaphore, DISPATCH_TIME_FOREVER);
        
        if (startError) {
            NSLog(@"[SystemAudio] Failed to start capture: %@", startError);
            g_capturer.stream = nil;
            g_capturer.filter = nil;
            return -6;
        }
        
        g_capturer.isCapturing = YES;
        NSLog(@"[SystemAudio] ✓ System audio capture started successfully!");
        return 0;
    }
    
    NSLog(@"[SystemAudio] ScreenCaptureKit not available");
    return -1;
}

void SystemAudioCapturer_Stop(void) {
    if (@available(macOS 13.0, *)) {
        if (!g_capturer || !g_capturer.isCapturing) {
            return;
        }
        
        NSLog(@"[SystemAudio] Stopping capture...");
        
        dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
        
        [g_capturer.stream stopCaptureWithCompletionHandler:^(NSError *error) {
            if (error) {
                NSLog(@"[SystemAudio] Error stopping capture: %@", error);
            }
            dispatch_semaphore_signal(semaphore);
        }];
        
        dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);
        
        g_capturer.isCapturing = NO;
        g_capturer.stream = nil;
        g_capturer.filter = nil;
        g_capturer.audioCallback = NULL;
        
        NSLog(@"[SystemAudio] ✓ Capture stopped");
    }
}

int SystemAudioCapturer_IsCapturing(void) {
    if (g_capturer) {
        return g_capturer.isCapturing ? 1 : 0;
    }
    return 0;
}

void SystemAudioCapturer_Cleanup(void) {
    NSLog(@"[SystemAudio] Cleaning up...");
    SystemAudioCapturer_Stop();
    g_capturer = nil;
    NSLog(@"[SystemAudio] ✓ Cleanup complete");
}
