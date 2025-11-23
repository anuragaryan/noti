import { EventsOn } from '../wailsjs/runtime/runtime';
import './app.css';

import MarkdownParser from './markdown-parser.js';
import State from './state.js';
import DOMRefs from './dom-refs.js';
import VoiceRecording from './modules/voice-recording.js';
import FolderManager from './modules/folder-manager.js';
import NoteManager from './modules/note-manager.js';
import Preview from './modules/preview.js';
import EventHandlers from './modules/event-handlers.js';
import { promptManager } from './modules/prompt-manager.js';
import PromptUI from './modules/prompt-ui.js';


// Main application entry point
// This file orchestrates the initialization of all modules

// Initialize app
async function init() {
    setupDownloadListeners();
    await VoiceRecording.checkSTTStatus();
    await FolderManager.loadFolders();
    await NoteManager.loadNotes();
    FolderManager.renderFolderTree();
    EventHandlers.setupEventListeners();
    VoiceRecording.setupRealtimeTranscription(); // Setup real-time event listener
    
    // Initialize prompt system
    await promptManager.initialize();
    PromptUI.initialize();
}

// Start the app
window.addEventListener('DOMContentLoaded', init);

function setupDownloadListeners() {
    const notification = document.createElement('div');
    notification.className = 'download-notification';
    document.body.appendChild(notification);

    EventsOn('download:start', (modelName) => {
        notification.textContent = `Downloading model ${modelName}...`;
        notification.classList.add('show');
    });

    EventsOn('download:finish', () => {
        notification.textContent = 'Model download complete! Initializing...';
        // Hide the notification after a delay. The stt:ready event will handle the final UI update.
        setTimeout(() => {
            notification.classList.remove('show');
        }, 3000);
    });

    EventsOn('stt:ready', () => {
        console.log('STT service is ready, enabling UI.');
        VoiceRecording.checkSTTStatus();
    });

    EventsOn('download:error', (errorMsg) => {
        notification.textContent = `Error downloading model: ${errorMsg}`;
        notification.style.backgroundColor = '#e74c3c'; // Red for error
        setTimeout(() => {
            notification.classList.remove('show');
            notification.style.backgroundColor = '#3498db'; // Reset color
        }, 5000);
    });
}