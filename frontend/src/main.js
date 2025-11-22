// Main application entry point
// This file orchestrates the initialization of all modules

// Initialize app
async function init() {
    await VoiceRecording.checkSTTStatus();
    await FolderManager.loadFolders();
    await NoteManager.loadNotes();
    FolderManager.renderFolderTree();
    EventHandlers.setupEventListeners();
    VoiceRecording.setupRealtimeTranscription(); // Setup real-time event listener
}

// Start the app
window.addEventListener('DOMContentLoaded', init);