// system_capturer.h - ScreenCaptureKit audio capture for macOS
#ifndef SYSTEM_CAPTURER_H
#define SYSTEM_CAPTURER_H

#import <Foundation/Foundation.h>

// Callback type for audio data
// data: pointer to float32 audio samples (interleaved, frameCount * channels values)
// frameCount: number of frames (each frame contains one sample per channel)
// sampleRate: sample rate in Hz
// channels: number of audio channels
typedef void (*AudioDataCallback)(float *data, int frameCount, int sampleRate, int channels);

// Permission status enum
typedef NS_ENUM(NSInteger, SCPermissionStatus) {
    SCPermissionStatusUnknown = 0,
    SCPermissionStatusGranted = 1,
    SCPermissionStatusDenied = 2,
};

// Initialize the system audio capturer
// Returns 0 on success, negative value on error
int SystemAudioCapturer_Initialize(void);

// Check screen recording permission status
// Returns SCPermissionStatus value
SCPermissionStatus SystemAudioCapturer_CheckPermission(void);

// Request screen recording permission
// This will open System Preferences on macOS
void SystemAudioCapturer_RequestPermission(void);

// Start capturing system audio
// sampleRate: desired sample rate in Hz (e.g., 16000, 44100, 48000)
// channels: number of channels (1 for mono, 2 for stereo)
// callback: function to call when audio data is available;
//           callback receives frameCount (not total sample count)
// Returns 0 on success, negative value on error
int SystemAudioCapturer_Start(int sampleRate, int channels, AudioDataCallback callback);

// Stop capturing system audio
void SystemAudioCapturer_Stop(void);

// Check if currently capturing
// Returns 1 if capturing, 0 if not
int SystemAudioCapturer_IsCapturing(void);

// Cleanup resources
void SystemAudioCapturer_Cleanup(void);

#endif // SYSTEM_CAPTURER_H