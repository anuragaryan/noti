// Markdown preview mode functionality
export default {
    togglePreview() {
        State.isPreviewMode = !State.isPreviewMode;
        
        if (State.isPreviewMode) {
            this.updatePreview();
            DOMRefs.editorMode.style.display = 'none';
            DOMRefs.previewMode.style.display = 'block';
            DOMRefs.previewBtn.textContent = 'Edit';
        } else {
            DOMRefs.editorMode.style.display = 'block';
            DOMRefs.previewMode.style.display = 'none';
            DOMRefs.previewBtn.textContent = 'Preview';
        }
    },

    updatePreview() {
        const html = MarkdownParser.parse(DOMRefs.noteContent.value);
        DOMRefs.markdownPreview.innerHTML = html;
    }
};