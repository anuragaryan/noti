import './app.css';

import MarkdownParser from './markdown-parser.js';
import State from './state.js';
import DOMRefs from './dom-refs.js';
import VoiceRecording from './modules/voice-recording.js';
import FolderManager from './modules/folder-manager.js';
import NoteManager from './modules/note-manager.js';
import Preview from './modules/preview.js';
import EventHandlers from './modules/event-handlers.js';


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