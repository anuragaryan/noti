import { EventsOn } from '../../wailsjs/runtime/runtime';
import State from '../state.js';
import DOMRefs from '../dom-refs.js';
import NoteManager from './note-manager.js';
// Voice recording and STT functionality
export default {
    async checkSTTStatus() {
        try {
            const status = await window.go.main.App.GetSTTStatus();
            State.sttAvailable = status.available;
            
            if (!State.sttAvailable && DOMRefs.modelPathEl) {
                DOMRefs.modelPathEl.textContent = status.modelPath;
            }
            
            // Update voice button visibility/state
            if (DOMRefs.voiceBtn) {
                DOMRefs.voiceBtn.disabled = !State.sttAvailable;
                if (!State.sttAvailable) {
                    DOMRefs.voiceBtn.title = 'Voice recording unavailable - model not installed';
                    DOMRefs.voiceBtn.style.opacity = '0.5';
                } else {
                    DOMRefs.voiceBtn.title = 'Start voice recording';
                    DOMRefs.voiceBtn.style.opacity = '1';
                }
            }
            
            // Load available audio sources
            await this.loadAudioSources();
        } catch (error) {
            console.error('Error checking STT status:', error);
            State.sttAvailable = false;
        }
    },

    async loadAudioSources() {
        try {
            const sources = await window.go.main.App.GetAudioSources();
            State.availableAudioSources = sources;
            
            // Update dropdown with available sources
            if (DOMRefs.audioSourceSelect) {
                // Clear existing options
                DOMRefs.audioSourceSelect.innerHTML = '';
                
                // Add options for each available source
                sources.forEach(source => {
                    const option = document.createElement('option');
                    option.value = source.id;
                    option.textContent = this.getSourceIcon(source.id) + ' ' + source.name;
                    DOMRefs.audioSourceSelect.appendChild(option);
                });
                
                // Get current source
                const currentSource = await window.go.main.App.GetCurrentAudioSource();
                DOMRefs.audioSourceSelect.value = currentSource;
                State.audioSource = currentSource;
                
                // Check permissions for current source
                await this.checkAudioPermissions(currentSource);
            }
        } catch (error) {
            console.error('Error loading audio sources:', error);
        }
    },

    getSourceIcon(sourceId) {
        switch (sourceId) {
            case 'microphone': return '🎤';
            case 'system': return '🔊';
            case 'mixed': return '🎛️';
            default: return '🎤';
        }
    },

    async checkAudioPermissions(source) {
        try {
            const result = await window.go.main.App.CheckAudioPermissions(source);
            State.audioPermissions[source] = result;
            
            // Update UI based on permission status
            if (DOMRefs.audioPermissionStatus) {
                if (result.granted) {
                    DOMRefs.audioPermissionStatus.textContent = '✓ Ready';
                    DOMRefs.audioPermissionStatus.className = 'audio-permission-status granted';
                    if (DOMRefs.audioPermissionBtn) {
                        DOMRefs.audioPermissionBtn.style.display = 'none';
                    }
                } else if (result.status === 'denied') {
                    DOMRefs.audioPermissionStatus.textContent = '✗ Permission denied';
                    DOMRefs.audioPermissionStatus.className = 'audio-permission-status denied';
                    if (DOMRefs.audioPermissionBtn) {
                        DOMRefs.audioPermissionBtn.style.display = 'inline-block';
                        DOMRefs.audioPermissionBtn.textContent = 'Open Settings';
                    }
                } else {
                    DOMRefs.audioPermissionStatus.textContent = '? Permission required';
                    DOMRefs.audioPermissionStatus.className = 'audio-permission-status unknown';
                    if (DOMRefs.audioPermissionBtn) {
                        DOMRefs.audioPermissionBtn.style.display = 'inline-block';
                        DOMRefs.audioPermissionBtn.textContent = 'Grant Permission';
                    }
                }
            }
        } catch (error) {
            console.error('Error checking audio permissions:', error);
        }
    },

    async requestAudioPermissions() {
        try {
            await window.go.main.App.RequestAudioPermissions(State.audioSource);
            // Re-check permissions after request
            await this.checkAudioPermissions(State.audioSource);
        } catch (error) {
            console.error('Error requesting audio permissions:', error);
            alert('Failed to request permissions. Please grant access in System Preferences.');
        }
    },

    async setAudioSource(source) {
        try {
            await window.go.main.App.SetAudioSource(source);
            State.audioSource = source;
            await this.checkAudioPermissions(source);
        } catch (error) {
            console.error('Error setting audio source:', error);
            alert('Failed to set audio source: ' + error);
        }
    },

    setupAudioSourceListeners() {
        // Audio source dropdown change
        if (DOMRefs.audioSourceSelect) {
            DOMRefs.audioSourceSelect.addEventListener('change', async (e) => {
                await this.setAudioSource(e.target.value);
            });
        }
        
        // Permission button click
        if (DOMRefs.audioPermissionBtn) {
            DOMRefs.audioPermissionBtn.addEventListener('click', async () => {
                await this.requestAudioPermissions();
            });
        }
    },

    async startVoiceRecording() {
        if (!State.sttAvailable) {
            this.showSTTSetupModal();
            return;
        }
        
        if (State.isRecording) return;
        
        // Check permissions before starting
        const permResult = State.audioPermissions[State.audioSource];
        if (permResult && !permResult.granted && permResult.status !== 'unknown') {
            alert('Please grant audio permission before recording.');
            return;
        }
        
        try {
            // Use the new source-aware recording method
            await window.go.main.App.StartVoiceRecordingWithSource(State.audioSource);
            State.isRecording = true;
            State.recordingStartTime = Date.now();
            
            // Reset last insert position to current cursor position
            State.lastInsertPosition = DOMRefs.noteContent.selectionStart;
            
            // Update UI
            DOMRefs.voiceBtn.classList.add('recording');
            DOMRefs.voiceBtn.querySelector('.voice-text').textContent = 'Recording...';
            DOMRefs.voiceStatusBar.style.display = 'flex';
            
            // Start timer
            this.updateRecordingTimer();
            State.recordingTimer = setInterval(() => this.updateRecordingTimer(), 1000);
            
        } catch (error) {
            console.error('Error starting recording:', error);
            alert('Failed to start recording: ' + error);
            State.isRecording = false;
        }
    },

    async stopVoiceRecording() {
        if (!State.isRecording) return;
        
        try {
            // Clear timer
            if (State.recordingTimer) {
                clearInterval(State.recordingTimer);
                State.recordingTimer = null;
            }
            
            // Update UI to show transcription in progress
            DOMRefs.voiceBtn.classList.remove('recording');
            DOMRefs.voiceBtn.querySelector('.voice-text').textContent = 'Voice';
            DOMRefs.voiceStatusBar.style.display = 'none';
            DOMRefs.transcriptionStatus.style.display = 'block';
            
            State.isRecording = false;
            
            // Stop recording and get transcription
            const result = await window.go.main.App.StopVoiceRecording();
            
            // Hide transcription status
            DOMRefs.transcriptionStatus.style.display = 'none';
            
            // Insert final transcribed text chunk (if any)
            // Note: Real-time chunks are already inserted via the event listener
            if (result && result.text && result.text.trim()) {
                // Use last insert position if available, otherwise current cursor
                const insertPos = State.lastInsertPosition !== null
                    ? State.lastInsertPosition
                    : DOMRefs.noteContent.selectionStart;
                
                const textBefore = DOMRefs.noteContent.value.substring(0, insertPos);
                const textAfter = DOMRefs.noteContent.value.substring(insertPos);
                
                // Add spacing if needed
                let prefix = '';
                let suffix = '';
                
                if (textBefore && !textBefore.endsWith(' ') && !textBefore.endsWith('\n')) {
                    prefix = ' ';
                }
                if (textAfter && !textAfter.startsWith(' ') && !textAfter.startsWith('\n')) {
                    suffix = ' ';
                }
                
                DOMRefs.noteContent.value = textBefore + prefix + result.text.trim() + suffix + textAfter;
                
                // Move cursor to end of inserted text
                const newCursorPos = insertPos + prefix.length + result.text.trim().length + suffix.length;
                DOMRefs.noteContent.setSelectionRange(newCursorPos, newCursorPos);
                DOMRefs.noteContent.focus();
                
                // Auto-save
                if (State.currentNote) {
                    NoteManager.saveNote();
                }
            }
            
            // Reset last insert position
            State.lastInsertPosition = null;
            
        } catch (error) {
            console.error('Error stopping recording:', error);
            alert('Transcription failed: ' + error);
            DOMRefs.transcriptionStatus.style.display = 'none';
            State.isRecording = false;
            State.lastInsertPosition = null;
        }
    },

    updateRecordingTimer() {
        if (!State.recordingStartTime) return;
        
        const elapsed = Math.floor((Date.now() - State.recordingStartTime) / 1000);
        const minutes = Math.floor(elapsed / 60);
        const seconds = elapsed % 60;
        
        DOMRefs.voiceTimer.textContent = `${minutes}:${seconds.toString().padStart(2, '0')}`;
    },

    showSTTSetupModal() {
        DOMRefs.sttSetupModal.classList.add('active');
    },

    setupRealtimeTranscription() {
        // Listen for real-time transcription chunks from the backend
        EventsOn("transcription:partial", (result) => {
            if (!result || !result.text || !result.text.trim()) {
                return;
            }
            
            // Only insert if we're currently recording and have a note open
            if (!State.isRecording || !State.currentNote) {
                return;
            }
            
            const text = result.text.trim() + ' ';
            
            // Get insertion position (use last position or current cursor)
            const insertPos = State.lastInsertPosition !== null
                ? State.lastInsertPosition
                : DOMRefs.noteContent.selectionStart;
            
            const textBefore = DOMRefs.noteContent.value.substring(0, insertPos);
            const textAfter = DOMRefs.noteContent.value.substring(insertPos);
            
            // Insert the transcribed text
            DOMRefs.noteContent.value = textBefore + text + textAfter;
            
            // Update last insert position for next chunk
            State.lastInsertPosition = insertPos + text.length;
            
            // Move cursor to end of inserted text
            DOMRefs.noteContent.setSelectionRange(State.lastInsertPosition, State.lastInsertPosition);
            DOMRefs.noteContent.focus();
        });
    },

    // Initialize all audio-related functionality
    initializeAudio() {
        this.setupAudioSourceListeners();
    }
};