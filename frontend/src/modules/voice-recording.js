// Voice recording and STT functionality
const VoiceRecording = {
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
                }
            }
        } catch (error) {
            console.error('Error checking STT status:', error);
            State.sttAvailable = false;
        }
    },

    async startVoiceRecording() {
        if (!State.sttAvailable) {
            this.showSTTSetupModal();
            return;
        }
        
        if (State.isRecording) return;
        
        try {
            await window.go.main.App.StartVoiceRecording();
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
        window.runtime.EventsOn("transcription:partial", (result) => {
            if (!result || !result.text || !result.text.trim()) {
                return;
            }
            
            // Ignore [BLANK_AUDIO] markers (sent when no audio detected in sample)
            if (result.text.trim() === '[BLANK_AUDIO]') {
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
            
            console.log('[Real-time] Inserted chunk:', text);
        });
    }
};