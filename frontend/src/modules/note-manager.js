import State from '../state.js';
import DOMRefs from '../dom-refs.js';
import FolderManager from './folder-manager.js';
import Preview from './preview.js';
// Note management and operations
export default {
    async loadNotes() {
        try {
            State.allNotes = await window.go.main.App.GetAllNotes();
        } catch (error) {
            console.error('Error loading notes:', error);
            State.allNotes = [];
        }
    },

    async loadNote(id) {
        try {
            const note = await window.go.main.App.GetNote(id);
            State.currentNote = note;
            
            DOMRefs.noteTitle.value = note.title;
            DOMRefs.noteContent.value = note.content;
            
            DOMRefs.emptyState.style.display = 'none';
            DOMRefs.editorView.style.display = 'flex';
            
            if (State.isPreviewMode) {
                Preview.updatePreview();
            }
            
            FolderManager.updateBreadcrumb();
            FolderManager.renderFolderTree();
        } catch (error) {
            console.error('Error loading note:', error);
            alert('Failed to load note');
        }
    },

    async createNote() {
        try {
            const folderId = State.currentFolder ? State.currentFolder.id : '';
            
            const note = await window.go.main.App.CreateNote('Untitled', '', folderId);
            
            State.allNotes.push(note);
            await this.loadNote(note.id);
            FolderManager.renderFolderTree();
        } catch (error) {
            console.error('[FRONTEND] Error creating note:', error);
            console.error('[FRONTEND] Error stack:', error.stack);
            alert('Failed to create note: ' + error);
        }
    },

    async saveNote() {
        if (!State.currentNote) return;
        
        try {
            await window.go.main.App.UpdateNote(
                State.currentNote.id,
                DOMRefs.noteTitle.value,
                DOMRefs.noteContent.value
            );
            
            State.currentNote.title = DOMRefs.noteTitle.value;
            State.currentNote.content = DOMRefs.noteContent.value;
            
            const index = State.allNotes.findIndex(n => n.id === State.currentNote.id);
            if (index !== -1) {
                State.allNotes[index].title = DOMRefs.noteTitle.value;
                State.allNotes[index].updatedAt = new Date().toISOString();
            }
            
            FolderManager.renderFolderTree();
            
            DOMRefs.saveBtn.textContent = 'Saved!';
            setTimeout(() => {
                DOMRefs.saveBtn.textContent = 'Save';
            }, 1500);
        } catch (error) {
            console.error('Error saving note:', error);
            alert('Failed to save note');
        }
    },

    showDeleteNoteModal() {
        if (!State.currentNote) {
            console.error('[ERROR] Cannot delete: No note is currently selected');
            alert('Please select a note first');
            return;
        }
        DOMRefs.deleteNoteModal.classList.add('active');
    },

    async deleteNote() {
        if (!State.currentNote) return;
        
        try {
            await window.go.main.App.DeleteNote(State.currentNote.id);
            
            State.allNotes = State.allNotes.filter(n => n.id !== State.currentNote.id);
            
            State.currentNote = null;
            DOMRefs.noteTitle.value = '';
            DOMRefs.noteContent.value = '';
            
            DOMRefs.emptyState.style.display = 'flex';
            DOMRefs.editorView.style.display = 'none';
            
            DOMRefs.deleteNoteModal.classList.remove('active');
            FolderManager.renderFolderTree();
        } catch (error) {
            console.error('Error deleting note:', error);
            alert('Failed to delete note: ' + error);
            DOMRefs.deleteNoteModal.classList.remove('active');
        }
    },

    showMoveNoteModal() {
        if (!State.currentNote) return;
        
        State.selectedTargetFolder = null;
        this.renderFolderSelector();
        DOMRefs.moveNoteModal.classList.add('active');
    },

    renderFolderSelector() {
        DOMRefs.folderSelector.innerHTML = '';
        
        // Root option
        const rootOption = document.createElement('div');
        rootOption.className = 'folder-selector-item';
        if (State.selectedTargetFolder === null) {
            rootOption.classList.add('selected');
        }
        rootOption.innerHTML = '<span class="folder-icon">📁</span><span>All Notes (Root)</span>';
        rootOption.onclick = () => {
            document.querySelectorAll('.folder-selector-item').forEach(el => el.classList.remove('selected'));
            rootOption.classList.add('selected');
            State.selectedTargetFolder = null;
        };
        DOMRefs.folderSelector.appendChild(rootOption);
        
        // Folder tree
        const tree = FolderManager.buildFolderTree();
        this.renderFolderSelectorTree(tree, 0);
    },

    renderFolderSelectorTree(folders, level) {
        folders.forEach(folder => {
            const item = document.createElement('div');
            item.className = 'folder-selector-item';
            if (State.selectedTargetFolder === folder.id) {
                item.classList.add('selected');
            }
            
            const indent = document.createElement('span');
            indent.className = 'folder-selector-indent';
            indent.style.width = (level * 20) + 'px';
            indent.style.display = 'inline-block';
            
            const icon = document.createElement('span');
            icon.className = 'folder-icon';
            icon.textContent = '📁';
            
            const name = document.createElement('span');
            name.textContent = folder.name;
            
            item.appendChild(indent);
            item.appendChild(icon);
            item.appendChild(name);
            
            item.onclick = () => {
                document.querySelectorAll('.folder-selector-item').forEach(el => el.classList.remove('selected'));
                item.classList.add('selected');
                State.selectedTargetFolder = folder.id;
            };
            
            DOMRefs.folderSelector.appendChild(item);
            
            if (folder.children.length > 0) {
                this.renderFolderSelectorTree(folder.children, level + 1);
            }
        });
    },

    async handleMoveNote() {
        if (!State.currentNote) return;
        
        const targetFolderId = State.selectedTargetFolder === null ? '' : State.selectedTargetFolder;
        
        try {
            await window.go.main.App.MoveNote(State.currentNote.id, targetFolderId);
            
            const index = State.allNotes.findIndex(n => n.id === State.currentNote.id);
            if (index !== -1) {
                State.allNotes[index].folderId = targetFolderId;
            }
            State.currentNote.folderId = targetFolderId;
            
            DOMRefs.moveNoteModal.classList.remove('active');
            FolderManager.renderFolderTree();
            FolderManager.updateBreadcrumb();
        } catch (error) {
            console.error('Error moving note:', error);
            alert('Failed to move note: ' + error);
        }
    }
};