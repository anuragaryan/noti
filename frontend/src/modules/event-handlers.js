import DOMRefs from '../dom-refs.js';
import State from '../state.js';
import NoteManager from './note-manager.js';
import FolderManager from './folder-manager.js';
import Preview from './preview.js';
import VoiceRecording from './voice-recording.js';
// Event listener setup and management
export default {
    setupEventListeners() {
        // Main action buttons
        DOMRefs.newNoteBtn.addEventListener('click', () => NoteManager.createNote());
        DOMRefs.newFolderBtn.addEventListener('click', () => FolderManager.createFolder());
        DOMRefs.saveBtn.addEventListener('click', () => NoteManager.saveNote());
        DOMRefs.deleteBtn.addEventListener('click', () => NoteManager.deleteNote());
        DOMRefs.previewBtn.addEventListener('click', () => Preview.togglePreview());
        DOMRefs.moveNoteBtn.addEventListener('click', () => NoteManager.showMoveNoteModal());
        
        // Voice recording listeners
        DOMRefs.voiceBtn.addEventListener('click', () => {
            if (State.isRecording) {
                VoiceRecording.stopVoiceRecording();
            } else {
                VoiceRecording.startVoiceRecording();
            }
        });
        
        DOMRefs.stopVoiceBtn.addEventListener('click', () => VoiceRecording.stopVoiceRecording());
        
        DOMRefs.closeSttSetupBtn.addEventListener('click', () => {
            DOMRefs.sttSetupModal.classList.remove('active');
        });
        
        // Folder modal
        DOMRefs.createFolderBtn.addEventListener('click', () => FolderManager.handleCreateFolder());
        DOMRefs.cancelFolderBtn.addEventListener('click', () => {
            DOMRefs.folderModal.classList.remove('active');
            DOMRefs.createFolderBtn.textContent = 'Create';
            DOMRefs.createFolderBtn.onclick = () => FolderManager.handleCreateFolder();
        });
        
        DOMRefs.folderNameInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                DOMRefs.createFolderBtn.click();
            }
        });
        
        // Move note modal
        DOMRefs.confirmMoveBtn.addEventListener('click', () => NoteManager.handleMoveNote());
        DOMRefs.cancelMoveBtn.addEventListener('click', () => {
            DOMRefs.moveNoteModal.classList.remove('active');
        });
        
        // Delete folder modal
        DOMRefs.confirmDeleteFolderBtn.addEventListener('click', () => FolderManager.handleDeleteFolder());
        DOMRefs.cancelDeleteFolderBtn.addEventListener('click', () => {
            DOMRefs.deleteFolderModal.classList.remove('active');
            State.folderToDelete = null;
        });
        
        // Close modals on background click
        [DOMRefs.folderModal, DOMRefs.moveNoteModal, DOMRefs.deleteFolderModal, DOMRefs.sttSetupModal].forEach(modal => {
            modal.addEventListener('click', (e) => {
                if (e.target === modal) {
                    modal.classList.remove('active');
                }
            });
        });
        
        // Auto-save
        let saveTimeout;
        DOMRefs.noteContent.addEventListener('input', () => {
            clearTimeout(saveTimeout);
            saveTimeout = setTimeout(() => {
                if (State.currentNote) {
                    NoteManager.saveNote();
                }
            }, 1000);
        });
        
        DOMRefs.noteTitle.addEventListener('input', () => {
            clearTimeout(saveTimeout);
            saveTimeout = setTimeout(() => {
                if (State.currentNote) {
                    NoteManager.saveNote();
                }
            }, 1000);
        });
        
        // Keyboard shortcuts
        document.addEventListener('keydown', (e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 's') {
                e.preventDefault();
                NoteManager.saveNote();
            }
            if ((e.ctrlKey || e.metaKey) && e.key === 'n') {
                e.preventDefault();
                NoteManager.createNote();
            }
            if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'N') {
                e.preventDefault();
                FolderManager.createFolder();
            }
            if ((e.ctrlKey || e.metaKey) && e.key === 'm') {
                e.preventDefault();
                if (State.currentNote) {
                    NoteManager.showMoveNoteModal();
                }
            }
            if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'V') {
                e.preventDefault();
                if (State.currentNote && State.sttAvailable) {
                    if (State.isRecording) {
                        VoiceRecording.stopVoiceRecording();
                    } else {
                        VoiceRecording.startVoiceRecording();
                    }
                }
            }
        });
    }
};