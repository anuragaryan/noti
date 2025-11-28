// App state management
export default {
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
    lastInsertPosition: null,
    
    // Audio source state
    audioSource: 'microphone',
    availableAudioSources: [],
    audioPermissions: {}
};