// Folder management and tree rendering
const FolderManager = {
    async loadFolders() {
        try {
            State.allFolders = await window.go.main.App.GetAllFolders();
        } catch (error) {
            console.error('Error loading folders:', error);
            State.allFolders = [];
        }
    },

    async createFolder() {
        DOMRefs.folderModalTitle.textContent = 'New Folder';
        DOMRefs.folderNameInput.value = '';
        DOMRefs.folderModal.classList.add('active');
        DOMRefs.folderNameInput.focus();
    },

    async handleCreateFolder() {
        const name = DOMRefs.folderNameInput.value.trim();
        if (!name) return;
        
        try {
            const parentId = State.currentFolder ? State.currentFolder.id : '';
            const folder = await window.go.main.App.CreateFolder(name, parentId);
            State.allFolders.push(folder);
            State.expandedFolders.add(parentId);
            DOMRefs.folderModal.classList.remove('active');
            this.renderFolderTree();
        } catch (error) {
            console.error('Error creating folder:', error);
            alert('Failed to create folder: ' + error);
        }
    },

    async renameFolder(folderId) {
        const folder = State.allFolders.find(f => f.id === folderId);
        if (!folder) return;
        
        DOMRefs.folderModalTitle.textContent = 'Rename Folder';
        DOMRefs.folderNameInput.value = folder.name;
        DOMRefs.folderModal.classList.add('active');
        DOMRefs.folderNameInput.focus();
        
        // Change button behavior temporarily
        const originalHandler = DOMRefs.createFolderBtn.onclick;
        DOMRefs.createFolderBtn.textContent = 'Rename';
        DOMRefs.createFolderBtn.onclick = async () => {
            const newName = DOMRefs.folderNameInput.value.trim();
            if (!newName) return;
            
            try {
                await window.go.main.App.UpdateFolder(folderId, newName, folder.parentId);
                folder.name = newName;
                DOMRefs.folderModal.classList.remove('active');
                this.renderFolderTree();
                
                // Restore original button
                DOMRefs.createFolderBtn.textContent = 'Create';
                DOMRefs.createFolderBtn.onclick = originalHandler;
            } catch (error) {
                console.error('Error renaming folder:', error);
                alert('Failed to rename folder: ' + error);
            }
        };
    },

    async showDeleteFolderModal(folderId) {
        State.folderToDelete = folderId;
        DOMRefs.deleteFolderModal.classList.add('active');
    },

    async handleDeleteFolder() {
        if (!State.folderToDelete) return;
        
        const deleteOption = document.querySelector('input[name="deleteOption"]:checked').value;
        const deleteNotes = deleteOption === 'delete';
        
        try {
            await window.go.main.App.DeleteFolder(State.folderToDelete, deleteNotes);
            State.allFolders = State.allFolders.filter(f => f.id !== State.folderToDelete);
            
            // Reload notes if we moved them
            if (!deleteNotes) {
                await NoteManager.loadNotes();
            } else {
                // Remove deleted notes from memory
                State.allNotes = State.allNotes.filter(n => n.folderId !== State.folderToDelete);
            }
            
            // Clear current folder if it was deleted
            if (State.currentFolder && State.currentFolder.id === State.folderToDelete) {
                State.currentFolder = null;
            }
            
            DOMRefs.deleteFolderModal.classList.remove('active');
            State.folderToDelete = null;
            this.renderFolderTree();
        } catch (error) {
            console.error('Error deleting folder:', error);
            alert('Failed to delete folder: ' + error);
        }
    },

    buildFolderTree() {
        const folderMap = new Map();
        const rootFolders = [];
        
        // Create folder objects with children array
        State.allFolders.forEach(folder => {
            folderMap.set(folder.id, { ...folder, children: [] });
        });
        
        // Build hierarchy
        State.allFolders.forEach(folder => {
            const folderObj = folderMap.get(folder.id);
            if (folder.parentId && folderMap.has(folder.parentId)) {
                folderMap.get(folder.parentId).children.push(folderObj);
            } else {
                rootFolders.push(folderObj);
            }
        });
        
        return rootFolders;
    },

    renderFolderTree() {
        DOMRefs.folderTree.innerHTML = '';
        
        // Root notes section (notes without folder)
        const rootNotes = State.allNotes.filter(n => !n.folderId);
        if (rootNotes.length > 0 || State.allFolders.length === 0) {
            const rootSection = this.createNotesSection(rootNotes, null);
            DOMRefs.folderTree.appendChild(rootSection);
        }
        
        // Render folder hierarchy
        const tree = this.buildFolderTree();
        tree.forEach(folder => {
            const folderElement = this.createFolderElement(folder, 0);
            DOMRefs.folderTree.appendChild(folderElement);
        });
    },

    createFolderElement(folder, level) {
        const container = document.createElement('div');
        
        // Folder item
        const folderItem = document.createElement('div');
        folderItem.className = 'folder-item';
        if (State.currentFolder && State.currentFolder.id === folder.id) {
            folderItem.classList.add('selected');
        }
        folderItem.style.paddingLeft = (level * 16 + 12) + 'px';
        
        // Toggle arrow
        const toggle = document.createElement('span');
        toggle.className = 'folder-toggle';
        if (folder.children.length === 0) {
            toggle.classList.add('hidden');
        } else {
            toggle.textContent = '▶';
            if (State.expandedFolders.has(folder.id)) {
                toggle.classList.add('expanded');
            }
        }
        toggle.onclick = (e) => {
            e.stopPropagation();
            this.toggleFolder(folder.id);
        };
        
        // Folder icon
        const icon = document.createElement('span');
        icon.className = 'folder-icon';
        icon.textContent = State.expandedFolders.has(folder.id) ? '📂' : '📁';
        
        // Folder name
        const name = document.createElement('span');
        name.className = 'folder-name';
        name.textContent = folder.name;
        
        // Actions
        const actions = document.createElement('div');
        actions.className = 'folder-actions';
        
        const renameBtn = document.createElement('button');
        renameBtn.className = 'folder-action-btn';
        renameBtn.textContent = '✏️';
        renameBtn.title = 'Rename';
        renameBtn.onclick = (e) => {
            e.stopPropagation();
            this.renameFolder(folder.id);
        };
        
        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'folder-action-btn';
        deleteBtn.textContent = '🗑️';
        deleteBtn.title = 'Delete';
        deleteBtn.onclick = (e) => {
            e.stopPropagation();
            this.showDeleteFolderModal(folder.id);
        };
        
        actions.appendChild(renameBtn);
        actions.appendChild(deleteBtn);
        
        folderItem.appendChild(toggle);
        folderItem.appendChild(icon);
        folderItem.appendChild(name);
        folderItem.appendChild(actions);
        
        folderItem.onclick = () => this.selectFolder(folder);
        
        container.appendChild(folderItem);
        
        // Children (subfolders and notes)
        if (folder.children.length > 0 || State.allNotes.some(n => n.folderId === folder.id)) {
            const children = document.createElement('div');
            children.className = 'folder-children';
            if (!State.expandedFolders.has(folder.id)) {
                children.classList.add('collapsed');
            }
            
            // Render subfolders
            folder.children.forEach(child => {
                children.appendChild(this.createFolderElement(child, level + 1));
            });
            
            // Render notes in this folder
            const folderNotes = State.allNotes.filter(n => n.folderId === folder.id);
            if (folderNotes.length > 0) {
                const notesSection = this.createNotesSection(folderNotes, folder.id);
                notesSection.style.paddingLeft = ((level + 1) * 16 + 12) + 'px';
                children.appendChild(notesSection);
            }
            
            container.appendChild(children);
        }
        
        return container;
    },

    createNotesSection(notes, folderId) {
        const section = document.createElement('div');
        section.className = 'notes-section';
        
        const list = document.createElement('div');
        list.className = 'notes-list';
        
        if (notes.length === 0 && folderId === null) {
            list.innerHTML = '<div class="empty-folder">No notes yet</div>';
        } else {
            notes.forEach(note => {
                const noteItem = document.createElement('div');
                noteItem.className = 'note-item';
                if (State.currentNote && State.currentNote.id === note.id) {
                    noteItem.classList.add('active');
                }
                
                const date = new Date(note.updatedAt);
                const formattedDate = date.toLocaleDateString('en-US', {
                    month: 'short',
                    day: 'numeric',
                    year: 'numeric'
                });
                
                noteItem.innerHTML = `
                    <div class="note-item-title">${note.title || 'Untitled'}</div>
                    <div class="note-item-date">${formattedDate}</div>
                `;
                
                noteItem.onclick = () => NoteManager.loadNote(note.id);
                list.appendChild(noteItem);
            });
        }
        
        section.appendChild(list);
        return section;
    },

    toggleFolder(folderId) {
        if (State.expandedFolders.has(folderId)) {
            State.expandedFolders.delete(folderId);
        } else {
            State.expandedFolders.add(folderId);
        }
        this.renderFolderTree();
    },

    selectFolder(folder) {
        State.currentFolder = folder;
        State.currentNote = null;
        State.expandedFolders.add(folder.id);
        this.renderFolderTree();
        this.updateBreadcrumb();
        
        DOMRefs.emptyState.style.display = 'none';
        DOMRefs.editorView.style.display = 'none';
    },

    async updateBreadcrumb() {
        DOMRefs.breadcrumb.innerHTML = '';
        
        // Root item
        const rootItem = document.createElement('span');
        rootItem.className = 'breadcrumb-item';
        rootItem.textContent = 'All Notes';
        rootItem.dataset.folderId = '';
        rootItem.onclick = () => this.navigateToBreadcrumb('');
        DOMRefs.breadcrumb.appendChild(rootItem);
        
        if (State.currentFolder || (State.currentNote && State.currentNote.folderId)) {
            const folderId = State.currentFolder ? State.currentFolder.id : State.currentNote.folderId;
            try {
                const path = await window.go.main.App.GetFolderPath(folderId);
                path.forEach(folder => {
                    const separator = document.createElement('span');
                    separator.className = 'breadcrumb-separator';
                    separator.textContent = '/';
                    DOMRefs.breadcrumb.appendChild(separator);
                    
                    const item = document.createElement('span');
                    item.className = 'breadcrumb-item';
                    item.textContent = folder.name;
                    item.dataset.folderId = folder.id;
                    item.onclick = () => this.navigateToBreadcrumb(folder.id);
                    DOMRefs.breadcrumb.appendChild(item);
                });
            } catch (error) {
                console.error('Error loading breadcrumb:', error);
            }
        }
    },

    navigateToBreadcrumb(folderId) {
        if (folderId === '') {
            State.currentFolder = null;
        } else {
            State.currentFolder = State.allFolders.find(f => f.id === folderId);
            if (State.currentFolder) {
                State.expandedFolders.add(folderId);
            }
        }
        this.renderFolderTree();
        this.updateBreadcrumb();
    }
};