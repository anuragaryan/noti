// DOM element references
export default {
    // Main containers
    folderTree: document.getElementById('folderTree'),
    emptyState: document.getElementById('emptyState'),
    editorView: document.getElementById('editorView'),
    
    // Editor elements
    noteTitle: document.getElementById('noteTitle'),
    noteContent: document.getElementById('noteContent'),
    markdownPreview: document.getElementById('markdownPreview'),
    editorMode: document.getElementById('editorMode'),
    previewMode: document.getElementById('previewMode'),
    breadcrumb: document.getElementById('breadcrumb'),
    
    // Action buttons
    newNoteBtn: document.getElementById('newNoteBtn'),
    newFolderBtn: document.getElementById('newFolderBtn'),
    saveBtn: document.getElementById('saveBtn'),
    deleteBtn: document.getElementById('deleteBtn'),
    previewBtn: document.getElementById('previewBtn'),
    moveNoteBtn: document.getElementById('moveNoteBtn'),
    
    // Voice recording elements
    voiceBtn: document.getElementById('voiceBtn'),
    stopVoiceBtn: document.getElementById('stopVoiceBtn'),
    voiceStatusBar: document.getElementById('voiceStatusBar'),
    voiceTimer: document.getElementById('voiceTimer'),
    transcriptionStatus: document.getElementById('transcriptionStatus'),
    sttSetupModal: document.getElementById('sttSetupModal'),
    closeSttSetupBtn: document.getElementById('closeSttSetupBtn'),
    modelPathEl: document.getElementById('modelPath'),
    
    // Folder modal elements
    folderModal: document.getElementById('folderModal'),
    folderModalTitle: document.getElementById('folderModalTitle'),
    folderNameInput: document.getElementById('folderNameInput'),
    createFolderBtn: document.getElementById('createFolderBtn'),
    cancelFolderBtn: document.getElementById('cancelFolderBtn'),
    
    // Move note modal elements
    moveNoteModal: document.getElementById('moveNoteModal'),
    folderSelector: document.getElementById('folderSelector'),
    confirmMoveBtn: document.getElementById('confirmMoveBtn'),
    cancelMoveBtn: document.getElementById('cancelMoveBtn'),
    
    // Delete folder modal elements
    deleteFolderModal: document.getElementById('deleteFolderModal'),
    confirmDeleteFolderBtn: document.getElementById('confirmDeleteFolderBtn'),
    cancelDeleteFolderBtn: document.getElementById('cancelDeleteFolderBtn')
};