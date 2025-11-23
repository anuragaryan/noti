import { promptManager } from './prompt-manager.js';
import State from '../state.js';

class PromptUI {
    constructor() {
        this.currentEditingPromptId = null;
        this.selectedPromptId = null;
    }

    initialize() {
        this.setupEventListeners();
        this.loadPromptsIntoDropdown();
    }

    setupEventListeners() {
        // Prompt selector and execution
        document.getElementById('promptSelector').addEventListener('change', (e) => {
            const runBtn = document.getElementById('runPromptBtn');
            runBtn.disabled = !e.target.value;
        });

        document.getElementById('runPromptBtn').addEventListener('click', () => {
            this.executePrompt();
        });

        document.getElementById('managePromptsBtn').addEventListener('click', () => {
            this.openPromptManagement();
        });

        // Result panel actions
        document.getElementById('closeResultBtn').addEventListener('click', () => {
            document.getElementById('promptResult').style.display = 'none';
        });

        document.getElementById('insertResultBtn').addEventListener('click', () => {
            this.insertResultIntoNote();
        });

        document.getElementById('copyResultBtn').addEventListener('click', () => {
            this.copyResult();
        });

        // Prompt management modal
        document.getElementById('closePromptManagementBtn').addEventListener('click', () => {
            this.closePromptManagement();
        });

        document.getElementById('newPromptBtn').addEventListener('click', () => {
            this.showPromptEditor();
        });

        document.getElementById('closeEditorBtn').addEventListener('click', () => {
            this.hidePromptEditor();
        });

        document.getElementById('savePromptBtn').addEventListener('click', () => {
            this.savePrompt();
        });

        document.getElementById('cancelPromptBtn').addEventListener('click', () => {
            this.hidePromptEditor();
        });

        document.getElementById('deletePromptBtn').addEventListener('click', () => {
            this.showDeleteConfirmation();
        });

        // Delete confirmation modal
        document.getElementById('cancelDeletePromptBtn').addEventListener('click', () => {
            this.hideDeleteConfirmation();
        });

        document.getElementById('confirmDeletePromptBtn').addEventListener('click', () => {
            this.confirmDeletePrompt();
        });

        // Temperature slider
        document.getElementById('promptTemp').addEventListener('input', (e) => {
            document.getElementById('tempValue').textContent = e.target.value;
        });

        // Listen for prompt changes
        promptManager.onPromptsChanged = (prompts) => {
            this.loadPromptsIntoDropdown();
            this.renderPromptsList(prompts);
        };
    }

    async loadPromptsIntoDropdown() {
        try {
            // Use existing prompts if already loaded, otherwise load them
            const prompts = promptManager.prompts.length > 0
                ? promptManager.prompts
                : await promptManager.loadPrompts();
            
            const selector = document.getElementById('promptSelector');
            
            selector.innerHTML = '<option value="">Select a prompt...</option>';
            
            prompts.forEach(prompt => {
                const option = document.createElement('option');
                option.value = prompt.id;
                option.textContent = prompt.name;
                selector.appendChild(option);
            });
        } catch (error) {
            console.error('Failed to load prompts:', error);
        }
    }

    async executePrompt() {
        const promptId = document.getElementById('promptSelector').value;
        const noteId = State.currentNote?.id;

        if (!promptId) {
            alert('Please select a prompt');
            return;
        }

        if (!noteId) {
            alert('Please select a note first');
            return;
        }

        try {
            // Show loading state
            const runBtn = document.getElementById('runPromptBtn');
            const originalText = runBtn.textContent;
            runBtn.textContent = 'Running...';
            runBtn.disabled = true;

            const result = await promptManager.executeOnNote(promptId, noteId);

            // Show result
            document.getElementById('promptOutput').textContent = result.output;
            document.getElementById('promptResult').style.display = 'block';

            runBtn.textContent = originalText;
            runBtn.disabled = false;
        } catch (error) {
            alert('Failed to execute prompt: ' + error.message);
            console.error('Prompt execution error:', error);
            
            const runBtn = document.getElementById('runPromptBtn');
            runBtn.textContent = 'Run Prompt';
            runBtn.disabled = false;
        }
    }

    insertResultIntoNote() {
        const output = document.getElementById('promptOutput').textContent;
        const textarea = document.getElementById('noteContent');
        
        // Insert at cursor or append
        const cursorPos = textarea.selectionStart;
        const textBefore = textarea.value.substring(0, cursorPos);
        const textAfter = textarea.value.substring(cursorPos);
        
        textarea.value = textBefore + '\n\n' + output + '\n\n' + textAfter;
        
        // Close result panel
        document.getElementById('promptResult').style.display = 'none';
    }

    async copyResult() {
        const output = document.getElementById('promptOutput').textContent;
        
        try {
            await navigator.clipboard.writeText(output);
            
            // Show feedback
            const btn = document.getElementById('copyResultBtn');
            const originalText = btn.textContent;
            btn.textContent = 'Copied!';
            setTimeout(() => {
                btn.textContent = originalText;
            }, 2000);
        } catch (error) {
            console.error('Failed to copy:', error);
            alert('Failed to copy to clipboard');
        }
    }

    openPromptManagement() {
        document.getElementById('promptManagementModal').classList.add('active');
        this.renderPromptsList(promptManager.prompts);
    }

    closePromptManagement() {
        document.getElementById('promptManagementModal').classList.remove('active');
        this.hidePromptEditor();
    }

    renderPromptsList(prompts) {
        const listContainer = document.getElementById('promptsList');
        
        if (prompts.length === 0) {
            listContainer.innerHTML = `
                <div class="prompts-empty">
                    <h4>No prompts yet</h4>
                    <p>Create your first prompt to get started</p>
                </div>
            `;
            return;
        }

        listContainer.innerHTML = prompts.map(prompt => `
            <div class="prompt-card ${this.selectedPromptId === prompt.id ? 'selected' : ''}" data-prompt-id="${prompt.id}">
                <div class="prompt-card-header">
                    <h5>${this.escapeHtml(prompt.name)}</h5>
                    <div class="prompt-card-actions">
                        <button class="prompt-card-btn edit-prompt-btn" data-prompt-id="${prompt.id}">✏️</button>
                    </div>
                </div>
                <p>${this.escapeHtml(prompt.description)}</p>
                <div class="prompt-card-meta">
                    <span>🌡️ ${prompt.temperature}</span>
                    <span>📝 ${prompt.maxTokens} tokens</span>
                </div>
            </div>
        `).join('');

        // Add click handlers
        listContainer.querySelectorAll('.prompt-card').forEach(card => {
            card.addEventListener('click', (e) => {
                // Check if click is on edit button or its children
                const editBtn = e.target.closest('.edit-prompt-btn');
                if (!editBtn) {
                    const promptId = card.dataset.promptId;
                    this.selectPrompt(promptId);
                }
            });
        });

        listContainer.querySelectorAll('.edit-prompt-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const promptId = btn.dataset.promptId;
                this.editPrompt(promptId);
            });
        });
    }

    selectPrompt(promptId) {
        this.selectedPromptId = promptId;
        this.renderPromptsList(promptManager.prompts);
    }

    showPromptEditor(prompt = null) {
        this.currentEditingPromptId = prompt?.id || null;
        
        document.getElementById('promptEditorTitle').textContent = prompt ? 'Edit Prompt' : 'New Prompt';
        document.getElementById('promptName').value = prompt?.name || '';
        document.getElementById('promptDescription').value = prompt?.description || '';
        document.getElementById('promptSystem').value = prompt?.systemPrompt || '';
        document.getElementById('promptUser').value = prompt?.userPrompt || '';
        document.getElementById('promptTemp').value = prompt?.temperature || 0.7;
        document.getElementById('tempValue').textContent = prompt?.temperature || 0.7;
        document.getElementById('promptTokens').value = prompt?.maxTokens || 512;
        
        const deleteBtn = document.getElementById('deletePromptBtn');
        deleteBtn.style.display = prompt ? 'block' : 'none';
        
        document.getElementById('promptEditor').style.display = 'flex';
    }

    hidePromptEditor() {
        document.getElementById('promptEditor').style.display = 'none';
        this.currentEditingPromptId = null;
    }

    async editPrompt(promptId) {
        try {
            const prompt = await promptManager.getPrompt(promptId);
            this.showPromptEditor(prompt);
        } catch (error) {
            console.error('Failed to load prompt:', error);
            alert('Failed to load prompt');
        }
    }

    async savePrompt() {
        const name = document.getElementById('promptName').value.trim();
        const description = document.getElementById('promptDescription').value.trim();
        const systemPrompt = document.getElementById('promptSystem').value.trim();
        const userPrompt = document.getElementById('promptUser').value.trim();
        const temperature = parseFloat(document.getElementById('promptTemp').value);
        const maxTokens = parseInt(document.getElementById('promptTokens').value);

        if (!name || !systemPrompt || !userPrompt) {
            alert('Please fill in all required fields (Name, System Prompt, User Prompt)');
            return;
        }

        try {
            if (this.currentEditingPromptId) {
                // Update existing prompt
                await promptManager.updatePrompt(
                    this.currentEditingPromptId,
                    name,
                    description,
                    systemPrompt,
                    userPrompt,
                    temperature,
                    maxTokens
                );
            } else {
                // Create new prompt
                await promptManager.createPrompt(
                    name,
                    description,
                    systemPrompt,
                    userPrompt,
                    temperature,
                    maxTokens
                );
            }

            this.hidePromptEditor();
        } catch (error) {
            console.error('Failed to save prompt:', error);
            alert('Failed to save prompt: ' + error.message);
        }
    }

    showDeleteConfirmation() {
        if (!this.currentEditingPromptId) return;
        document.getElementById('deletePromptModal').classList.add('active');
    }

    hideDeleteConfirmation() {
        document.getElementById('deletePromptModal').classList.remove('active');
    }

    async confirmDeletePrompt() {
        if (!this.currentEditingPromptId) return;

        try {
            await promptManager.deletePrompt(this.currentEditingPromptId);
            this.hideDeleteConfirmation();
            this.hidePromptEditor();
            // Refresh the prompts list after deletion
            await this.loadPromptsIntoDropdown();
        } catch (error) {
            console.error('Failed to delete prompt:', error);
            alert('Failed to delete prompt: ' + error.message);
            this.hideDeleteConfirmation();
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

export default new PromptUI();