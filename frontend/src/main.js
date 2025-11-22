import './app.css';

import MarkdownParser from './markdown-parser.js';
import State from './state.js';
import DOMRefs from './dom-refs.js';
import VoiceRecording from './modules/voice-recording.js';
import FolderManager from './modules/folder-manager.js';
import NoteManager from './modules/note-manager.js';
import Preview from './modules/preview.js';
import EventHandlers from './modules/event-handlers.js';

// Assign modules to the window object for legacy access.
// This is a temporary workaround. A better solution would be to
// refactor the code to not rely on global variables.
window.MarkdownParser = MarkdownParser;
window.State = State;
window.DOMRefs = DOMRefs;
window.VoiceRecording = VoiceRecording;
window.FolderManager = FolderManager;
window.NoteManager = NoteManager;
window.Preview = Preview;
window.EventHandlers = EventHandlers;

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