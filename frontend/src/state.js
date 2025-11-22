// App state management
const State = {
    // Note and folder state
    currentNote: null,
    currentFolder: null,
    allNotes: [],
    allFolders: [],
    expandedFolders: new Set(),
    
    // UI state
    isPreviewMode: false,
    selectedTargetFolder: null,
    folderToDelete: null,
    
    // Voice recording state
    isRecording: false,
    recordingStartTime: null,
    recordingTimer: null,
    sttAvailable: false,
    lastInsertPosition: null
};